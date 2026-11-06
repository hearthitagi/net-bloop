package cert

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"

	"net-bloop/internal/config"
	"net-bloop/internal/logx"
	"net-bloop/internal/netx"
)

const (
	dirName      = "cert"
	certName     = "fullchain.pem"
	keyName      = "privkey.pem"
	accountName  = "account.key"
	issueTimeout = 5 * time.Minute
	renewCheck   = 6 * time.Hour
)

// Status is what the panel shows about the certificate in use.
type Status struct {
	Enabled      bool      `json:"enabled"`
	Ready        bool      `json:"ready"`
	Domains      []string  `json:"domains"`
	Issuer       string    `json:"issuer"`
	NotAfter     time.Time `json:"not_after"`
	DaysLeft     int       `json:"days_left"`
	LastError    string    `json:"last_error"`
	LastIssue    time.Time `json:"last_issue"`
	Issuing      bool      `json:"issuing"`
	CertPath     string    `json:"cert_path"`
	ManualCert   bool      `json:"manual_cert"`
	ProviderName string    `json:"provider"`
}

// Manager issues and renews one wildcard certificate over DNS-01. Certificates
// are written to <dataDir>/cert and picked up by the proxy without a restart.
type Manager struct {
	store   *config.Store
	log     *logx.Logger
	dataDir string

	mu       sync.RWMutex
	status   Status
	issuing  bool

	cancel context.CancelFunc
	done   chan struct{}
}

func NewManager(store *config.Store, log *logx.Logger, dataDir string) *Manager {
	m := &Manager{store: store, log: log, dataDir: filepath.Join(dataDir, dirName)}
	m.refreshStatus()
	return m
}

func (m *Manager) CertPath() string { return filepath.Join(m.dataDir, certName) }
func (m *Manager) KeyPath() string  { return filepath.Join(m.dataDir, keyName) }

// Start kicks off the renewal loop. Issuance is attempted immediately when
// there is no usable certificate yet.
func (m *Manager) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.done = make(chan struct{})
	go func() {
		defer close(m.done)
		cfg := m.store.Get()
		if cfg.Cert.Enable {
			if !m.status.Ready {
				m.log.Info("证书尚不存在，开始申请 %s", strings.Join(cfg.Cert.Domains, ", "))
				if err := m.Ensure(ctx, false); err != nil {
					m.log.Error("首次签发失败: %v", err)
				}
			} else if m.needsRenew() {
				if err := m.Ensure(ctx, false); err != nil {
					m.log.Error("续期失败: %v", err)
				}
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(renewCheck):
			}
			cfg := m.store.Get()
			if !cfg.Cert.Enable {
				continue
			}
			if m.needsRenew() {
				m.log.Info("证书临近到期，开始续期")
				if err := m.Ensure(ctx, false); err != nil {
					m.log.Error("续期失败，继续使用旧证书: %v", err)
				}
			}
		}
	}()
}

