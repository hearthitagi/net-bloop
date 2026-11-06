package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// Duration is a time.Duration that (un)marshals as a TOML/JSON string like "5m".
type Duration time.Duration

func (d Duration) Duration() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(strings.TrimSpace(string(b)))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	return d.UnmarshalText([]byte(s))
}

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Net      NetConfig      `toml:"net"`
	Web      WebConfig      `toml:"web"`
	Cert     CertConfig     `toml:"cert"`
	STUNList STUNListConfig `toml:"stun_list"`
	DDNS     []DDNSRule     `toml:"ddns"`
	Proxy    []ProxyRule    `toml:"proxy"`
	STUN     []STUNRule     `toml:"stun"`
}

type ServerConfig struct {
	Listen            string `toml:"listen" json:"listen"`
	AdminUser         string `toml:"admin_user" json:"admin_user"`
	AdminPasswordHash string `toml:"admin_password_hash" json:"-"`
	LogLevel          string `toml:"log_level" json:"log_level"`
}

// NetConfig holds process-wide outbound settings.
type NetConfig struct {
	Proxy string `toml:"proxy" json:"proxy"`
}

// WebConfig describes the single reverse-proxy listener. Every service shares
// one port and is told apart by TLS SNI / Host header, so there is exactly one
// place to configure the port and one certificate to keep alive.
type WebConfig struct {
	Listen        string `toml:"listen" json:"listen"`
	HTTPS         bool   `toml:"https" json:"https"`
	RedirectHTTPS bool   `toml:"redirect_https" json:"redirect_https"`
	CertFile      string `toml:"cert_file" json:"cert_file"`
	KeyFile       string `toml:"key_file" json:"key_file"`
	AccessLog     bool   `toml:"access_log" json:"access_log"`
}

// CertConfig drives the built-in ACME client. DNS-01 is the only challenge
// type that can issue a wildcard certificate, which is what this deployment
// needs for every host under one domain.
type CertConfig struct {
	Enable      bool              `toml:"enable" json:"enable"`
	Provider    string            `toml:"provider" json:"provider"`
	Email       string            `toml:"email" json:"email"`
	Domains     []string          `toml:"domains" json:"domains"`
	Auth        map[string]string `toml:"auth" json:"auth"`
	RenewBefore Duration          `toml:"renew_before" json:"renew_before"`
	Directory   string            `toml:"directory" json:"directory"`
}

// STUNListConfig keeps the shared STUN server list fresh from a public list.
// Servers is a manual override: when set, it is used instead of the fetch.
type STUNListConfig struct {
	Enable   bool     `toml:"enable" json:"enable"`
	URL      string   `toml:"url" json:"url"`
	Interval Duration `toml:"interval" json:"interval"`
	Servers  []string `toml:"servers" json:"servers"`
}

type DDNSRule struct {
	ID       string            `toml:"id" json:"id"`
	Name     string            `toml:"name" json:"name"`
	Enable   bool              `toml:"enable" json:"enable"`
	Provider string            `toml:"provider" json:"provider"`
	Domain   string            `toml:"domain" json:"domain"`
	Type     string            `toml:"type" json:"type"`
	Source   string            `toml:"source" json:"source"`
	Interval Duration          `toml:"interval" json:"interval"`
	TTL      int               `toml:"ttl" json:"ttl"`
	Auth     map[string]string `toml:"auth" json:"auth"`
}

// ProxyRule is one host-based route. It no longer carries a listen address or
// certificate: those are global now.
type ProxyRule struct {
	ID            string   `toml:"id" json:"id"`
	Name          string   `toml:"name" json:"name"`
	Enable        bool     `toml:"enable" json:"enable"`
	Domains       []string `toml:"domains" json:"domains"`
	Upstreams     []string `toml:"upstreams" json:"upstreams"`
	UseTargetHost bool     `toml:"use_target_host" json:"use_target_host"`
	BasicAuth     string   `toml:"basic_auth" json:"basic_auth"`
}

// STUN rule modes.
const (
	// ModeForward means the router already forwards a fixed public port to the
	// local service, so the rule only observes the public address.
	ModeForward = "forward"
	// ModePunch means the rule keeps a NAT pinhole open and relays traffic to
	// Target, for setups without a static port forward.
	ModePunch = "punch"
)

