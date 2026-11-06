package provider

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type godaddy struct{}

func (godaddy) Update(ctx context.Context, r Request) error {
	key, secret := Auth(r, "key"), Auth(r, "secret")
	if key == "" || secret == "" {
		return fmt.Errorf("缺少 API Key / Secret")
	}
	rr, root, err := SplitDomain(r.Domain, Auth(r, "root_domain"))
	if err != nil {
		return err
	}
	u := fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records/%s/%s", root, r.Type, url.PathEscape(rr))
	body := []map[string]any{{"data": r.Value, "ttl": r.TTL}}
	return doJSON(ctx, "PUT", u, map[string]string{
		"Authorization": "sso-key " + key + ":" + secret,
	}, body, nil)
}

type porkbun struct{}

type pbResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Records []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Content string `json:"content"`
	} `json:"records"`
}

func (p porkbun) Update(ctx context.Context, r Request) error {
	apiKey, secret := Auth(r, "api_key"), Auth(r, "secret_api_key")
	if apiKey == "" || secret == "" {
		return fmt.Errorf("缺少 API Key / Secret API Key")
	}
	rr, root, err := SplitDomain(r.Domain, Auth(r, "root_domain"))
	if err != nil {
		return err
	}
	name := rr
	if name == "@" {
		name = ""
	}
	base := map[string]any{"apikey": apiKey, "secretapikey": secret}

	var list pbResponse
	if err := doJSON(ctx, "POST", "https://api.porkbun.com/api/json/v3/dns/retrieve/"+root, nil, base, &list); err != nil {
		return err
	}
	if !strings.EqualFold(list.Status, "SUCCESS") {
		return fmt.Errorf("Porkbun: %s", list.Message)
	}
	full := r.Domain
	for _, rec := range list.Records {
		if strings.EqualFold(rec.Type, r.Type) && strings.EqualFold(rec.Name, full) {
			body := map[string]any{
				"apikey": apiKey, "secretapikey": secret,
				"name": name, "type": r.Type, "content": r.Value, "ttl": fmt.Sprint(r.TTL),
			}
			var out pbResponse
			if err := doJSON(ctx, "POST", "https://api.porkbun.com/api/json/v3/dns/edit/"+root+"/"+rec.ID, nil, body, &out); err != nil {
				return err
			}
			if !strings.EqualFold(out.Status, "SUCCESS") {
				return fmt.Errorf("Porkbun: %s", out.Message)
			}
			return nil
		}
	}
	body := map[string]any{
		"apikey": apiKey, "secretapikey": secret,
		"name": name, "type": r.Type, "content": r.Value, "ttl": fmt.Sprint(r.TTL),
	}
	var out pbResponse
	if err := doJSON(ctx, "POST", "https://api.porkbun.com/api/json/v3/dns/create/"+root, nil, body, &out); err != nil {
		return err
	}
	if !strings.EqualFold(out.Status, "SUCCESS") {
		return fmt.Errorf("Porkbun: %s", out.Message)
	}
	return nil
}

type callback struct{}

func (callback) Update(ctx context.Context, r Request) error {
	raw := Auth(r, "url")
	if raw == "" {
		return fmt.Errorf("缺少回调地址")
	}
	repl := strings.NewReplacer(
		"{domain}", url.QueryEscape(r.Domain),
		"{ip}", url.QueryEscape(r.Value),
		"{type}", url.QueryEscape(r.Type),
	)
	target := repl.Replace(raw)
	method := strings.ToUpper(Auth(r, "method"))
	if method == "" {
		method = "GET"
	}
	if method == "GET" {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + "domain=" + url.QueryEscape(r.Domain) + "&ip=" + url.QueryEscape(r.Value) + "&type=" + url.QueryEscape(r.Type)
		return doJSON(ctx, "GET", target, nil, nil, nil)
	}
	body := map[string]string{"domain": r.Domain, "ip": r.Value, "type": r.Type}
	return doJSON(ctx, "POST", target, nil, body, nil)
}

func init() {
	Register(Info{Name: "godaddy", Label: "GoDaddy", Fields: []Field{
		{Key: "key", Label: "API Key", Required: true},
		{Key: "secret", Label: "API Secret", Secret: true, Required: true},
		{Key: "root_domain", Label: "根域名", Placeholder: "example.com，留空自动推导"},
	}}, func() Provider { return godaddy{} })

	Register(Info{Name: "porkbun", Label: "Porkbun", Fields: []Field{
		{Key: "api_key", Label: "API Key", Required: true},
		{Key: "secret_api_key", Label: "Secret API Key", Secret: true, Required: true},
		{Key: "root_domain", Label: "根域名", Placeholder: "example.com，留空自动推导"},
	}}, func() Provider { return porkbun{} })

	Register(Info{Name: "callback", Label: "自定义回调", Fields: []Field{
		{Key: "url", Label: "回调地址", Placeholder: "https://example.com/ddns?ip={ip}", Required: true},
		{Key: "method", Label: "请求方法", Placeholder: "GET 或 POST"},
	}}, func() Provider { return callback{} })
}
