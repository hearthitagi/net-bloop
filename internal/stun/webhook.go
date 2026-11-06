package stun

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"net-bloop/internal/config"
	"net-bloop/internal/netx"
)

// HookVars is the data exposed to webhook templates.
type HookVars struct {
	IP        string
	Port      int
	Addr      string
	OldIP     string
	OldPort   int
	OldAddr   string
	Name      string
	Proto     string
	Time      string
	LocalPort int
	Target    string
}

func (v HookVars) AddrString() string { return v.Addr }

func newHookVars(rule config.STUNRule, ip string, port int, oldIP string, oldPort int) HookVars {
	return HookVars{
		IP:        ip,
		Port:      port,
		Addr:      joinAddr(ip, port),
		OldIP:     oldIP,
		OldPort:   oldPort,
		OldAddr:   joinAddr(oldIP, oldPort),
		Name:      rule.Name,
		Proto:     strings.ToUpper(rule.Proto),
		Time:      time.Now().Format(time.RFC3339),
		LocalPort: rule.LocalPort,
		Target:    rule.Target,
	}
}

func render(tpl string, vars HookVars) (string, error) {
	if tpl == "" {
		return "", nil
	}
	t, err := template.New("hook").Option("missingkey=zero").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var hookClient = &http.Client{Timeout: 15 * time.Second}

// fireWebhook renders and sends the rule's webhook. It is safe to call from
// multiple goroutines. Failures are logged, never fatal.
func (rt *runtime) fireWebhook(ctx context.Context, vars HookVars) {
	w := rt.rule.Webhook
	if w == nil || !w.Enable || strings.TrimSpace(w.URL) == "" {
		return
	}
	lg := rt.log.WithRule(rt.rule.Name)
	target, err := render(w.URL, vars)
	if err != nil {
		lg.Error("Webhook 地址模板错误: %v", err)
		return
	}
	body, err := render(w.Body, vars)
	if err != nil {
		lg.Error("Webhook 正文模板错误: %v", err)
		return
	}
	method := strings.ToUpper(strings.TrimSpace(w.Method))
	if method == "" {
		method = http.MethodPost
	}
	timeout := w.Timeout.Duration()
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := netx.Client(rt.proxyAddr(), timeout)

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if ctx.Err() != nil {
			return
		}
		req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
		if err != nil {
			lg.Error("Webhook 请求构建失败: %v", err)
			return
		}
		req.Header.Set("User-Agent", "net-bloop/1.0")
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range w.Headers {
			val, err := render(v, vars)
			if err != nil {
				lg.Error("Webhook 请求头 %s 模板错误: %v", k, err)
				continue
			}
			req.Header.Set(k, val)
		}
		resp, err := client.Do(req)
		if err == nil {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				lg.Info("Webhook 已触发: %s %s -> %s", method, vars.Addr, resp.Status)
				return
			}
			err = fmt.Errorf("HTTP %s: %s", resp.Status, truncateText(string(raw), 200))
		}
		lastErr = err
		lg.Warn("Webhook 第 %d 次失败: %v", attempt, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	lg.Error("Webhook 最终失败: %v", lastErr)
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
