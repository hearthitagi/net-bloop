package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type aliyun struct{}

func aliyunEncode(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	e = strings.ReplaceAll(e, "*", "%2A")
	e = strings.ReplaceAll(e, "%7E", "~")
	return e
}

func (aliyun) call(ctx context.Context, auth map[string]string, params map[string]string) (json.RawMessage, error) {
	id := strings.TrimSpace(auth["access_key_id"])
	secret := strings.TrimSpace(auth["access_key_secret"])
	if id == "" || secret == "" {
		return nil, fmt.Errorf("缺少 AccessKey ID / Secret")
	}
	p := map[string]string{
		"AccessKeyId":      id,
		"Format":           "JSON",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"SignatureVersion": "1.0",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2015-01-09",
	}
	for k, v := range params {
		p[k] = v
	}
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, aliyunEncode(k)+"="+aliyunEncode(p[k]))
	}
	canonical := strings.Join(pairs, "&")
	stringToSign := "GET&%2F&" + aliyunEncode(canonical)
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	u := "https://alidns.aliyuncs.com/?Signature=" + aliyunEncode(sig) + "&" + canonical
	var raw json.RawMessage
	if err := doJSON(ctx, "GET", u, nil, nil, &raw); err != nil {
		return nil, err
	}
	var e struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	_ = json.Unmarshal(raw, &e)
	if e.Code != "" {
		return nil, fmt.Errorf("阿里云: %s %s", e.Code, e.Message)
	}
	return raw, nil
}

func (a aliyun) Update(ctx context.Context, r Request) error {
	rr, root, err := SplitDomain(r.Domain, Auth(r, "root_domain"))
	if err != nil {
		return err
	}
	raw, err := a.call(ctx, r.Auth, map[string]string{
		"Action": "DescribeDomainRecords", "DomainName": root,
		"RRKeyWord": rr, "Type": r.Type, "SearchMode": "EXACT",
	})
	if err != nil {
		return err
	}
	var list struct {
		DomainRecords struct {
			Record []struct {
				RecordID string `json:"RecordId"`
				RR       string `json:"RR"`
				Type     string `json:"Type"`
			} `json:"Record"`
		} `json:"DomainRecords"`
	}
	_ = json.Unmarshal(raw, &list)
	for _, rec := range list.DomainRecords.Record {
		if rec.RR == rr && strings.EqualFold(rec.Type, r.Type) {
			_, err := a.call(ctx, r.Auth, map[string]string{
				"Action": "UpdateDomainRecord", "RecordId": rec.RecordID,
				"RR": rr, "Type": r.Type, "Value": r.Value,
				"TTL": fmt.Sprint(r.TTL),
			})
			return err
		}
	}
	_, err = a.call(ctx, r.Auth, map[string]string{
		"Action": "AddDomainRecord", "DomainName": root, "RR": rr,
		"Type": r.Type, "Value": r.Value, "TTL": fmt.Sprint(r.TTL),
	})
	return err
}

func init() {
	Register(Info{Name: "aliyun", Label: "阿里云 DNS", Fields: []Field{
		{Key: "access_key_id", Label: "AccessKey ID", Secret: false, Required: true},
		{Key: "access_key_secret", Label: "AccessKey Secret", Secret: true, Required: true},
		{Key: "root_domain", Label: "根域名", Placeholder: "example.com，留空自动推导"},
	}}, func() Provider { return aliyun{} })
}