type STUNRule struct {
	ID         string   `toml:"id" json:"id"`
	Name       string   `toml:"name" json:"name"`
	Enable     bool     `toml:"enable" json:"enable"`
	Proto      string   `toml:"proto" json:"proto"`
	Mode       string   `toml:"mode" json:"mode"`
	PublicPort int      `toml:"public_port" json:"public_port"`
	LocalPort  int      `toml:"local_port" json:"local_port"`
	ListenIP   string   `toml:"listen_ip" json:"listen_ip"`
	Target     string   `toml:"target" json:"target"`
	Servers    []string `toml:"servers" json:"servers"`
	Heartbeat  Duration `toml:"heartbeat" json:"heartbeat"`
	Webhook    *Webhook `toml:"webhook" json:"webhook,omitempty"`
}

type Webhook struct {
	Enable     bool              `toml:"enable" json:"enable"`
	URL        string            `toml:"url" json:"url"`
	Method     string            `toml:"method" json:"method"`
	Headers    map[string]string `toml:"headers" json:"headers"`
	Body       string            `toml:"body" json:"body"`
	OnlyChange bool              `toml:"only_change" json:"only_change"`
	Timeout    Duration          `toml:"timeout" json:"timeout"`
}

const (
	DefaultListen    = ":16601"
	DefaultWebListen = ":33320"
	DefaultAdminUser = "admin"
	DefaultLogLevel  = "info"

	DefaultSTUNServer      = "stun.miwifi.com:3478"
	DefaultSTUNListURL     = "https://raw.githubusercontent.com/pradt2/always-online-stun/master/valid_hosts_tcp.txt"
	DefaultACMEDirectory   = "https://acme-v02.api.letsencrypt.org/directory"
	DefaultCertRenewBefore = 30 * 24 * time.Hour
)

var DefaultSTUNServers = []string{
	"stun.miwifi.com:3478",
	"stun.cloudflare.com:3478",
	"stun.l.google.com:19302",
}

var DefaultTCPSTUNServers = []string{
	"stun.nextcloud.com:3478",
	"stun.antisip.com:3478",
	"stun.voip.blackberry.com:3478",
}

// DefaultServers returns sensible STUN servers for the protocol. Many public
// servers only answer over UDP, so TCP gets its own list.
func DefaultServers(proto string) []string {
	if strings.EqualFold(proto, "tcp") {
		return append([]string(nil), DefaultTCPSTUNServers...)
	}
	return append([]string(nil), DefaultSTUNServers...)
}

func NewID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// NormalizeListen accepts "33320", ":33320" or "0.0.0.0:33320" and returns a
// net.Listen style address. The UI only ever shows the bare port, but nothing
// breaks if a hand-edited config keeps the prefix.
func NormalizeListen(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, ":") {
		return v
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n < 1 || n > 65535 {
			return v
		}
		return ":" + v
	}
	return v
}

// PortOf returns just the port part of a listen address, for display.
func PortOf(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if idx := strings.LastIndex(v, ":"); idx >= 0 {
		return v[idx+1:]
	}
	return v
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:   DefaultListen,
			LogLevel: DefaultLogLevel,
		},
		Web: WebConfig{
			Listen:        DefaultWebListen,
			HTTPS:         true,
			RedirectHTTPS: true,
		},
		Cert: CertConfig{
			Provider:    "cloudflare",
			RenewBefore: Duration(DefaultCertRenewBefore),
		},
		STUNList: STUNListConfig{
			Enable:   true,
			Interval: Duration(24 * time.Hour),
		},
	}
}

