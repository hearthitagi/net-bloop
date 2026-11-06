package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type cloudflare struct{}

const cfBase = "https://api.cloudflare.com/client/v4"

type cfResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

func (c *cfResponse) err() error {
	if c.Success {
		return nil
	}
	msgs := make([]string, 0, len(c.Errors))
	for _, e := range c.Errors {
		msgs = append(msgs, e.Message)
	}
	if len(msgs) == 0 {
		msgs = []string{"未知错误"}
	}
	return fmt.Errorf("Cloudflare: %s", strings.Join(msgs, "; "))
}

func (cloudflare) Update(ctx context.Context, r Request) error {
	token := Auth(r, "token")
	if token == "" {
		return fmt.Errorf("缺少 API Token")
	}
	headers := map[string]string{"Authorization": "Bearer " + token}
	_, root, err := SplitDomain(r.Domain, Auth(r, "root_domain"))
	if err != nil {
		return err
	}
	zoneID := Auth(r, "zone_id")
	if zoneID == "" {
		var zr cfResponse
		u := cfBase + "/zones?name=" + url.QueryEscape(root)
		if err := doJSON(ctx, "GET", u, headers, nil, &zr); err != nil {
			return err
		}
		if err := zr.err(); err != nil {
			return err
		}
		var zones []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(zr.Result, &zones); err != nil || len(zones) == 0 {
			return fmt.Errorf("Cloudflare: 找不到 zone %s", root)
		}
		zoneID = zones[0].ID
	}

	var rr cfResponse
	u := cfBase + "/zones/" + zoneID + "/dns_records?type=" + url.QueryEscape(r.Type) + "&name=" + url.QueryEscape(r.Domain)
	if err := doJSON(ctx, "GET", u, headers, nil, &rr); err != nil {
		return err
	}
	if err := rr.err(); err != nil {
		return err
	}
	var records []struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Result, &records)

	ttl := r.TTL
	if ttl < 60 {
		ttl = 1
	}
	proxied, proxiedSet := boolAuth(r, "proxied")

	if len(records) > 0 {
		// PATCH only what this module owns: the address, plus the proxy flag
		// when the user configured it explicitly. A full PUT would reset the
		// record's Cloudflare proxy/TTL settings on every sync, which can take
		// a working wildcard off the proxy (and kill redirect rules).
		patch := map[string]any{"content": r.Value}
		if proxiedSet {
			patch["proxied"] = proxied
			if proxied {
				patch["ttl"] = 1
			}
		}
		var out cfResponse
		if err := doJSON(ctx, "PATCH", cfBase+"/zones/"+zoneID+"/dns_records/"+records[0].ID, headers, patch, &out); err != nil {
			return err
		}
		return out.err()
	}

	body := map[string]any{
		"type":    r.Type,
		"name":    r.Domain,
		"content": r.Value,
		"ttl":     ttl,
		"proxied": proxied,
	}
	var out cfResponse
	if err := doJSON(ctx, "POST", cfBase+"/zones/"+zoneID+"/dns_records", headers, body, &out); err != nil {
		return err
	}
	return out.err()
}

// boolAuth reports the parsed value of an auth key, and whether it was set at
// all. "Leave it empty" must stay distinguishable from "false".
func boolAuth(r Request, key string) (bool, bool) {
	switch strings.ToLower(Auth(r, key)) {
	case "true", "1", "yes", "on":
		return true, true
	case "false", "0", "no", "off":
		return false, true
	}
	return false, false
}

func init() {
	Register(Info{Name: "cloudflare", Label: "Cloudflare", Fields: []Field{
		{Key: "token", Label: "API Token", Placeholder: "Zone.DNS 编辑权限", Secret: true, Required: true},
		{Key: "zone_id", Label: "Zone ID", Placeholder: "留空则按根域名自动查询"},
		{Key: "root_domain", Label: "根域名", Placeholder: "example.com，留空自动推导"},
		{Key: "proxied", Label: "Cloudflare 代理（橙云）", Placeholder: "留空保持现有设置，或 true / false"},
	}}, func() Provider { return cloudflare{} })
}
