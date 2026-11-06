package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Request struct {
	Domain string
	Type   string
	Value  string
	TTL    int
	Auth   map[string]string
}

type Provider interface {
	Update(ctx context.Context, r Request) error
}

type Field struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret"`
	Required    bool   `json:"required"`
}

type Info struct {
	Name   string  `json:"name"`
	Label  string  `json:"label"`
	Fields []Field `json:"fields"`
}

type Factory func() Provider

var registry = map[string]struct {
	info    Info
	factory Factory
}{}

func Register(info Info, f Factory) {
	registry[info.Name] = struct {
		info    Info
		factory Factory
	}{info, f}
}

func Get(name string) (Provider, bool) {
	e, ok := registry[name]
	if !ok {
		return nil, false
	}
	return e.factory(), true
}

func Infos() []Info {
	out := make([]Info, 0, len(registry))
	for _, e := range registry {
		out = append(out, e.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func Auth(r Request, key string) string { return strings.TrimSpace(r.Auth[key]) }

var httpClient = &http.Client{Timeout: 15 * time.Second}

func doJSON(ctx context.Context, method, url string, headers map[string]string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "net-bloop/1.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, url, resp.Status, truncate(string(raw), 300))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("解析响应失败: %w: %s", err, truncate(string(raw), 200))
		}
	}
	return nil
}

func doForm(ctx context.Context, url string, headers map[string]string, form map[string]string, out any) error {
	values := make([]string, 0, len(form))
	for k, v := range form {
		values = append(values, k+"="+v)
	}
	sort.Strings(values)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(strings.Join(values, "&")))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "net-bloop/1.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s: %s", resp.Status, truncate(string(raw), 300))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("解析响应失败: %w: %s", err, truncate(string(raw), 200))
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// SplitDomain returns (rr, root). If rootHint is set it is used directly.
func SplitDomain(full, rootHint string) (string, string, error) {
	full = strings.TrimSuffix(strings.TrimSpace(full), ".")
	if rootHint = strings.TrimSuffix(strings.TrimSpace(rootHint), "."); rootHint != "" {
		if full == rootHint {
			return "@", rootHint, nil
		}
		suffix := "." + rootHint
		if !strings.HasSuffix(full, suffix) {
			return "", "", fmt.Errorf("域名 %s 不属于 %s", full, rootHint)
		}
		return strings.TrimSuffix(full, suffix), rootHint, nil
	}
	parts := strings.Split(full, ".")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("无法从 %s 推导根域名，请填写根域名", full)
	}
	root := strings.Join(parts[len(parts)-2:], ".")
	rr := strings.Join(parts[:len(parts)-2], ".")
	if rr == "" {
		rr = "@"
	}
	return rr, root, nil
}
