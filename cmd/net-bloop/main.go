package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"net-bloop/internal/admin"
	"net-bloop/internal/cert"
	"net-bloop/internal/config"
	"net-bloop/internal/ddns"
	"net-bloop/internal/logx"
	"net-bloop/internal/proxy"
	stunmgr "net-bloop/internal/stun"
)

var version = "0.2.6"

func main() {
	var (
		configPath = flag.String("config", defaultConfigPath(), "配置文件路径")
		showVer    = flag.Bool("version", false, "显示版本后退出")
	)
	flag.Parse()
	if *showVer {
		fmt.Println("net-bloop", version)
		return
	}
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("")

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		if err := config.Save(*configPath, cfg); err != nil {
			log.Printf("写入初始配置失败: %v", err)
		} else {
			log.Printf("已生成配置文件: %s", *configPath)
		}
	}

	dataDir := filepath.Dir(*configPath)
	hub := logx.NewHub(3000)
	coreLog := logx.New(hub, "core", "")
	store := config.NewStore(*configPath, cfg)

	ddnsMgr := ddns.NewManager(store, logx.New(hub, "ddns", ""))
	proxyMgr := proxy.NewManager(store, logx.New(hub, "web", ""), dataDir)
	stunMgr := stunmgr.NewManager(store, logx.New(hub, "stun", ""))
	certMgr := cert.NewManager(store, logx.New(hub, "cert", ""), dataDir)
	servers := stunmgr.NewServers(store, logx.New(hub, "stun", ""), dataDir)

	proxyMgr.SetCertManager(certMgr)
	stunMgr.SetServerSource(servers.For)
	ddnsMgr.SetSTUNIPResolver(stunMgr.PublicIP)

	reconcile := func(c *config.Config) {
		ddnsMgr.Reconcile(c)
		proxyMgr.Reconcile(c)
		stunMgr.Reconcile(c)
	}
	store.SetOnApply(reconcile)
	reconcile(cfg)
	servers.Start()
	certMgr.Start()

	adminSrv := admin.NewServer(store, hub, coreLog, ddnsMgr, proxyMgr, stunMgr, certMgr, servers, version)
	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           adminSrv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		coreLog.Info("管理界面已启动: http://%s", displayListen(cfg.Server.Listen))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	if reload := reloadSignal(); reload != nil {
		signal.Notify(sigCh, reload)
	}
	for {
		select {
		case err := <-errCh:
			coreLog.Error("HTTP 服务异常: %v", err)
			shutdown(srv, coreLog, ddnsMgr, proxyMgr, stunMgr, certMgr, servers)
			os.Exit(1)
		case sig := <-sigCh:
			if isReloadSignal(sig) {
				reloadConfig(*configPath, store, coreLog)
				continue
			}
			coreLog.Info("收到退出信号，正在停止服务...")
			shutdown(srv, coreLog, ddnsMgr, proxyMgr, stunMgr, certMgr, servers)
			return
		}
	}
}

func defaultConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("NETBLOOP_CONFIG")); v != "" {
		return v
	}
	return "net-bloop.toml"
}

func displayListen(listen string) string {
	host, port, err := splitListen(listen)
	if err != nil {
		return listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1:" + port
	}
	return host + ":" + port
}

func splitListen(listen string) (string, string, error) {
	if strings.HasPrefix(listen, ":") {
		return "", listen[1:], nil
	}
	idx := strings.LastIndex(listen, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("无效监听地址")
	}
	return listen[:idx], listen[idx+1:], nil
}

func reloadConfig(path string, store *config.Store, lg *logx.Logger) {
	next, err := config.Load(path)
	if err != nil {
		lg.Error("重新加载配置失败: %v", err)
		return
	}
	if next.Server.AdminPasswordHash == "" {
		next.Server.AdminPasswordHash = store.Get().Server.AdminPasswordHash
	}
	if err := store.Replace(next); err != nil {
		lg.Error("应用新配置失败: %v", err)
		return
	}
	lg.Info("配置已重新加载")
}

func shutdown(srv *http.Server, lg *logx.Logger, d *ddns.Manager, p *proxy.Manager, s *stunmgr.Manager, c *cert.Manager, list *stunmgr.Servers) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	s.Stop()
	p.Stop()
	d.Stop()
	c.Stop()
	list.Stop()
	lg.Info("已停止")
}
