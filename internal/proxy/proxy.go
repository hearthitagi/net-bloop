package proxy

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"net-bloop/internal/cert"
	"net-bloop/internal/config"
	"net-bloop/internal/logx"
)

// Manager owns the single HTTPS/HTTP listener that every rule shares. Rules are
// told apart by the requested host name, so adding a service never means adding
// a port or a certificate.
type Manager struct {
	store   *config.Store
	log     *logx.Logger
	dataDir string
	certs   *cert.Manager

	mu       sync.Mutex
	inst     *instance
	lastErr  string
	routes   int
	https    bool
}

type instance struct {
	listen string
	hash   string
	ln     net.Listener
	srv    *http.Server
	done   chan struct{}
	stop   context.CancelFunc
	router *hostRouter
}

// State is what the panel shows about the web listener.
type State struct {
	Running bool   `json:"running"`
	Listen  string `json:"listen"`
	HTTPS   bool   `json:"https"`
	Routes  int    `json:"routes"`
	Domains int    `json:"domains"`
	Error   string `json:"error"`
}

func NewManager(store *config.Store, log *logx.Logger, dataDir string) *Manager {
	return &Manager{store: store, log: log, dataDir: dataDir}
}

// SetCertManager lets the listener pick up certificates issued by the built-in
// ACME client.
func (m *Manager) SetCertManager(c *cert.Manager) { m.certs = c }

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := State{Running: m.inst != nil, Routes: m.routes, HTTPS: m.https, Error: m.lastErr}
	if m.inst != nil {
		st.Listen = m.inst.listen
		st.Domains = m.inst.router.domainCount()
	}
	return st
}

func routeHash(cfg *config.Config) string {
	rules := make([]config.ProxyRule, 0, len(cfg.Proxy))
	for _, r := range cfg.Proxy {
		if r.Enable {
			rules = append(rules, r)
		}
	}
	raw, _ := json.Marshal(struct {
		Web   config.WebConfig
		Rules []config.ProxyRule
	}{cfg.Web, rules})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

func (m *Manager) Reconcile(cfg *config.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()

	router, err := buildRouter(cfg, m.log)
	if err != nil {
		m.lastErr = err.Error()
		m.log.Error("规则有误: %v", err)
		if m.inst != nil {
			return
		}
	}
	m.routes = len(cfg.Proxy)
	m.https = cfg.Web.HTTPS

	hash := routeHash(cfg)
	if m.inst != nil && m.inst.listen == cfg.Web.Listen && router != nil {
		// Same port: swap routes in place so open streams are not interrupted.
		m.inst.router.replace(router)
		m.inst.hash = hash
		if err != nil {
			m.lastErr = err.Error()
		} else {
			m.lastErr = ""
		}
		m.log.Info("路由已更新，共 %d 条规则", m.routes)
		return
	}

	if m.inst != nil {
		m.inst.stop()
		m.inst = nil
	}
	if router == nil {
		return
	}
	inst, err := m.start(cfg, router, hash)
	if err != nil {
		m.lastErr = err.Error()
		m.log.Error("Web 服务启动失败: %v", err)
		return
	}
	m.lastErr = ""
	m.inst = inst
}

func (m *Manager) start(cfg *config.Config, router *hostRouter, hash string) (*instance, error) {
	ln, err := net.Listen("tcp", cfg.Web.Listen)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	inst := &instance{listen: cfg.Web.Listen, hash: hash, ln: ln, router: router, done: make(chan struct{}), stop: cancel}

	srv := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          log.New(os.Stderr, "[web] ", log.LstdFlags),
	}
	inst.srv = srv

	var tlsCfg *tls.Config
	if cfg.Web.HTTPS {
		store := &certStore{manager: m.certs, log: m.log, web: cfg.Web}
		if _, err := store.load(); err != nil {
			// No certificate yet — typical on a first boot, because ACME is
			// still running. Keep TLS installed anyway: certStore.get retries
			// the load on every handshake, so the port upgrades itself the
			// moment the file lands instead of waiting for a restart.
			m.log.Warn("HTTPS 已启用但暂时没有证书: %v（签发完成后自动生效）", err)
		}
		tlsCfg = &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: store.get,
		}
	}

	lst := &muxListener{inner: ln, tlsCfg: tlsCfg}
	go func() {
		defer close(inst.done)
		if err := srv.Serve(lst); err != nil && err != http.ErrServerClosed {
			m.log.Error("Web 服务退出: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		sctx, scancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer scancel()
		_ = srv.Shutdown(sctx)
		_ = ln.Close()
	}()

	mode := "HTTP"
	if tlsCfg != nil {
		mode = "HTTPS"
	}
	m.log.Info("[%s:%s] 已启动，%d 条规则，%d 个域名", mode, config.PortOf(cfg.Web.Listen), len(cfg.Proxy), router.domainCount())
	return inst, nil
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inst != nil {
		m.inst.stop()
		select {
		case <-m.inst.done:
		case <-time.After(6 * time.Second):
		}
		m.inst = nil
	}
}

// ------------------------------------------------------------- host routing

type hostRouter struct {
	mu          sync.RWMutex
	exact       map[string]*ruleHandler
	wildcard    []*ruleHandler
	defaultRule *ruleHandler
	accessLog   bool
	web         config.WebConfig
	log         *logx.Logger
}

func buildRouter(cfg *config.Config, lg *logx.Logger) (*hostRouter, error) {
	h := &hostRouter{exact: map[string]*ruleHandler{}, accessLog: cfg.Web.AccessLog, web: cfg.Web, log: lg}
	for _, r := range cfg.Proxy {
		if !r.Enable {
			continue
		}
		rh, err := newRuleHandler(r, lg)
		if err != nil {
			return nil, fmt.Errorf("规则 %s: %w", r.Name, err)
		}
		if len(r.Domains) == 0 {
			if h.defaultRule != nil {
				return nil, fmt.Errorf("只能有一条不填域名的默认规则")
			}
			h.defaultRule = rh
			continue
		}
		for _, d := range r.Domains {
			d = normalizeHost(d)
			if strings.HasPrefix(d, "*.") {
				h.wildcard = append(h.wildcard, rh)
			} else {
				h.exact[d] = rh
			}
		}
	}
	return h, nil
}

// replace swaps the routing table without touching the listener.
func (h *hostRouter) replace(next *hostRouter) {
	h.mu.Lock()
	h.exact = next.exact
	h.wildcard = next.wildcard
	h.defaultRule = next.defaultRule
	h.accessLog = next.accessLog
	h.web = next.web
	h.mu.Unlock()
}

func (h *hostRouter) domainCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := len(h.exact)
	for _, rh := range h.wildcard {
		n += len(rh.rule.Domains)
	}
	if h.defaultRule != nil {
		n++
	}
	return n
}

