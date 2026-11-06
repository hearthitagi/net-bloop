package proxy

import (
	"net/url"
	"testing"

	"net-bloop/internal/config"
)

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"Example.COM:443": "example.com",
		"example.com.":    "example.com",
		"  a.b.c:80  ":    "a.b.c",
		"":                "",
	}
	for in, want := range cases {
		if got := normalizeHost(in); got != want {
			t.Errorf("normalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchDomain(t *testing.T) {
	cases := []struct {
		host, pattern string
		want          bool
	}{
		{"nas.example.com", "nas.example.com", true},
		{"NAS.Example.com", "nas.example.com", true},
		{"a.example.com", "*.example.com", true},
		{"a.b.example.com", "*.example.com", true},
		{"example.com", "*.example.com", false},
		{"notexample.com", "*.example.com", false},
		{"a.example.com", "other.example.com", false},
	}
	for _, c := range cases {
		if got := matchDomain(c.host, c.pattern); got != c.want {
			t.Errorf("matchDomain(%q, %q) = %v, want %v", c.host, c.pattern, got, c.want)
		}
	}
}

func TestHostRouterPrecedence(t *testing.T) {
	exact := &ruleHandler{rule: config.ProxyRule{ID: "exact", Domains: []string{"nas.example.com"}}}
	wild := &ruleHandler{rule: config.ProxyRule{ID: "wild", Domains: []string{"*.example.com"}}}
	fallback := &ruleHandler{rule: config.ProxyRule{ID: "default"}}
	router := &hostRouter{
		exact:       map[string]*ruleHandler{"nas.example.com": exact},
		wildcard:    []*ruleHandler{wild},
		defaultRule: fallback,
	}

	if got := router.match("nas.example.com:443"); got != exact {
		t.Fatalf("exact match lost: %+v", got.rule)
	}
	if got := router.match("app.example.com"); got != wild {
		t.Fatalf("wildcard match lost: %+v", got.rule)
	}
	if got := router.match("unknown.test"); got != fallback {
		t.Fatalf("fallback lost: %+v", got.rule)
	}
}

func TestJoinURLPath(t *testing.T) {
	cases := []struct{ base, in, wantPath, wantRaw string }{
		{"/", "/a/b", "/a/b", ""},
		{"/api", "/users", "/api/users", ""},
		{"/api/", "/users", "/api/users", ""},
	}
	for _, c := range cases {
		b := mustURL(c.base)
		i := mustURL(c.in)
		p, raw := joinURLPath(b, i)
		if p != c.wantPath || raw != c.wantRaw {
			t.Errorf("joinURLPath(%q,%q) = (%q,%q), want (%q,%q)", c.base, c.in, p, raw, c.wantPath, c.wantRaw)
		}
	}
}

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}
