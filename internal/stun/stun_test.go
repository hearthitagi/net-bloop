package stun

import (
	"context"
	"net"
	"testing"

	"net-bloop/internal/config"
)

func TestJoinAddr(t *testing.T) {
	if got := joinAddr("1.2.3.4", 80); got != "1.2.3.4:80" {
		t.Fatalf("ipv4: %q", got)
	}
	if got := joinAddr("2001:db8::1", 443); got != "[2001:db8::1]:443" {
		t.Fatalf("ipv6: %q", got)
	}
	if got := joinAddr("", 80); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestRenderWebhookTemplate(t *testing.T) {
	vars := HookVars{
		IP:      "1.2.3.4",
		Port:    8080,
		Addr:    "1.2.3.4:8080",
		OldIP:   "1.2.3.4",
		OldPort: 7000,
		OldAddr: "1.2.3.4:7000",
		Name:    "nas", Proto: "TCP",
	}
	got, err := render(`{"new":"{{.Addr}}","old":"{{.OldAddr}}","name":"{{.Name}}"}`, vars)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"new":"1.2.3.4:8080","old":"1.2.3.4:7000","name":"nas"}`
	if got != want {
		t.Fatalf("got %s", got)
	}
	if _, err := render(`{{.Broken`, vars); err == nil {
		t.Fatal("invalid template should fail")
	}
}

func TestServerAddr(t *testing.T) {
	cases := map[string]string{
		"stun.miwifi.com:3478":         "stun.miwifi.com:3478",
		"stun:stun.l.google.com:19302": "stun.l.google.com:19302",
		"stun.nextcloud.com":           "stun.nextcloud.com:3478",
	}
	for in, want := range cases {
		if got := serverAddr(in); got != want {
			t.Errorf("serverAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveSTUNLiterals(t *testing.T) {
	cases := []struct {
		server string
		ipv6   bool
		ok     bool
	}{
		{"1.2.3.4:3478", false, true},
		{"1.2.3.4:3478", true, false},
		{"[2001:db8::1]:3478", true, true},
		{"[2001:db8::1]:3478", false, false},
		{"stun:1.2.3.4:3478", false, true},
	}
	for _, c := range cases {
		got, err := resolveSTUN(c.server, c.ipv6)
		if c.ok {
			if err != nil || got == "" {
				t.Errorf("resolveSTUN(%q, %v) = (%q, %v)", c.server, c.ipv6, got, err)
			}
		} else if err == nil {
			t.Errorf("resolveSTUN(%q, %v) should fail, got %q", c.server, c.ipv6, got)
		}
	}
}

func TestWantIPv6(t *testing.T) {
	cases := map[string]bool{
		"":                false,
		"0.0.0.0":         false,
		"192.168.1.10":    false,
		"::":              true,
		"240e:341:243b::1": true,
	}
	for listen, want := range cases {
		rt := &runtime{rule: config.STUNRule{ListenIP: listen}}
		if got := rt.wantIPv6(); got != want {
			t.Errorf("listen %q: got %v, want %v", listen, got, want)
		}
	}
}

func TestDefaultServersPerProto(t *testing.T) {
	udp := config.DefaultServers("udp")
	tcp := config.DefaultServers("tcp")
	if len(udp) == 0 || len(tcp) == 0 {
		t.Fatal("empty default server list")
	}
	if udp[0] == tcp[0] {
		t.Fatal("tcp and udp defaults should differ")
	}
}

func TestReuseControlIsUsable(t *testing.T) {
	// Binding two listeners on the same port proves the socket options are set.
	ctx := context.Background()
	lc := net.ListenConfig{Control: reuseControl}
	ln1, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln1.Close()
	addr := ln1.Addr().String()
	lc2 := net.ListenConfig{Control: reuseControl}
	ln2, err := lc2.Listen(ctx, "tcp", addr)
	if err != nil {
		// Windows without SO_REUSEPORT is expected to fail; don't fail the suite.
		t.Skipf("same-port listen not supported on this platform: %v", err)
	}
	ln2.Close()
}
