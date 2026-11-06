package provider

import (
	"context"
	"fmt"
	"strings"
)

type dnspod struct{}

type dpStatus struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type dpResponse struct {
	Status  dpStatus `json:"status"`
	Records []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"records"`
}

func (d dnspod) call(ctx context.Context, r Request, params map[string]string, out *dpResponse) error {
	id := Auth(r, "id")
	token := Auth(r, "token")
	if id == "" || token == "" {
		return fmt.Errorf("缺少 API ID / Token")
	}
	form := map[string]string{
		"login_token": id + "," + token,
		"format":      "json",
		"lang":        "cn",
	}
	for k, v := range params {
		if k == "_action" {
			continue
		}
		form[k] = v
	}
	action := params["_action"]
	if err := doForm(ctx, "https://dnsapi.cn/"+action, nil, form, out); err != nil {
		return err
	}
	if out.Status.Code != "1" && out.Status.Code != "10" {
		return fmt.Errorf("DNSPod: %s %s", out.Status.Code, out.Status.Message)
	}
	return nil
}

func (d dnspod) Update(ctx context.Context, r Request) error {
	rr, root, err := SplitDomain(r.Domain, Auth(r, "root_domain"))
	if err != nil {
		return err
	}
	base := map[string]string{
		"domain":      root,
		"sub_domain":  rr,
		"record_type": r.Type,
	}
	list := map[string]string{"_action": "Record.List"}
	for k, v := range base {
		list[k] = v
	}
	var lr dpResponse
	if err := d.call(ctx, r, list, &lr); err != nil {
		return err
	}
	for _, rec := range lr.Records {
		if rec.Name == rr && strings.EqualFold(rec.Type, r.Type) {
			mod := map[string]string{
				"_action":     "Record.Modify",
				"domain":      root,
				"record_id":   rec.ID,
				"sub_domain":  rr,
				"record_type": r.Type,
				"record_line": "默认",
				"value":       r.Value,
				"ttl":         fmt.Sprint(r.TTL),
			}
			var mr dpResponse
			return d.call(ctx, r, mod, &mr)
		}
	}
	create := map[string]string{
		"_action":     "Record.Create",
		"domain":      root,
		"sub_domain":  rr,
		"record_type": r.Type,
		"record_line": "默认",
		"value":       r.Value,
		"ttl":         fmt.Sprint(r.TTL),
	}
	var cr dpResponse
	return d.call(ctx, r, create, &cr)
}

func init() {
	Register(Info{Name: "dnspod", Label: "DNSPod / 腾讯云", Fields: []Field{
		{Key: "id", Label: "API ID", Required: true},
		{Key: "token", Label: "API Token", Secret: true, Required: true},
		{Key: "root_domain", Label: "根域名", Placeholder: "example.com，留空自动推导"},
	}}, func() Provider { return dnspod{} })
}