func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.done != nil {
		select {
		case <-m.done:
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *Manager) needsRenew() bool {
	m.refreshStatus()
	if !m.status.Ready {
		return true
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Until(m.status.NotAfter) < m.store.Get().Cert.RenewBefore.Duration()
}

// refreshStatus reads the certificate on disk so the panel stays truthful even
// after a restart or a manual file replacement.
func (m *Manager) refreshStatus() {
	cfg := m.store.Get()
	st := Status{
		Enabled:      cfg.Cert.Enable,
		ProviderName: cfg.Cert.Provider,
		CertPath:     m.CertPath(),
		Domains:      append([]string(nil), cfg.Cert.Domains...),
	}
	path := m.CertPath()
	if cfg.Web.CertFile != "" {
		path = resolvePath(m.dataDir, cfg.Web.CertFile)
		st.ManualCert = true
	}
	if leaf, err := readLeaf(path); err == nil {
		st.Ready = true
		st.Issuer = leaf.Issuer.CommonName
		st.NotAfter = leaf.NotAfter
		st.DaysLeft = int(time.Until(leaf.NotAfter).Hours() / 24)
		st.Domains = leaf.DNSNames
	}
	m.mu.Lock()
	st.LastError = m.status.LastError
	st.LastIssue = m.status.LastIssue
	st.Issuing = m.issuing
	m.status = st
	m.mu.Unlock()
}

func (m *Manager) Status() Status {
	m.refreshStatus()
	m.mu.RLock()
	defer m.mu.RUnlock()
	st := m.status
	st.Issuing = m.issuing
	return st
}

// Ensure issues a certificate if needed. force skips the "still valid" check.
func (m *Manager) Ensure(ctx context.Context, force bool) error {
	m.mu.Lock()
	if m.issuing {
		m.mu.Unlock()
		return fmt.Errorf("正在签发中，请稍候")
	}
	m.issuing = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.issuing = false
		m.mu.Unlock()
	}()

	cfg := m.store.Get()
	if !cfg.Cert.Enable {
		return fmt.Errorf("自动证书未启用")
	}
	if len(cfg.Cert.Domains) == 0 {
		return fmt.Errorf("没有配置域名")
	}
	if !force && m.status.Ready && !m.needsRenew() {
		return nil
	}
	if cfg.Cert.Provider != "cloudflare" {
		return fmt.Errorf("暂只支持 Cloudflare DNS，当前为 %s", cfg.Cert.Provider)
	}
	token := strings.TrimSpace(cfg.Cert.Auth["token"])
	if token == "" {
		return fmt.Errorf("缺少 Cloudflare API Token")
	}

	ictx, cancel := context.WithTimeout(ctx, issueTimeout)
	defer cancel()

	der, key, err := m.issue(ictx, cfg, token)
	if err != nil {
		m.setErr(err)
		return err
	}
	if err := m.write(der, key); err != nil {
		m.setErr(err)
		return err
	}
	m.mu.Lock()
	m.status.LastIssue = time.Now()
	m.status.LastError = ""
	m.mu.Unlock()
	m.refreshStatus()
	m.log.Info("证书已更新：%s，有效期至 %s", strings.Join(m.status.Domains, ", "), m.status.NotAfter.Format("2006-01-02"))
	return nil
}

func (m *Manager) setErr(err error) {
	m.mu.Lock()
	m.status.LastError = err.Error()
	m.mu.Unlock()
}

func (m *Manager) issue(ctx context.Context, cfg *config.Config, token string) ([][]byte, *ecdsa.PrivateKey, error) {
	acctKey, err := m.accountKey()
	if err != nil {
		return nil, nil, err
	}
	dirURL := cfg.Cert.Directory
	if dirURL == "" {
		dirURL = config.DefaultACMEDirectory
	}
	client := &acme.Client{
		Key:          acctKey,
		DirectoryURL: dirURL,
		HTTPClient:   netx.Client(cfg.Net.Proxy, 30*time.Second),
		UserAgent:    "net-bloop/1.0",
	}
	acct := &acme.Account{}
	if e := cfg.Cert.Email; e != "" {
		acct.Contact = []string{"mailto:" + e}
	}
	if _, err := client.Register(ctx, acct, acme.AcceptTOS); err != nil && err != acme.ErrAccountAlreadyExists {
		return nil, nil, fmt.Errorf("注册 ACME 账号失败: %w", err)
	}

	domains := cfg.Cert.Domains
	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(domains...))
	if err != nil {
		return nil, nil, fmt.Errorf("创建订单失败: %w", err)
	}

	dns := &cloudflareDNS{token: token, ctx: ctx, client: netx.Client(cfg.Net.Proxy, 20*time.Second)}
	var cleanups []func()
	defer func() {
		for _, f := range cleanups {
			f()
		}
	}()

	// Collect every pending authorization first. Apex and wildcard share the
	// same _acme-challenge name, so their records must coexist: Cloudflare
	// happily keeps several TXT values on one name, and Let's Encrypt accepts
	// a match against any of them — but only if both are already published
	// when validation runs.
	type pending struct {
		z    *acme.Authorization
		chal *acme.Challenge
		name string
		txt  string
	}
	var jobs []*pending
	for _, u := range order.AuthzURLs {
		z, err := client.GetAuthorization(ctx, u)
		if err != nil {
			return nil, nil, fmt.Errorf("读取授权失败: %w", err)
		}
		if z.Status == acme.StatusValid {
			continue
		}
		var chal *acme.Challenge
		for _, c := range z.Challenges {
			if c.Type == "dns-01" {
				chal = c
				break
			}
		}
		if chal == nil {
			return nil, nil, fmt.Errorf("%s 不支持 DNS-01", z.Identifier.Value)
		}
		txt, err := client.DNS01ChallengeRecord(chal.Token)
		if err != nil {
			return nil, nil, err
		}
		// A wildcard authorization is answered on the base domain.
		name := "_acme-challenge." + strings.TrimPrefix(z.Identifier.Value, "*.")
		jobs = append(jobs, &pending{z: z, chal: chal, name: name, txt: txt})
	}

	// Phase 1: publish every record.
	want := map[string][]string{}
	recorded := 0
	for _, j := range jobs {
		recID, zone, err := dns.setTXT(j.name, j.txt)
		if err != nil {
			return nil, nil, fmt.Errorf("写入 TXT 记录 %s 失败: %w", j.name, err)
		}
		r, z := recID, zone
		cleanups = append(cleanups, func() { dns.delTXT(z, r) })
		want[j.name] = append(want[j.name], j.txt)
		recorded++
	}
	if recorded == 0 {
		m.log.Info("所有域名此前已验证通过，直接签发")
	}

	// Phase 2: wait until the records are actually resolvable. Without this
	// the ACME server checks milliseconds after the API call returns and
	// reports "No TXT record found" — measured propagation was 2-6 seconds.
	if recorded > 0 {
		m.log.Info("已写入 %d 条 TXT 记录，等待 DNS 生效…", recorded)
		start := time.Now()
		if err := waitTXT(ctx, netx.Client(cfg.Net.Proxy, 8*time.Second), want, m.log); err != nil {
			return nil, nil, err
		}
		m.log.Info("TXT 记录已生效，用时 %s", time.Since(start).Round(time.Second))
	}

	// Phase 3: tell the server to validate.
	for _, j := range jobs {
		if _, err := client.Accept(ctx, j.chal); err != nil {
			return nil, nil, fmt.Errorf("确认挑战失败: %w", err)
		}
	}

	// Phase 4: wait for the result.
	for _, j := range jobs {
		if _, err := client.WaitAuthorization(ctx, j.z.URI); err != nil {
			return nil, nil, fmt.Errorf("等待 %s 验证失败: %w", j.z.Identifier.Value, err)
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	csr, err := makeCSR(domains, key)
	if err != nil {
		return nil, nil, err
	}
	der, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, nil, fmt.Errorf("签发证书失败: %w", err)
	}
	return der, key, nil
}

func (m *Manager) accountKey() (*ecdsa.PrivateKey, error) {
	path := filepath.Join(m.dataDir, accountName)
	if raw, err := os.ReadFile(path); err == nil {
		if blk, _ := pem.Decode(raw); blk != nil {
			if k, err := x509.ParseECPrivateKey(blk.Bytes); err == nil {
				return k, nil
			}
			if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
				if e, ok := k.(*ecdsa.PrivateKey); ok {
					return e, nil
				}
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(m.dataDir, 0o700); err != nil {
		return nil, err
	}
	body := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// makeCSR returns a DER-encoded request. acme.Client expects DER, not PEM:
// handing it PEM makes Let's Encrypt answer 400 malformed (base64 of the
// "-----BEGIN..." text is not valid ASN.1).
func makeCSR(domains []string, key *ecdsa.PrivateKey) ([]byte, error) {
	tmpl := &x509.CertificateRequest{DNSNames: domains}
	if len(domains) > 0 {
		tmpl.Subject = pkix.Name{CommonName: strings.TrimPrefix(domains[0], "*.")}
	}
	return x509.CreateCertificateRequest(rand.Reader, tmpl, key)
}

// write stores the chain and key atomically so a reader never sees half a file.
func (m *Manager) write(der [][]byte, key *ecdsa.PrivateKey) error {
	if err := os.MkdirAll(m.dataDir, 0o700); err != nil {
		return err
	}
	var chain bytes.Buffer
	for _, d := range der {
		_ = pem.Encode(&chain, &pem.Block{Type: "CERTIFICATE", Bytes: d})
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := writeAtomic(m.KeyPath(), keyPEM, 0o600); err != nil {
		return err
	}
	return writeAtomic(m.CertPath(), chain.Bytes(), 0o644)
}

func writeAtomic(path string, body []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load returns the certificate currently on disk, so the proxy can hot-swap it.
func (m *Manager) Load(certFile, keyFile string) (*tls.Certificate, error) {
	cf, kf := m.CertPath(), m.KeyPath()
	if certFile != "" && keyFile != "" {
		cf = resolvePath(filepath.Dir(m.dataDir), certFile)
		kf = resolvePath(filepath.Dir(m.dataDir), keyFile)
	}
	c, err := tls.LoadX509KeyPair(cf, kf)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func readLeaf(path string) (*x509.Certificate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(raw)
	if blk == nil {
		return nil, fmt.Errorf("证书格式无效")
	}
	return x509.ParseCertificate(blk.Bytes)
}

func resolvePath(dataDir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dataDir, p)
}

// ---------------------------------------------------------------- cloudflare

// cloudflareDNS only does the two things ACME needs: add a TXT record and
// remove it afterwards.
type cloudflareDNS struct {
	ctx    context.Context
	token  string
	client *http.Client
	zones  sync.Map
}

func (c *cloudflareDNS) do(method, path string, body any) (json.RawMessage, error) {
	return cfDo(c.ctx, c.client, c.token, method, "https://api.cloudflare.com/client/v4"+path, body)
}

// cfDo performs one Cloudflare API call and unwraps the envelope.
func cfDo(ctx context.Context, client *http.Client, token, method, u string, body any) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "net-bloop/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var env struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("Cloudflare 响应异常: %s", resp.Status)
	}
	if !env.Success {
		msgs := make([]string, 0, len(env.Errors))
		for _, e := range env.Errors {
			msgs = append(msgs, e.Message)
		}
		if len(msgs) == 0 {
			msgs = []string{resp.Status}
		}
		return nil, fmt.Errorf("Cloudflare: %s", strings.Join(msgs, "; "))
	}
	return env.Result, nil
}

func (c *cloudflareDNS) zoneFor(name string) (string, error) {
	if v, ok := c.zones.Load(name); ok {
		return v.(string), nil
	}
	labels := strings.Split(name, ".")
	for i := 1; i < len(labels)-1; i++ {
		candidate := strings.Join(labels[i:], ".")
		if v, ok := c.zones.Load(candidate); ok {
			id := v.(string)
			c.zones.Store(name, id)
			return id, nil
		}
		raw, err := c.do("GET", "/zones?name="+candidate, nil)
		if err != nil {
			continue
		}
		var zones []struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &zones) == nil && len(zones) > 0 {
			c.zones.Store(candidate, zones[0].ID)
			c.zones.Store(name, zones[0].ID)
			return zones[0].ID, nil
		}
	}
	return "", fmt.Errorf("找不到 %s 所属的 Cloudflare zone", name)
}

func (c *cloudflareDNS) setTXT(name, value string) (string, string, error) {
	zone, err := c.zoneFor(name)
	if err != nil {
		return "", "", err
	}
	body := map[string]any{"type": "TXT", "name": name, "content": value, "ttl": 60}
	raw, err := c.do("POST", "/zones/"+zone+"/dns_records", body)
	if err != nil {
		return "", "", err
	}
	var rec struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return "", "", err
	}
	return rec.ID, zone, nil
}

func (c *cloudflareDNS) delTXT(zone, id string) {
	if id == "" {
		return
	}
	_, _ = c.do("DELETE", "/zones/"+zone+"/dns_records/"+id, nil)
}

// ------------------------------------------------------- propagation check

// resolvers are DNS-over-HTTPS JSON endpoints. Three different networks on
// purpose: whichever one answers first is enough to prove the record is live.
var txtResolvers = []string{
	"https://cloudflare-dns.com/dns-query?name=%s&type=TXT",
	"https://dns.alidns.com/resolve?name=%s&type=TXT",
	"https://doh.pub/dns-query?name=%s&type=TXT",
}

var txtWaitTimeout = 150 * time.Second

// waitTXT polls public resolvers until every expected value shows up for its
// name. Apex and wildcard share one name with two values, hence the map.
func waitTXT(ctx context.Context, client *http.Client, want map[string][]string, lg *logx.Logger) error {
	deadline := time.Now().Add(txtWaitTimeout)
	answered := false
	for {
		seen := map[string]map[string]bool{}
		for name := range want {
			for _, tmpl := range txtResolvers {
				qctx, cancel := context.WithTimeout(ctx, 6*time.Second)
				vals, err := queryTXT(qctx, client, tmpl, name)
				cancel()
				if err != nil {
					continue
				}
				answered = true
				for _, v := range vals {
					if seen[name] == nil {
						seen[name] = map[string]bool{}
					}
					seen[name][v] = true
				}
			}
		}
		ready := 0
		for name, vals := range want {
			ok := true
			for _, v := range vals {
				if !seen[name][v] {
					ok = false
					break
				}
			}
			if ok {
				ready++
			}
		}
		if ready == len(want) {
			return nil
		}
		if time.Now().After(deadline) {
			if !answered {
				// Every resolver is unreachable (likely blocked outbound). A
				// fixed wait beats failing for the wrong reason.
				lg.Warn("DNS 解析器都不可达，固定等待 15 秒后继续验证")
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(15 * time.Second):
				}
				return nil
			}
			return fmt.Errorf("等待 DNS 生效超时（%s）：%d/%d 条已可见，检查该域名的 NS 是否指向 Cloudflare",
				txtWaitTimeout, ready, len(want))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// queryTXT reads TXT answers from a DNS-over-HTTPS JSON endpoint.
func queryTXT(ctx context.Context, client *http.Client, tmpl, name string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(tmpl, url.QueryEscape(name)), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/dns-json")
	req.Header.Set("User-Agent", "net-bloop/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	var body struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(body.Answer))
	for _, a := range body.Answer {
		if a.Type != 16 {
			continue
		}
		out = append(out, unquoteTXT(a.Data))
	}
	return out, nil
}

// unquoteTXT strips the presentation-format quotes; long records may arrive as
// several quoted chunks.
func unquoteTXT(s string) string {
	s = strings.ReplaceAll(s, "\"", "")
	return strings.ReplaceAll(s, " ", "")
}
