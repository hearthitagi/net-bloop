package admin

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"net-bloop/internal/cert"
	"net-bloop/internal/config"
	"net-bloop/internal/ddns"
	"net-bloop/internal/logx"
	"net-bloop/internal/proxy"
	stunmgr "net-bloop/internal/stun"
)

//go:embed dist
var uiFS embed.FS

type Server struct {
	store   *config.Store
	hub     *logx.Hub
	log     *logx.Logger
	ddns    *ddns.Manager
	proxy   *proxy.Manager
	stun    *stunmgr.Manager
	cert    *cert.Manager
	servers *stunmgr.Servers
	auth    *auth
	started time.Time
	version string
}

func NewServer(store *config.Store, hub *logx.Hub, log *logx.Logger, d *ddns.Manager, p *proxy.Manager, s *stunmgr.Manager, c *cert.Manager, list *stunmgr.Servers, version string) *Server {
	return &Server{
		store: store, hub: hub, log: log, ddns: d, proxy: p, stun: s, cert: c, servers: list,
		auth: newAuth(), started: time.Now(), version: version,
	}
}

// Handler builds the HTTP handler for the admin UI and API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now()})
	})

	mux.HandleFunc("GET /api/state", s.authed(s.handleState))

	mux.HandleFunc("GET /api/ddns", s.authed(s.handleDDNSList))
	mux.HandleFunc("GET /api/ddns/providers", s.authed(s.handleDDNSProviders))
	mux.HandleFunc("POST /api/ddns", s.authed(s.handleDdnsCreate))
	mux.HandleFunc("PUT /api/ddns/{id}", s.authed(s.handleDdnsUpdate))
	mux.HandleFunc("DELETE /api/ddns/{id}", s.authed(s.handleDdnsDelete))
	mux.HandleFunc("POST /api/ddns/{id}/run", s.authed(s.handleDdnsRun))

	mux.HandleFunc("GET /api/proxy", s.authed(s.handleProxyList))
	mux.HandleFunc("POST /api/proxy", s.authed(s.handleProxyCreate))
	mux.HandleFunc("PUT /api/proxy/{id}", s.authed(s.handleProxyUpdate))
	mux.HandleFunc("DELETE /api/proxy/{id}", s.authed(s.handleProxyDelete))

	mux.HandleFunc("GET /api/stun", s.authed(s.handleSTUNList))
	mux.HandleFunc("POST /api/stun", s.authed(s.handleSTUNCreate))
	mux.HandleFunc("PUT /api/stun/{id}", s.authed(s.handleSTUNUpdate))
	mux.HandleFunc("DELETE /api/stun/{id}", s.authed(s.handleSTUNDelete))
	mux.HandleFunc("POST /api/stun/probe", s.authed(s.handleSTUNProbe))
	mux.HandleFunc("GET /api/stun/servers", s.authed(s.handleSTUNServers))
	mux.HandleFunc("POST /api/stun/servers/refresh", s.authed(s.handleSTUNServersRefresh))

	mux.HandleFunc("GET /api/cert", s.authed(s.handleCertGet))
	mux.HandleFunc("POST /api/cert/issue", s.authed(s.handleCertIssue))

	mux.HandleFunc("GET /api/logs", s.authed(s.handleLogs))
	mux.HandleFunc("GET /api/stream", s.authed(s.handleStream))

	mux.HandleFunc("GET /api/settings", s.authed(s.handleSettingsGet))
	mux.HandleFunc("PUT /api/settings", s.authed(s.handleSettingsPut))
	mux.HandleFunc("GET /api/config/export", s.authed(s.handleExport))
	mux.HandleFunc("POST /api/config/import", s.authed(s.handleImport))

	mux.Handle("/", s.staticHandler())
	return mux
}

func (s *Server) secret() string {
	cfg := s.store.Get()
	return sessionSecret(cfg.Server.AdminPasswordHash)
}

func (s *Server) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			writeErr(w, http.StatusForbidden, "请求来源不合法")
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "未登录")
			return
		}
		user, ok := validToken(s.secret(), c.Value)
		if !ok || user != s.store.Get().Server.AdminUser {
			writeErr(w, http.StatusUnauthorized, "登录已过期")
			return
		}
		h(w, r)
	}
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	host := r.Host
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	return strings.HasSuffix(origin, "://"+host) || origin == "http://"+host || origin == "https://"+host
}

func (s *Server) staticHandler() http.Handler {
	sub, err := fs.Sub(uiFS, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "UI 未打包", http.StatusInternalServerError)
		})
	}
	files := http.FileServer(http.FS(sub))
	assetVer := assetVersion(sub)
	serveIndex := func(w http.ResponseWriter) {
		raw, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.NotFound(w, nil)
			return
		}
		body := strings.ReplaceAll(string(raw), "{{ASSET_VER}}", assetVer)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = io.WriteString(w, body)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "index.html" {
			serveIndex(w)
			return
		}
		if _, err := fs.Stat(sub, path); err != nil {
			// SPA fallback
			serveIndex(w)
			return
		}
		// Static assets carry a content hash in the query string, so they can be
		// cached long term; a new binary gets new URLs automatically.
		if strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".svg") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// assetVersion derives a short content hash so index.html can cache-bust the
// bundled assets after an upgrade.
func assetVersion(sub fs.FS) string {
	h := sha256.New()
	for _, name := range []string{"app.js", "style.css"} {
		raw, err := fs.ReadFile(sub, name)
		if err != nil {
			continue
		}
		_, _ = h.Write([]byte(name))
		_, _ = h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))[:8]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "读取请求失败")
		return false
	}
	if len(raw) == 0 {
		writeErr(w, http.StatusBadRequest, "请求体为空")
		return false
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func readAllLimited(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 2<<20))
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