func (c *Config) Normalize() {
	c.Server.Listen = NormalizeListen(c.Server.Listen)
	if c.Server.Listen == "" {
		c.Server.Listen = DefaultListen
	}
	if c.Server.AdminUser == "" {
		c.Server.AdminUser = DefaultAdminUser
	}
	if c.Server.LogLevel == "" {
		c.Server.LogLevel = DefaultLogLevel
	}

	c.Net.Proxy = strings.TrimSpace(c.Net.Proxy)

	c.Web.Listen = NormalizeListen(c.Web.Listen)
	if c.Web.Listen == "" {
		c.Web.Listen = DefaultWebListen
	}
	c.Web.CertFile = strings.TrimSpace(c.Web.CertFile)
	c.Web.KeyFile = strings.TrimSpace(c.Web.KeyFile)

	c.Cert.Provider = strings.ToLower(strings.TrimSpace(c.Cert.Provider))
	if c.Cert.Provider == "" {
		c.Cert.Provider = "cloudflare"
	}
	c.Cert.Email = strings.TrimSpace(c.Cert.Email)
	c.Cert.Domains = cleanList(c.Cert.Domains, true)
	c.Cert.Directory = strings.TrimSpace(c.Cert.Directory)
	if c.Cert.RenewBefore <= 0 {
		c.Cert.RenewBefore = Duration(DefaultCertRenewBefore)
	}
	if c.Cert.Auth == nil {
		c.Cert.Auth = map[string]string{}
	}

	c.STUNList.URL = strings.TrimSpace(c.STUNList.URL)
	if c.STUNList.Interval <= 0 {
		c.STUNList.Interval = Duration(24 * time.Hour)
	}
	c.STUNList.Servers = cleanList(c.STUNList.Servers, false)

	for i := range c.DDNS {
		r := &c.DDNS[i]
		if r.ID == "" {
			r.ID = NewID()
		}
		r.Name = strings.TrimSpace(r.Name)
		r.Provider = strings.TrimSpace(r.Provider)
		// Users paste domains with stray spaces; trim before validation so a
		// rule can never silently point at " example.com ".
		r.Domain = strings.TrimSpace(r.Domain)
		r.Type = strings.ToUpper(strings.TrimSpace(r.Type))
		if r.Type == "" {
			r.Type = "A"
		}
		if r.Source == "" {
			r.Source = "auto"
		}
		if r.Interval <= 0 {
			r.Interval = Duration(10 * time.Minute)
		}
		if r.TTL <= 0 {
			r.TTL = 600
		}
		if r.Auth == nil {
			r.Auth = map[string]string{}
		}
	}
	for i := range c.Proxy {
		r := &c.Proxy[i]
		if r.ID == "" {
			r.ID = NewID()
		}
		r.Name = strings.TrimSpace(r.Name)
		r.Domains = cleanList(r.Domains, true)
		r.Upstreams = cleanList(r.Upstreams, false)
	}
	for i := range c.STUN {
		r := &c.STUN[i]
		if r.ID == "" {
			r.ID = NewID()
		}
		r.Name = strings.TrimSpace(r.Name)
		r.Proto = strings.ToLower(strings.TrimSpace(r.Proto))
		r.Mode = strings.ToLower(strings.TrimSpace(r.Mode))
		r.Target = strings.TrimSpace(r.Target)
		if r.Proto == "" {
			r.Proto = "tcp"
		}
		if r.Mode != ModePunch {
			r.Mode = ModeForward
		}
		if r.ListenIP == "" {
			r.ListenIP = "0.0.0.0"
		}
		if r.Heartbeat <= 0 {
			r.Heartbeat = Duration(20 * time.Second)
		}
		r.Servers = cleanList(r.Servers, false)
		if r.Webhook != nil {
			if r.Webhook.Method == "" {
				r.Webhook.Method = "POST"
			}
			if r.Webhook.Timeout <= 0 {
				r.Webhook.Timeout = Duration(10 * time.Second)
			}
		}
	}
}

// cleanList drops blank entries and trims surrounding spaces. Domains are also
// lowercased because host matching is case-insensitive.
func cleanList(in []string, lower bool) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if lower {
			s = strings.ToLower(s)
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *Config) Validate() error {
	seen := map[string]string{}
	check := func(kind, id, name string) error {
		if id == "" {
			return fmt.Errorf("%s %q 缺少 id", kind, name)
		}
		if prev, ok := seen[kind+":"+id]; ok {
			return fmt.Errorf("%s id 重复: %s / %s", kind, prev, name)
		}
		seen[kind+":"+id] = name
		return nil
	}
	if c.Web.Listen == c.Server.Listen {
		return fmt.Errorf("Web 服务端口与管理端口不能相同")
	}
	if c.Cert.Enable && len(c.Cert.Domains) == 0 {
		return fmt.Errorf("启用自动证书需要至少一个域名")
	}
	for _, r := range c.DDNS {
		if err := check("ddns", r.ID, r.Name); err != nil {
			return err
		}
		if r.Domain == "" {
			return fmt.Errorf("DDNS %q 缺少域名", r.Name)
		}
	}
	for _, r := range c.Proxy {
		if err := check("proxy", r.ID, r.Name); err != nil {
			return err
		}
		if len(r.Upstreams) == 0 {
			return fmt.Errorf("Web 服务 %q 缺少上游地址", r.Name)
		}
	}
	for _, r := range c.STUN {
		if err := check("stun", r.ID, r.Name); err != nil {
			return err
		}
		if r.Mode != ModePunch {
			continue
		}
		if r.Target == "" {
			return fmt.Errorf("STUN %q 处于打洞模式，缺少转发目标", r.Name)
		}
		if r.LocalPort < 1 || r.LocalPort > 65535 {
			return fmt.Errorf("STUN %q 本地监听端口无效", r.Name)
		}
		key := strings.ToLower(r.Proto) + ":" + fmt.Sprint(r.LocalPort)
		if prev, ok := seen["port:"+key]; ok {
			return fmt.Errorf("STUN 规则 %q 与 %q 占用了同一个 %s 端口 %d", r.Name, prev, strings.ToUpper(r.Proto), r.LocalPort)
		}
		seen["port:"+key] = r.Name
	}
	return nil
}

