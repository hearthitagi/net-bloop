package stun

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"net-bloop/internal/config"
	"net-bloop/internal/logx"
	"net-bloop/internal/netx"
)

const (
	maxServers     = 20
	minServers     = 3
	serversFile    = "stun-servers.txt"
	ghProxyPrefix  = "https://gh-proxy.com/"
	fetchMaxBytes  = 1 << 18
	fetchTimeout   = 15 * time.Second
	retryOnStartup = 20 * time.Second
)

// Servers keeps the shared STUN server list fresh. The public list is a plain
// text file of host:port lines on GitHub, which is not directly reachable from
// most Chinese networks, so several mirrors are tried in order.
type Servers struct {
	store   *config.Store
	log     *logx.Logger
	dataDir string

	mu      sync.RWMutex
	list    []string
	source  string
	updated time.Time
	lastErr string
	fetching bool

	cancel context.CancelFunc
	done   chan struct{}
}

func NewServers(store *config.Store, log *logx.Logger, dataDir string) *Servers {
	s := &Servers{store: store, log: log, dataDir: dataDir}
	s.loadCache()
	return s
}

func (s *Servers) cachePath() string { return filepath.Join(s.dataDir, serversFile) }

func (s *Servers) loadCache() {
	b, err := os.ReadFile(s.cachePath())
	if err != nil {
		return
	}
	list := parseServerList(string(b))
	if len(list) >= minServers {
		s.mu.Lock()
		s.list = list
		s.source = "本地缓存"
		s.mu.Unlock()
	}
}

func (s *Servers) saveCache(list []string) error {
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return err
	}
	body := strings.Join(list, "\n") + "\n"
	return os.WriteFile(s.cachePath(), []byte(body), 0o644)
}

// Start loads the cache and begins the daily refresh loop.
func (s *Servers) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		cfg := s.store.Get()
		if cfg.STUNList.Enable {
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryOnStartup):
			}
			s.refresh(ctx, false)
		}
		for {
			cfg := s.store.Get()
			interval := cfg.STUNList.Interval.Duration()
			if interval < time.Hour {
				interval = 24 * time.Hour
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
			if s.store.Get().STUNList.Enable {
				s.refresh(ctx, false)
			}
		}
	}()
}

func (s *Servers) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.done != nil {
		select {
		case <-s.done:
		case <-time.After(2 * time.Second):
		}
	}
}

// For returns the merged list for a protocol: manual config entries win, then
// the fetched list, then the built-in defaults so the feature never goes dark.
func (s *Servers) For(proto string) []string {
	cfg := s.store.Get()
	out := make([]string, 0, maxServers+len(config.DefaultServers(proto)))
	seen := map[string]bool{}
	add := func(v string) {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" || seen[v] || len(out) >= maxServers {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, v := range cfg.STUNList.Servers {
		add(v)
	}
	s.mu.RLock()
	for _, v := range s.list {
		add(v)
	}
	s.mu.RUnlock()
	for _, v := range config.DefaultServers(proto) {
		if len(out) >= maxServers {
			break
		}
		add(v)
	}
	return out
}

// Status describes the last fetch, for the settings page.
func (s *Servers) Status() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{
		"count":    len(s.list),
		"source":   s.source,
		"updated":  s.updated,
		"error":    s.lastErr,
		"fetching": s.fetching,
		"builtin":  len(config.DefaultServers("tcp")),
	}
}

// Refresh triggers a fetch. When manual is true the caller is waiting, so it
// runs synchronously and returns the error.
func (s *Servers) Refresh(ctx context.Context, manual bool) error {
	return s.refresh(ctx, manual)
}

func (s *Servers) refresh(ctx context.Context, manual bool) error {
	s.mu.Lock()
	if s.fetching && !manual {
		s.mu.Unlock()
		return nil
	}
	s.fetching = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.fetching = false
		s.mu.Unlock()
	}()

	cfg := s.store.Get()
	proxy := cfg.Net.Proxy
	candidates := s.candidates(cfg.STUNList.URL)

	var lastErr error
	for _, u := range candidates {
		list, err := s.fetch(ctx, u, proxy)
		if err != nil {
			lastErr = err
			s.log.Debug("STUN 列表 %s 获取失败: %v", u, err)
			continue
		}
		if len(list) < minServers {
			lastErr = fmt.Errorf("%s 只返回 %d 条，少于 %d 条", u, len(list), minServers)
			continue
		}
		s.mu.Lock()
		s.list = list
		s.source = hostOf(u)
		s.updated = time.Now()
		s.lastErr = ""
		s.mu.Unlock()
		_ = s.saveCache(list)
		s.log.Info("STUN 服务器列表已更新：%d 条，来源 %s", len(list), hostOf(u))
		return nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的下载地址")
	}
	s.mu.Lock()
	s.lastErr = lastErr.Error()
	keep := len(s.list)
	s.mu.Unlock()
	if manual {
		s.log.Warn("STUN 列表更新失败: %v（继续使用原有 %d 条）", lastErr, keep)
	}
	return lastErr
}

func (s *Servers) candidates(custom string) []string {
	def := config.DefaultSTUNListURL
	out := make([]string, 0, 4)
	add := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" {
			return
		}
		for _, v := range out {
			if v == u {
				return
			}
		}
		out = append(out, u)
	}
	if custom != "" {
		add(custom)
	}
	add(ghProxyPrefix + def)
	add(def)
	return out
}

func (s *Servers) fetch(ctx context.Context, u, proxy string) ([]string, error) {
	fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(fctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "net-bloop/1.0")
	client := netx.Client(proxy, fetchTimeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBytes))
	if err != nil {
		return nil, err
	}
	list := parseServerList(string(body))
	if len(list) == 0 {
		return nil, fmt.Errorf("响应里没有合法的 host:port")
	}
	return list, nil
}

// parseServerList keeps only well-formed host:port lines, de-duplicated and
// capped, so a malformed mirror can never poison the list.
func parseServerList(text string) []string {
	out := make([]string, 0, maxServers)
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.IndexAny(line, " \t"); i > 0 {
			line = line[:i]
		}
		host, port, err := net.SplitHostPort(line)
		if err != nil {
			continue
		}
		host = strings.Trim(host, "[]")
		if host == "" || strings.ContainsAny(host, "/@") {
			continue
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			continue
		}
		key := strings.ToLower(line)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
		if len(out) >= maxServers {
			break
		}
	}
	return out
}

func hostOf(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	if i := strings.Index(u, "/"); i > 0 {
		return u[:i]
	}
	return u
}
