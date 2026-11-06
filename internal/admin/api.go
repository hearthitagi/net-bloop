package admin

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"net-bloop/internal/config"
	"net-bloop/internal/ddns/provider"
	stunmgr "net-bloop/internal/stun"
)

// ---------------------------------------------------------------- sessions

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if s.store.Get().Server.AdminPasswordHash != "" {
		writeErr(w, http.StatusConflict, "管理员已存在")
		return
	}
	user := strings.TrimSpace(in.Username)
	if user == "" {
		writeErr(w, http.StatusBadRequest, "请填写用户名")
		return
	}
	if len(in.Password) < 4 {
		writeErr(w, http.StatusBadRequest, "密码至少 4 位")
		return
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "生成密码失败")
		return
	}
	if err := s.store.Apply(func(c *config.Config) error {
		c.Server.AdminUser = user
		c.Server.AdminPasswordHash = hash
		return nil
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Info("管理员账号已创建: %s", user)
	s.setSession(w, r, user)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) setSession(w http.ResponseWriter, r *http.Request, user string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token(s.secret(), user),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		MaxAge:   7 * 24 * 3600,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	cfg := s.store.Get()
	if cfg.Server.AdminPasswordHash == "" {
		writeErr(w, http.StatusPreconditionRequired, "还没有创建管理员账号")
		return
	}
	ip := clientIP(r)
	if ok, wait := s.auth.allowed(ip); !ok {
		writeErr(w, http.StatusTooManyRequests, fmt.Sprintf("尝试过于频繁，请 %s 后再试", wait))
		return
	}
	if in.Username != cfg.Server.AdminUser || !VerifyPassword(cfg.Server.AdminPasswordHash, in.Password) {
		s.auth.fail(ip)
		writeErr(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	s.auth.reset(ip)
	s.setSession(w, r, cfg.Server.AdminUser)
	writeJSON(w, http.StatusOK, map[string]any{"user": cfg.Server.AdminUser})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	cfg := s.store.Get()
	setup := cfg.Server.AdminPasswordHash == ""
	resp := map[string]any{
		"authenticated":  false,
		"setup_required": setup,
		"user":           cfg.Server.AdminUser,
		"version":        s.version,
	}
	if c, err := r.Cookie(sessionCookie); err == nil && !setup {
		if user, ok := validToken(s.secret(), c.Value); ok && user == cfg.Server.AdminUser {
			resp["authenticated"] = true
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ------------------------------------------------------------------- state

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	cfg := s.store.Get()
	ddnsStatus := s.ddns.Statuses()
	stunStatus := s.stun.Statuses()
	web := s.proxy.State()
	certStatus := s.cert.Status()

	ddnsRules := make([]map[string]any, 0, len(cfg.DDNS))
	active, total := 0, 0
	for _, rule := range cfg.DDNS {
		st, ok := ddnsStatus[rule.ID]
		ddnsRules = append(ddnsRules, map[string]any{"rule": rule, "status": st, "has_status": ok})
		if rule.Enable {
			total++
			if ok && st.OK {
				active++
			}
		}
	}
	proxyRules := make([]map[string]any, 0, len(cfg.Proxy))
	for _, rule := range cfg.Proxy {
		proxyRules = append(proxyRules, map[string]any{"rule": rule, "active": web.Running && rule.Enable})
		if rule.Enable {
			total++
			if web.Running {
				active++
			}
		}
	}
	stunRules := make([]map[string]any, 0, len(cfg.STUN))
	for _, rule := range cfg.STUN {
		st, ok := stunStatus[rule.ID]
		stunRules = append(stunRules, map[string]any{"rule": rule, "status": st, "has_status": ok})
		if rule.Enable {
			total++
			if ok && st.State == "active" {
				active++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version": s.version,
		"uptime":  int64(time.Since(s.started).Seconds()),
		"host": map[string]any{
			"os":     runtime.GOOS,
			"arch":   runtime.GOARCH,
			"ips":    localIPs(),
			"config": s.store.Path(),
			"port":   config.PortOf(cfg.Server.Listen),
		},
		"counters": map[string]any{"active": active, "total": total},
		"web":      web,
		"cert":     certStatus,
		"stunlist": s.servers.Status(),
		"ddns":     ddnsRules,
		"proxy":    proxyRules,
		"stun":     stunRules,
	})
}

func localIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipn.IP.To4(); ip4 != nil && ip4.IsGlobalUnicast() {
				out = append(out, ip4.String())
			}
		}
	}
	return out
}

// -------------------------------------------------------------------- DDNS

func (s *Server) handleDDNSList(w http.ResponseWriter, r *http.Request) {
	cfg := s.store.Get()
	writeJSON(w, http.StatusOK, map[string]any{"rules": cfg.DDNS, "status": s.ddns.Statuses(), "providers": provider.Infos()})
}

func (s *Server) handleDDNSProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, provider.Infos())
}

func (s *Server) handleDdnsCreate(w http.ResponseWriter, r *http.Request) {
	var rule config.DDNSRule
	if !readJSON(w, r, &rule) {
		return
	}
	rule.ID = config.NewID()
	if err := s.store.Apply(func(c *config.Config) error {
		c.DDNS = append(c.DDNS, rule)
		return nil
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.ddns.RunNow(rule.ID)
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleDdnsUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var rule config.DDNSRule
	if !readJSON(w, r, &rule) {
		return
	}
	rule.ID = id
	if err := s.store.Apply(func(c *config.Config) error {
		for i := range c.DDNS {
			if c.DDNS[i].ID == id {
				c.DDNS[i] = rule
				return nil
			}
		}
		return fmt.Errorf("规则不存在")
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.ddns.RunNow(id)
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleDdnsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.Apply(func(c *config.Config) error {
		for i := range c.DDNS {
			if c.DDNS[i].ID == id {
				c.DDNS = append(c.DDNS[:i], c.DDNS[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("规则不存在")
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDdnsRun(w http.ResponseWriter, r *http.Request) {
	s.ddns.RunNow(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ------------------------------------------------------------------- proxy

func (s *Server) handleProxyList(w http.ResponseWriter, r *http.Request) {
	cfg := s.store.Get()
	writeJSON(w, http.StatusOK, map[string]any{"rules": cfg.Proxy, "web": s.proxy.State()})
}

func (s *Server) handleProxyCreate(w http.ResponseWriter, r *http.Request) {
	var rule config.ProxyRule
	if !readJSON(w, r, &rule) {
		return
	}
	rule.ID = config.NewID()
	if err := s.store.Apply(func(c *config.Config) error {
		c.Proxy = append(c.Proxy, rule)
		return nil
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleProxyUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var rule config.ProxyRule
	if !readJSON(w, r, &rule) {
		return
	}
	rule.ID = id
	if err := s.store.Apply(func(c *config.Config) error {
		for i := range c.Proxy {
			if c.Proxy[i].ID == id {
				c.Proxy[i] = rule
				return nil
			}
		}
		return fmt.Errorf("规则不存在")
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleProxyDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.Apply(func(c *config.Config) error {
		for i := range c.Proxy {
			if c.Proxy[i].ID == id {
				c.Proxy = append(c.Proxy[:i], c.Proxy[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("规则不存在")
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// -------------------------------------------------------------------- STUN

func (s *Server) handleSTUNList(w http.ResponseWriter, r *http.Request) {
	cfg := s.store.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"rules":   cfg.STUN,
		"status":  s.stun.Statuses(),
		"servers": s.servers.Status(),
	})
}

func (s *Server) handleSTUNCreate(w http.ResponseWriter, r *http.Request) {
	var rule config.STUNRule
	if !readJSON(w, r, &rule) {
		return
	}
	rule.ID = config.NewID()
	if err := s.store.Apply(func(c *config.Config) error {
		c.STUN = append(c.STUN, rule)
		return nil
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleSTUNUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var rule config.STUNRule
	if !readJSON(w, r, &rule) {
		return
	}
	rule.ID = id
	if err := s.store.Apply(func(c *config.Config) error {
		for i := range c.STUN {
			if c.STUN[i].ID == id {
				c.STUN[i] = rule
				return nil
			}
		}
		return fmt.Errorf("规则不存在")
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleSTUNDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.Apply(func(c *config.Config) error {
		for i := range c.STUN {
			if c.STUN[i].ID == id {
				c.STUN = append(c.STUN[:i], c.STUN[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("规则不存在")
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSTUNProbe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Servers []string `json:"servers"`
	}
	_ = readJSONOptional(w, r, &in)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	res, err := stunmgr.Probe(ctx, in.Servers)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleSTUNServers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"list":   s.servers.For("tcp"),
		"status": s.servers.Status(),
	})
}

func (s *Server) handleSTUNServersRefresh(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	if err := s.servers.Refresh(ctx, true); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"list":   s.servers.For("tcp"),
		"status": s.servers.Status(),
	})
}

func readJSONOptional(w http.ResponseWriter, r *http.Request, v any) error {
	raw, err := readAllLimited(r)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return jsonUnmarshal(raw, v)
}

// -------------------------------------------------------------------- logs

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	entries := s.hub.List(r.URL.Query().Get("module"), r.URL.Query().Get("rule"), limit)
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "不支持流式输出")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	for _, e := range s.hub.List("", "", 100) {
		writeSSE(w, "log", e)
	}
	flusher.Flush()

	ch, cancel := s.hub.Subscribe()
	defer cancel()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			writeSSE(w, "log", e)
			flusher.Flush()
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, event string, v any) {
	raw, err := jsonMarshal(v)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
}

// ----------------------------------------------------------------- settings

type settingsInput struct {
	AdminUser string `json:"admin_user"`
	Password  string `json:"password"`
	LogLevel  string `json:"log_level"`
	ServerPort string `json:"server_port"`

	WebPort          string `json:"web_port"`
	WebHTTPS         bool   `json:"web_https"`
	WebRedirectHTTPS bool   `json:"web_redirect_https"`
	WebAccessLog     bool   `json:"web_access_log"`
	WebCertFile      string `json:"web_cert_file"`
	WebKeyFile       string `json:"web_key_file"`

	Proxy string `json:"proxy"`

	CertEnable      bool     `json:"cert_enable"`
	CertEmail       string   `json:"cert_email"`
	CertDomains     []string `json:"cert_domains"`
	CertToken       string   `json:"cert_token"`
	CertRenewBefore string   `json:"cert_renew_before"`

	STUNListEnable   bool     `json:"stun_list_enable"`
	STUNListURL      string   `json:"stun_list_url"`
	STUNListInterval string   `json:"stun_list_interval"`
	STUNListServers  []string `json:"stun_list_servers"`
}

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	cfg := s.store.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"admin_user":  cfg.Server.AdminUser,
		"log_level":   cfg.Server.LogLevel,
		"server_port": config.PortOf(cfg.Server.Listen),
		"config":      s.store.Path(),

		"web_port":           config.PortOf(cfg.Web.Listen),
		"web_https":          cfg.Web.HTTPS,
		"web_redirect_https": cfg.Web.RedirectHTTPS,
		"web_access_log":     cfg.Web.AccessLog,
		"web_cert_file":      cfg.Web.CertFile,
		"web_key_file":       cfg.Web.KeyFile,

		"proxy": cfg.Net.Proxy,

		"cert_enable":       cfg.Cert.Enable,
		"cert_provider":     cfg.Cert.Provider,
		"cert_email":        cfg.Cert.Email,
		"cert_domains":      cfg.Cert.Domains,
		"cert_token_set":    cfg.Cert.Auth["token"] != "",
		"cert_renew_before": time.Duration(cfg.Cert.RenewBefore).String(),

		"stun_list_enable":   cfg.STUNList.Enable,
		"stun_list_url":      cfg.STUNList.URL,
		"stun_list_interval": time.Duration(cfg.STUNList.Interval).String(),
		"stun_list_servers":  cfg.STUNList.Servers,
	})
}

func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var in settingsInput
	if !readJSON(w, r, &in) {
		return
	}
	var newHash string
	if strings.TrimSpace(in.Password) != "" {
		h, err := HashPassword(in.Password)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "生成密码失败")
			return
		}
		newHash = h
	}
	restart := false
	if err := s.store.Apply(func(c *config.Config) error {
		if strings.TrimSpace(in.AdminUser) != "" {
			c.Server.AdminUser = strings.TrimSpace(in.AdminUser)
		}
		if newHash != "" {
			c.Server.AdminPasswordHash = newHash
		}
		switch in.LogLevel {
		case "debug", "info", "warn", "error":
			c.Server.LogLevel = in.LogLevel
		}
		if p := strings.TrimSpace(in.ServerPort); p != "" {
			next := config.NormalizeListen(p)
			if next != c.Server.Listen {
				restart = true
				c.Server.Listen = next
			}
		}

		if p := strings.TrimSpace(in.WebPort); p != "" {
			c.Web.Listen = config.NormalizeListen(p)
		}
		c.Web.HTTPS = in.WebHTTPS
		c.Web.RedirectHTTPS = in.WebRedirectHTTPS
		c.Web.AccessLog = in.WebAccessLog
		c.Web.CertFile = strings.TrimSpace(in.WebCertFile)
		c.Web.KeyFile = strings.TrimSpace(in.WebKeyFile)

		c.Net.Proxy = strings.TrimSpace(in.Proxy)

		c.Cert.Enable = in.CertEnable
		c.Cert.Email = strings.TrimSpace(in.CertEmail)
		c.Cert.Domains = in.CertDomains
		if tok := strings.TrimSpace(in.CertToken); tok != "" {
			if c.Cert.Auth == nil {
				c.Cert.Auth = map[string]string{}
			}
			c.Cert.Auth["token"] = tok
		}
		if d := strings.TrimSpace(in.CertRenewBefore); d != "" {
			if dur, err := time.ParseDuration(d); err == nil && dur > 0 {
				c.Cert.RenewBefore = config.Duration(dur)
			}
		}

		c.STUNList.Enable = in.STUNListEnable
		c.STUNList.URL = strings.TrimSpace(in.STUNListURL)
		c.STUNList.Servers = in.STUNListServers
		if d := strings.TrimSpace(in.STUNListInterval); d != "" {
			if dur, err := time.ParseDuration(d); err == nil && dur > 0 {
				c.STUNList.Interval = config.Duration(dur)
			}
		}
		return nil
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if newHash != "" {
		s.setSession(w, r, s.store.Get().Server.AdminUser)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restart_required": restart, "settings": s.store.Get()})
}

// --------------------------------------------------------------------- cert

func (s *Server) handleCertGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cert.Status())
}

func (s *Server) handleCertIssue(w http.ResponseWriter, r *http.Request) {
	st := s.cert.Status()
	if st.Issuing {
		writeErr(w, http.StatusConflict, "正在签发中，请稍候")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		if err := s.cert.Ensure(ctx, true); err != nil {
			s.log.Error("手动签发失败: %v", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": "已开始签发，稍后刷新查看结果"})
}

// ------------------------------------------------------------------- config

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	cfg := s.store.Get()
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="net-bloop.toml"`)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	raw, err := readAllLimited(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "读取失败")
		return
	}
	current := s.store.Get()
	next := config.Default()
	if _, err := toml.Decode(string(raw), next); err != nil {
		writeErr(w, http.StatusBadRequest, "TOML 解析失败: "+err.Error())
		return
	}
	if next.Server.AdminPasswordHash == "" {
		next.Server.AdminPasswordHash = current.Server.AdminPasswordHash
	}
	// Keep the account settings of the running instance unless the uploaded
	// file spells them out. A partial config (rules only) must not reset the
	// admin name or move the listener to the default port.
	var probe struct {
		Server map[string]any `toml:"server"`
	}
	_, _ = toml.Decode(string(raw), &probe)
	if _, ok := probe.Server["admin_user"]; !ok {
		next.Server.AdminUser = current.Server.AdminUser
	}
	next.Server.Listen = current.Server.Listen
	if _, ok := probe.Server["log_level"]; !ok {
		next.Server.LogLevel = current.Server.LogLevel
	}
	if len(next.Cert.Auth) == 0 || next.Cert.Auth["token"] == "" {
		next.Cert.Auth = current.Cert.Auth
	}
	if err := s.store.Replace(next); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