func Load(path string) (*Config, error) {
	c := Default()
	if _, err := os.Stat(path); errorsIsNotExist(err) {
		c.Normalize()
		return c, nil
	}
	if _, err := toml.DecodeFile(path, c); err != nil {
		return nil, err
	}
	c.Normalize()
	return c, nil
}

func errorsIsNotExist(err error) bool { return err != nil && os.IsNotExist(err) }

func Save(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	if err := toml.NewEncoder(&b).Encode(c); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Clone returns a deep copy of the configuration.
func (c *Config) Clone() *Config {
	next := *c
	next.Cert.Domains = append([]string(nil), c.Cert.Domains...)
	if c.Cert.Auth != nil {
		next.Cert.Auth = maps.Clone(c.Cert.Auth)
	}
	next.STUNList.Servers = append([]string(nil), c.STUNList.Servers...)

	next.DDNS = make([]DDNSRule, len(c.DDNS))
	copy(next.DDNS, c.DDNS)
	for i := range next.DDNS {
		if c.DDNS[i].Auth != nil {
			next.DDNS[i].Auth = maps.Clone(c.DDNS[i].Auth)
		}
	}
	next.Proxy = make([]ProxyRule, len(c.Proxy))
	copy(next.Proxy, c.Proxy)
	for i := range next.Proxy {
		next.Proxy[i].Domains = append([]string(nil), c.Proxy[i].Domains...)
		next.Proxy[i].Upstreams = append([]string(nil), c.Proxy[i].Upstreams...)
	}
	next.STUN = make([]STUNRule, len(c.STUN))
	copy(next.STUN, c.STUN)
	for i := range next.STUN {
		next.STUN[i].Servers = append([]string(nil), c.STUN[i].Servers...)
		if c.STUN[i].Webhook != nil {
			wh := *c.STUN[i].Webhook
			if wh.Headers != nil {
				wh.Headers = maps.Clone(wh.Headers)
			}
			next.STUN[i].Webhook = &wh
		}
	}
	return &next
}

// Store holds the live configuration and notifies subscribers on change.
type Store struct {
	mu      sync.RWMutex
	cfg     *Config
	path    string
	onApply func(*Config)
}

func NewStore(path string, c *Config) *Store {
	return &Store{cfg: c, path: path}
}

func (s *Store) Path() string { return s.path }

func (s *Store) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Snapshot returns a deep copy safe for concurrent use.
func (s *Store) Snapshot() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Clone()
}

func (s *Store) SetOnApply(f func(*Config)) { s.onApply = f }

// Replace swaps the whole configuration, validates and persists it.
func (s *Store) Replace(next *Config) error {
	s.mu.Lock()
	next.Normalize()
	if err := next.Validate(); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := Save(s.path, next); err != nil {
		s.mu.Unlock()
		return err
	}
	s.cfg = next
	cb := s.onApply
	s.mu.Unlock()
	if cb != nil {
		cb(next)
	}
	return nil
}

func (s *Store) Apply(mutate func(*Config) error) error {
	s.mu.Lock()
	next := s.cfg.Clone()
	if err := mutate(next); err != nil {
		s.mu.Unlock()
		return err
	}
	next.Normalize()
	if err := next.Validate(); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := Save(s.path, next); err != nil {
		s.mu.Unlock()
		return err
	}
	s.cfg = next
	cb := s.onApply
	s.mu.Unlock()
	if cb != nil {
		cb(next)
	}
	return nil
}