func (h *hostRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rh := h.match(r.Host)
	if rh == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, "404 未找到 %s 对应的服务\n\nnet-bloop 收到请求，但没有规则匹配这个域名。\n", r.Host)
		return
	}
	cfg := h.webConfig()
	if cfg.RedirectHTTPS && cfg.HTTPS && r.TLS == nil {
		target := "https://" + stripPort(r.Host) + portSuffix(cfg.Listen) + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
		return
	}
	if rh.rule.BasicAuth != "" {
		user, pass, ok := r.BasicAuth()
		want := rh.rule.BasicAuth
		got := user + ":" + pass
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="net-bloop"`)
			http.Error(w, "401 需要认证", http.StatusUnauthorized)
			return
		}
	}
	h.mu.RLock()
	accessLog := h.accessLog
	h.mu.RUnlock()
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: 200}
	rh.rp.ServeHTTP(rec, r)
	if accessLog {
		rh.log.Info("%s %s%s %d %s", r.Method, r.Host, r.URL.RequestURI(), rec.status, time.Since(start).Round(time.Millisecond))
	}
}

func (h *hostRouter) webConfig() config.WebConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.web
}

func (h *hostRouter) match(host string) *ruleHandler {
	host = normalizeHost(host)
	h.mu.RLock()
	defer h.mu.RUnlock()
	if rh, ok := h.exact[host]; ok {
		return rh
	}
	for _, rh := range h.wildcard {
		for _, d := range rh.rule.Domains {
			if matchDomain(host, d) {
				return rh
			}
		}
	}
	return h.defaultRule
}

// matchDomain reports whether a host matches a pattern, honouring a leading
// "*." wildcard. The bare domain never matches its own wildcard.
func matchDomain(host, pattern string) bool {
	host, pattern = normalizeHost(host), normalizeHost(pattern)
	if host == pattern {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(host, pattern[1:])
	}
	return false
}

func normalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.TrimSuffix(h, ".")
}

func stripPort(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

func portSuffix(listen string) string {
	p := config.PortOf(listen)
	if p == "" || p == "443" {
		return ""
	}
	return ":" + p
}

// -------------------------------------------------------------- rule handler

type ruleHandler struct {
	rule    config.ProxyRule
	log     *logx.Logger
	rp      *httputil.ReverseProxy
	targets []*url.URL
	counter atomic.Uint64
}

func newRuleHandler(r config.ProxyRule, lg *logx.Logger) (*ruleHandler, error) {
	targets := make([]*url.URL, 0, len(r.Upstreams))
	for _, raw := range r.Upstreams {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
			raw = "http://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("上游地址无效: %q", raw)
		}
		targets = append(targets, u)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("没有可用的上游地址")
	}
	rh := &ruleHandler{rule: r, log: lg.WithRule(r.Name), targets: targets}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
	}
	rh.rp = &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			t := targets[int(rh.counter.Add(1)-1)%len(targets)]
			pr.Out.URL.Scheme = t.Scheme
			pr.Out.URL.Host = t.Host
			pr.Out.URL.Path, pr.Out.URL.RawPath = joinURLPath(t, pr.In.URL)
			if r.UseTargetHost {
				pr.Out.Host = t.Host
			} else {
				pr.Out.Host = pr.In.Host
			}
			pr.SetXForwarded()
			if ip, _, err := net.SplitHostPort(pr.In.RemoteAddr); err == nil {
				pr.Out.Header.Set("X-Real-IP", ip)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			rh.log.Error("转发 %s%s 失败: %v", req.Host, req.URL.Path, err)
			http.Error(w, "502 上游不可用", http.StatusBadGateway)
		},
	}
	return rh, nil
}

func joinURLPath(base *url.URL, in *url.URL) (string, string) {
	if base.Path == "" || base.Path == "/" {
		return in.Path, in.RawPath
	}
	return strings.TrimSuffix(base.Path, "/") + in.Path, ""
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("不支持 hijack")
	}
	return h.Hijack()
}

func (r *statusRecorder) Push(target string, opts *http.PushOptions) error {
	if p, ok := r.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// ----------------------------------------------------------- TLS with reload

// certStore reloads the certificate when the file changes on disk, so a renewal
// takes effect without restarting the container.
type certStore struct {
	mu      sync.Mutex
	manager *cert.Manager
	log     *logx.Logger
	web     config.WebConfig
	cur     *tls.Certificate
	modTime time.Time
	size    int64
	lastErr error
}

func (s *certStore) load() (*tls.Certificate, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("证书模块未初始化")
	}
	c, err := s.manager.Load(s.web.CertFile, s.web.KeyFile)
	if err != nil {
		s.lastErr = err
		return nil, err
	}
	s.cur = c
	s.lastErr = nil
	if st, err := os.Stat(s.manager.CertPath()); err == nil {
		s.modTime = st.ModTime()
		s.size = st.Size()
	}
	return c, nil
}

func (s *certStore) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.manager == nil {
		return nil, fmt.Errorf("没有可用证书")
	}
	path := s.manager.CertPath()
	if s.web.CertFile != "" {
		path = s.web.CertFile
	}
	reload := s.cur == nil
	if st, err := os.Stat(path); err == nil {
		if !st.ModTime().Equal(s.modTime) || st.Size() != s.size {
			reload = true
		}
	}
	if reload {
		if _, err := s.load(); err != nil {
			if s.cur != nil {
				s.log.Warn("证书重载失败，继续使用旧证书: %v", err)
			} else {
				return nil, err
			}
		} else {
			s.log.Info("证书已热加载")
		}
	}
	return s.cur, nil
}

// muxListener splits TLS and plain HTTP on one port by peeking the first byte.
type muxListener struct {
	inner  net.Listener
	tlsCfg *tls.Config
}

func (l *muxListener) Accept() (net.Conn, error) {
	for {
		c, err := l.inner.Accept()
		if err != nil {
			return nil, err
		}
		br := bufio.NewReader(c)
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
		first, err := br.Peek(1)
		_ = c.SetReadDeadline(time.Time{})
		if err != nil || len(first) == 0 {
			_ = c.Close()
			continue
		}
		bc := &bufferedConn{Conn: c, r: br}
		if first[0] == 0x16 && l.tlsCfg != nil {
			return tls.Server(bc, l.tlsCfg), nil
		}
		return bc, nil
	}
}

func (l *muxListener) Close() error   { return l.inner.Close() }
func (l *muxListener) Addr() net.Addr { return l.inner.Addr() }

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) { return c.r.Read(b) }
