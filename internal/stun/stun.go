package stun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"net-bloop/internal/config"
	"net-bloop/internal/logx"
)

// Status is the live state of one STUN rule.
type Status struct {
	ID            string    `json:"id"`
	State         string    `json:"state"` // starting | probing | punching | active | error | stopped
	Mode          string    `json:"mode"`
	Proto         string    `json:"proto"`
	Target        string    `json:"target"`
	PublicIP      string    `json:"public_ip"`
	PublicPort    int       `json:"public_port"`
	ObservedPort  int       `json:"observed_port"`
	Access        string    `json:"access"`
	LastError     string    `json:"last_error"`
	LastChange    time.Time `json:"last_change"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	Heartbeats    uint64    `json:"heartbeats"`
	HookFires     uint64    `json:"hook_fires"`
}

type Manager struct {
	store *config.Store
	log   *logx.Logger
	mu    sync.Mutex
	runs  map[string]*runtime
	// list returns the shared STUN server list maintained by the updater.
	list func(proto string) []string
}

type runtime struct {
	rule  config.STUNRule
	hash  string
	log   *logx.Logger
	list  func(proto string) []string
	proxy func() string

	cancel context.CancelFunc
	done   chan struct{}

	// STUN server stickiness. On a symmetric NAT the mapping key includes the
	// destination, so asking a different server hands out a different public
	// port. Rotating through the server list every heartbeat therefore made the
	// reported address jump around every 30 seconds. Once a server answers we
	// keep using it until it fails repeatedly.
	picked string
	rotate int
	fails  int

	mu sync.RWMutex
	st Status
}

// pickServer returns the sticky server, or the next candidate when none has
// answered yet.
func (rt *runtime) pickServer(servers []string) string {
	if rt.picked != "" {
		for _, s := range servers {
			if s == rt.picked {
				return s
			}
		}
		rt.picked = ""
	}
	return servers[rt.rotate%len(servers)]
}

// noteServerResult keeps the sticky choice stable, only giving it up after
// serverFailLimit consecutive failures.
func (rt *runtime) noteServerResult(servers []string, server string, err error) {
	if err == nil {
		rt.picked = server
		rt.fails = 0
		return
	}
	rt.fails++
	if rt.picked != "" && rt.fails >= serverFailLimit {
		for i, s := range servers {
			if s == rt.picked {
				rt.rotate = i + 1
				break
			}
		}
		rt.picked = ""
		rt.fails = 0
	}
}

// serverFailLimit is how many consecutive failures a sticky server is allowed
// before we move on to the next candidate.
const serverFailLimit = 3

// Keepalive pacing. A TCP NAT binding normally survives far longer than a
// minute, so a heartbeat faster than this only loads the public STUN server
// without buying anything: one rule at 5s is ~17k requests a day against a
// machine nobody is paying us to use. 20s keeps the pinhole alive and costs
// about a tenth of that.
const (
	minKeepalive     = 10 * time.Second
	defaultKeepalive = 20 * time.Second
)

// keepaliveInterval clamps the configured heartbeat into a polite range.
func (rt *runtime) keepaliveInterval() time.Duration {
	d := rt.rule.Heartbeat.Duration()
	if d < minKeepalive {
		return minKeepalive
	}
	return d
}

// proxyAddr returns the configured outbound proxy, if any.
func (rt *runtime) proxyAddr() string {
	if rt.proxy == nil {
		return ""
	}
	return rt.proxy()
}

func NewManager(store *config.Store, log *logx.Logger) *Manager {
	return &Manager{store: store, log: log, runs: map[string]*runtime{}}
}

// SetServerSource wires the shared, auto-updated server list into the manager.
func (m *Manager) SetServerSource(f func(proto string) []string) { m.list = f }

func ruleHash(r config.STUNRule) string {
	raw, _ := json.Marshal(r)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

func (m *Manager) Reconcile(cfg *config.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	for _, rule := range cfg.STUN {
		seen[rule.ID] = true
		h := ruleHash(rule)
		if cur, ok := m.runs[rule.ID]; ok {
			if !rule.Enable {
				cur.stop()
				delete(m.runs, rule.ID)
				continue
			}
			if cur.hash == h {
				continue
			}
			cur.stop()
			delete(m.runs, rule.ID)
		}
		if !rule.Enable {
			continue
		}
		rt := &runtime{
			rule: rule,
			hash: h,
			log:  m.log,
			list: m.list,
			proxy: func() string {
				return m.store.Get().Net.Proxy
			},
			done: make(chan struct{}),
			st: Status{
				ID: rule.ID, State: "starting", Mode: rule.Mode,
				Proto: rule.Proto, Target: rule.Target,
			},
		}
		ctx, cancel := context.WithCancel(context.Background())
		rt.cancel = cancel
		m.runs[rule.ID] = rt
		go rt.loop(ctx)
	}
	for id, rt := range m.runs {
		if !seen[id] {
			rt.stop()
			delete(m.runs, id)
		}
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, rt := range m.runs {
		rt.stop()
		delete(m.runs, id)
	}
}

func (m *Manager) Statuses() map[string]Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]Status, len(m.runs))
	for id, rt := range m.runs {
		out[id] = rt.snapshot()
	}
	return out
}

// PublicAddr returns the current public address for DDNS rules. An empty id
// picks the first active rule with a usable address.
func (m *Manager) PublicAddr(id string) (string, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id != "" {
		rt, ok := m.runs[id]
		if !ok {
			return "", 0, false
		}
		st := rt.snapshot()
		if st.PublicIP == "" || st.PublicPort == 0 {
			return "", 0, false
		}
		return st.PublicIP, st.PublicPort, true
	}
	for _, rt := range m.runs {
		st := rt.snapshot()
		if st.PublicIP != "" && st.PublicPort != 0 {
			return st.PublicIP, st.PublicPort, true
		}
	}
	return "", 0, false
}

// PublicIP implements the DDNS resolver signature.
func (m *Manager) PublicIP(id string) (string, bool) {
	ip, _, ok := m.PublicAddr(id)
	return ip, ok
}

func (rt *runtime) stop() {
	if rt.cancel != nil {
		rt.cancel()
	}
	select {
	case <-rt.done:
	case <-time.After(6 * time.Second):
	}
}

func (rt *runtime) snapshot() Status {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.st
}

func (rt *runtime) setState(state string) {
	rt.mu.Lock()
	rt.st.State = state
	rt.mu.Unlock()
}

func (rt *runtime) setError(err error) {
	rt.mu.Lock()
	if err == nil {
		rt.st.LastError = ""
	} else {
		rt.st.LastError = err.Error()
	}
	rt.mu.Unlock()
}

func (rt *runtime) noteHeartbeat() {
	rt.mu.Lock()
	rt.st.LastHeartbeat = time.Now()
	rt.st.Heartbeats++
	rt.mu.Unlock()
}

// report records the public address and fires the webhook when it changes.
// observed is the port the STUN server saw; in forward mode it is only
// informational because the router owns the real public port.
func (rt *runtime) report(ip string, observed int, note string) {
	if ip == "" {
		return
	}
	port := rt.rule.PublicPort
	if port <= 0 {
		port = observed
	}
	if port <= 0 {
		return
	}
	rt.mu.Lock()
	oldIP, oldPort := rt.st.PublicIP, rt.st.PublicPort
	changed := oldIP != ip || oldPort != port
	rt.st.PublicIP = ip
	rt.st.PublicPort = port
	rt.st.ObservedPort = observed
	rt.st.Access = joinAddr(ip, port)
	rt.st.LastError = ""
	rt.st.State = "active"
	if changed {
		rt.st.LastChange = time.Now()
	}
	rt.mu.Unlock()

	if !changed {
		return
	}
	lg := rt.log.WithRule(rt.rule.Name)
	if oldIP == "" {
		lg.Info("公网地址 %s（%s）", joinAddr(ip, port), note)
	} else {
		lg.Info("公网地址变化: %s -> %s", joinAddr(oldIP, oldPort), joinAddr(ip, port))
	}
	vars := newHookVars(rt.rule, ip, port, oldIP, oldPort)
	if w := rt.rule.Webhook; w != nil && w.Enable {
		go rt.fireWebhook(context.Background(), vars)
		rt.mu.Lock()
		rt.st.HookFires++
		rt.mu.Unlock()
	}
}

func (rt *runtime) loop(ctx context.Context) {
	defer close(rt.done)
	for {
		if ctx.Err() != nil {
			rt.setState("stopped")
			return
		}
		err := rt.serve(ctx)
		if ctx.Err() != nil {
			rt.setState("stopped")
			return
		}
		rt.setError(err)
		rt.setState("error")
		if err != nil {
			rt.log.WithRule(rt.rule.Name).Error("运行中断: %v，20 秒后重试", err)
		}
		select {
		case <-ctx.Done():
			rt.setState("stopped")
			return
		case <-time.After(20 * time.Second):
		}
	}
}

// serve runs the rule. Forward mode only observes the public address; punch
// mode additionally keeps a listener open and relays traffic to the target.
func (rt *runtime) serve(ctx context.Context) error {
	if rt.rule.Mode == config.ModePunch {
		rt.setState("punching")
		if strings.EqualFold(rt.rule.Proto, "udp") {
			return rt.serveUDP(ctx)
		}
		return rt.serveTCP(ctx)
	}
	rt.setState("probing")
	return rt.probeLoop(ctx)
}

// probeLoop periodically asks a STUN server what address the outside world
// sees. It opens no listening socket, so it can never conflict with the port
// the router forwards.
func (rt *runtime) probeLoop(ctx context.Context) error {
	lg := rt.log.WithRule(rt.rule.Name)
	interval := rt.rule.Heartbeat.Duration()
	if interval < 10*time.Second {
		interval = 30 * time.Second
	}
	servers := rt.stunServers()
	if len(servers) == 0 {
		return fmt.Errorf("没有可用的 STUN 服务器")
	}
	for {
		server := rt.pickServer(servers)
		pctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		ip, port, err := probeOnce(pctx, server, rt.rule.Proto)
		cancel()
		if ctx.Err() != nil {
			return nil
		}
		rt.noteServerResult(servers, server, err)
		if err != nil {
			lg.Warn("STUN(%s) 探测失败: %v", server, err)
			rt.setError(err)
			rt.setState("error")
		} else {
			rt.noteHeartbeat()
			rt.report(ip, port, "固定转发")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

// stunServers merges the per-rule override with the shared list.
func (rt *runtime) stunServers() []string {
	out := make([]string, 0, 8)
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimPrefix(strings.TrimSpace(s), "stun:")
		s = strings.TrimSpace(s)
		if s == "" || seen[strings.ToLower(s)] {
			return
		}
		seen[strings.ToLower(s)] = true
		out = append(out, s)
	}
	for _, s := range rt.rule.Servers {
		add(s)
	}
	if len(out) == 0 && rt.list != nil {
		for _, s := range rt.list(rt.rule.Proto) {
			add(s)
		}
	}
	if len(out) == 0 {
		for _, s := range config.DefaultServers(rt.rule.Proto) {
			add(s)
		}
	}
	return out
}

func (rt *runtime) listenAddr() string {
	ip := strings.TrimSpace(rt.rule.ListenIP)
	if ip == "" {
		ip = "0.0.0.0"
	}
	return net.JoinHostPort(ip, fmt.Sprint(rt.rule.LocalPort))
}

// wantIPv6 reports whether the punch listener is bound to IPv6.
func (rt *runtime) wantIPv6() bool {
	ip := strings.TrimSpace(rt.rule.ListenIP)
	if ip == "" {
		return false
	}
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() == nil
}

func serverAddr(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "stun:")
	if _, _, err := net.SplitHostPort(s); err == nil {
		return s
	}
	return net.JoinHostPort(s, "3478")
}

func joinAddr(ip string, port int) string {
	if ip == "" || port <= 0 {
		return ""
	}
	if strings.Contains(ip, ":") {
		return fmt.Sprintf("[%s]:%d", ip, port)
	}
	return fmt.Sprintf("%s:%d", ip, port)
}

func familyLabel(ipv6 bool) string {
	if ipv6 {
		return "IPv6"
	}
	return "IPv4"
}
