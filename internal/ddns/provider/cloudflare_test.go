package provider

import "testing"

func TestBoolAuth(t *testing.T) {
	cases := []struct {
		val    string
		want   bool
		wantOK bool
	}{
		{"", false, false},
		{"  ", false, false},
		{"true", true, true},
		{"TRUE", true, true},
		{"1", true, true},
		{"false", false, true},
		{"0", false, true},
		{"maybe", false, false},
	}
	for _, c := range cases {
		got, ok := boolAuth(Request{Auth: map[string]string{"proxied": c.val}}, "proxied")
		if got != c.want || ok != c.wantOK {
			t.Errorf("boolAuth(%q) = (%v,%v), want (%v,%v)", c.val, got, ok, c.want, c.wantOK)
		}
	}
}

func TestSplitDomainWildcard(t *testing.T) {
	rr, root, err := SplitDomain("*.stun.kissshot.site", "kissshot.site")
	if err != nil {
		t.Fatal(err)
	}
	if rr != "*.stun" || root != "kissshot.site" {
		t.Fatalf("got rr=%q root=%q", rr, root)
	}
}
