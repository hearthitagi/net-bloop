package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultNormalizeValidate(t *testing.T) {
	c := Default()
	c.Normalize()
	if err := c.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	if c.Server.Listen != DefaultListen || c.Server.AdminUser != DefaultAdminUser {
		t.Fatalf("unexpected defaults: %+v", c.Server)
	}
}

func TestSTUNValidation(t *testing.T) {
	base := func() *Config {
		c := Default()
		c.STUN = []STUNRule{
			{ID: "a", Name: "a", Enable: true, Proto: "udp", Mode: ModePunch, LocalPort: 26666, Target: "127.0.0.1:80"},
			{ID: "b", Name: "b", Enable: true, Proto: "udp", Mode: ModePunch, LocalPort: 26666, Target: "127.0.0.1:81"},
		}
		c.Normalize()
		return c
	}

	if err := base().Validate(); err == nil {
		t.Fatal("duplicate udp port should be rejected")
	}

	// Same number on different protocols is fine.
	c := base()
	c.STUN[1].Proto = "tcp"
	c.Normalize()
	if err := c.Validate(); err != nil {
		t.Fatalf("tcp+udp same port should be valid: %v", err)
	}

	// Forward mode never listens, so it needs no target and no unique port.
	fwd := Default()
	fwd.STUN = []STUNRule{{ID: "f", Name: "f", Enable: true, Mode: ModeForward, PublicPort: 33222}}
	fwd.Normalize()
	if err := fwd.Validate(); err != nil {
		t.Fatalf("forward mode should not require a target: %v", err)
	}
	if fwd.STUN[0].Mode != ModeForward {
		t.Fatalf("mode should default to forward, got %q", fwd.STUN[0].Mode)
	}

	// Punch mode without a target is a configuration error.
	bad := Default()
	bad.STUN = []STUNRule{{ID: "p", Name: "p", Enable: true, Mode: ModePunch, LocalPort: 26666}}
	bad.Normalize()
	if err := bad.Validate(); err == nil {
		t.Fatal("punch mode without target should be rejected")
	}
}

func TestDurationJSON(t *testing.T) {
	type wrap struct {
		D Duration `json:"d"`
	}
	raw, err := json.Marshal(wrap{D: Duration(90 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"d":"1m30s"}` {
		t.Fatalf("unexpected json: %s", raw)
	}
	var back wrap
	if err := json.Unmarshal([]byte(`{"d":"5m0s"}`), &back); err != nil {
		t.Fatal(err)
	}
	if back.D.Duration() != 5*time.Minute {
		t.Fatalf("got %v", back.D.Duration())
	}
}

func TestStoreApplyIsCopyOnWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "net-bloop.toml")
	c := Default()
	c.Normalize()
	store := NewStore(path, c)

	before := store.Get()
	if err := store.Apply(func(next *Config) error {
		next.Proxy = append(next.Proxy, ProxyRule{ID: "p1", Domains: []string{"nas.example.com"}, Upstreams: []string{"http://127.0.0.1:80"}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(before.Proxy) != 0 {
		t.Fatal("Apply must not mutate the previous snapshot")
	}
	if len(store.Get().Proxy) != 1 {
		t.Fatal("rule was not added")
	}

	// A rejected mutation must not persist anything.
	err := store.Apply(func(next *Config) error {
		next.Proxy[0].Name = "changed"
		return errRejected
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if store.Get().Proxy[0].Name == "changed" {
		t.Fatal("failed mutation leaked into the live config")
	}
}

type rejectErr struct{}

func (rejectErr) Error() string { return "rejected" }

var errRejected = rejectErr{}

func TestLoadCreatesDefaultWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "net-bloop.toml")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Listen != DefaultListen {
		t.Fatalf("got %q", c.Server.Listen)
	}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Server.Listen != DefaultListen {
		t.Fatalf("round trip failed: %q", again.Server.Listen)
	}
}

func TestSplitAndValidateDomain(t *testing.T) {
	c := Default()
	c.DDNS = []DDNSRule{{ID: "d1", Name: "d", Provider: "cloudflare", Domain: "  ", Type: "A"}}
	c.Normalize()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "域名") {
		t.Fatalf("empty domain should be rejected, got %v", err)
	}
}
