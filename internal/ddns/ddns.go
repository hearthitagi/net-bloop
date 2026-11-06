package ddns

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"net-bloop/internal/config"
	"net-bloop/internal/ddns/provider"
	"net-bloop/internal/logx"
	"net-bloop/internal/netx"
)

type Status struct {
	ID        string    `json:"id"`
	LastRun   time.Time `json:"last_run"`
	NextRun   time.Time `json:"next_run"`
	LastIP    string    `json:"last_ip"`
	LastError string    `json:"last_error"`
	OK        bool      `json:"ok"`
}

type Manager struct {
	store   *config.Store
	log     *logx.Logger
	mu      sync.Mutex
	runners map[string]*runner
	stunIP  func(rule string) (string, bool)
}

type runner struct {
	cfg    config.DDNSRule
	hash   string
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.RWMutex
	status Status
}

func NewManager(store *config.Store, log *logx.Logger) *Manager {
	return &Manager{store: store, log: log, runners: map[string]*runner{}}
}

func (m *Manager) SetSTUNIPResolver(f func(rule string) (string, bool)) { m.stunIP = f }

func ruleHash(r config.DDNSRule) string {
	raw, _ := json.Marshal(r)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

func (m *Manager) Reconcile(cfg *config.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	for _, rule := range cfg.DDNS {
		seen[rule.ID] = true
		h := ruleHash(rule)
		if cur, ok := m.runners[rule.ID]; ok {
			if !rule.Enable {
				cur.stop()
				delete(m.runners, rule.ID)
				continue
			}
			if cur.hash == h {
				continue
			}
			cur.stop()
			delete(m.runners, rule.ID)
		}
		if !rule.Enable {
			continue
		}
		r := &runner{cfg: rule, hash: h, done: make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		r.cancel = cancel
		m.runners[rule.ID] = r
		go m.loop(ctx, r)
	}
	for id, r := range m.runners {
		if !seen[id] {
			r.stop()
			delete(m.runners, id)
		}
	}
}

func (r *runner) stop() {
	if r.cancel != nil {
		r.cancel()
	}
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
	}
}

func (r *runner) setStatus(f func(*Status)) {
	r.mu.Lock()
	f(&r.status)
	r.mu.Unlock()
}

func (r *runner) getStatus() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

func (m *Manager) loop(ctx context.Context, r *runner) {
	defer close(r.done)
	interval := r.cfg.Interval.Duration()
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	// small startup stagger so many rules don't hit APIs at once
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Duration(time.Now().UnixNano() % int64(5*time.Second))):
	}
	for {
		m.runOnce(ctx, r)
		jitter := time.Duration(time.Now().UnixNano() % int64(interval/10+1))
		next := time.Now().Add(interval + jitter)
		r.setStatus(func(s *Status) { s.NextRun = next })
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
		}
	}
}

func (m *Manager) runOnce(ctx context.Context, r *runner) {
	lg := m.log.WithRule(r.cfg.Name)
	r.setStatus(func(s *Status) { s.LastRun = time.Now(); s.NextRun = time.Time{} })
	ip, err := m.resolveIP(ctx, r.cfg)
	if err != nil {
		r.setStatus(func(s *Status) { s.OK = false; s.LastError = err.Error() })
		lg.Error("获取 IP 失败: %v", err)
		return
	}
	prov, ok := provider.Get(r.cfg.Provider)
	if !ok {
		err := fmt.Errorf("不支持的 DNS 服务商: %s", r.cfg.Provider)
		r.setStatus(func(s *Status) { s.OK = false; s.LastError = err.Error() })
		lg.Error("%v", err)
		return
	}
	// Nothing changed since the last successful update: skip the API call.
	// Without this the panel logs "已更新" every interval and hits the provider
	// on every tick even when the address has been stable for days.
	if st := r.getStatus(); st.OK && st.LastIP == ip {
		r.setStatus(func(s *Status) { s.LastError = "" })
		return
	}
	actx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err = prov.Update(actx, provider.Request{
		Domain: r.cfg.Domain,
		Type:   r.cfg.Type,
		Value:  ip,
		TTL:    r.cfg.TTL,
		Auth:   r.cfg.Auth,
	})
	if err != nil {
		r.setStatus(func(s *Status) { s.OK = false; s.LastError = err.Error() })
		lg.Error("更新失败: %v", err)
		return
	}
	r.setStatus(func(s *Status) { s.OK = true; s.LastError = ""; s.LastIP = ip })
	lg.Info("已更新 %s %s -> %s", r.cfg.Type, r.cfg.Domain, ip)
}

func (m *Manager) resolveIP(ctx context.Context, r config.DDNSRule) (string, error) {
	src := strings.TrimSpace(r.Source)
	switch {
	case src == "" || src == "auto":
		if m.stunIP != nil {
			if ip, ok := m.stunIP(""); ok && ip != "" {
				return ip, nil
			}
		}
		if r.Type == "AAAA" {
			ip, err := netx.InterfaceIP("", true)
			if err != nil {
				return "", err
			}
			return ip.String(), nil
		}
		return netx.PublicIP(ctx, m.store.Get().Net.Proxy)
	case strings.HasPrefix(src, "interface:"):
		ip, err := netx.InterfaceIP(strings.TrimPrefix(src, "interface:"), r.Type == "AAAA")
		if err != nil {
			return "", err
		}
		return ip.String(), nil
	case strings.HasPrefix(src, "url:"):
		return fetchIP(ctx, strings.TrimPrefix(src, "url:"), m.store.Get().Net.Proxy)
	case strings.HasPrefix(src, "stun:"):
		key := strings.TrimPrefix(src, "stun:")
		if m.stunIP == nil {
			return "", fmt.Errorf("STUN 模块未就绪")
		}
		ip, ok := m.stunIP(key)
		if !ok || ip == "" {
			return "", fmt.Errorf("STUN 规则 %s 还没有公网地址", key)
		}
		return ip, nil
	default:
		if ip := net.ParseIP(src); ip != nil {
			return ip.String(), nil
		}
		return "", fmt.Errorf("无效的 IP 来源: %s", src)
	}
}

func fetchIP(ctx context.Context, u, proxy string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "net-bloop/1.0")
	client := netx.Client(proxy, 11*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("%s: %s", u, resp.Status)
	}
	for _, f := range strings.Fields(string(body)) {
		if ip := net.ParseIP(strings.Trim(f, "[](),;")); ip != nil {
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("%s: 响应中没有 IP", u)
}

func (m *Manager) RunNow(id string) {
	m.mu.Lock()
	r := m.runners[id]
	m.mu.Unlock()
	if r == nil {
		return
	}
	go m.runOnce(context.Background(), r)
}

func (m *Manager) Statuses() map[string]Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]Status, len(m.runners))
	for id, r := range m.runners {
		out[id] = r.getStatus()
	}
	return out
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, r := range m.runners {
		r.stop()
		delete(m.runners, id)
	}
}
