package stun

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"net-bloop/internal/config"
	"net-bloop/internal/logx"
)

// TestRelaySurvivesReload locks in the fix for a rule edit cutting live
// connections. The admin panel may itself be served through this relay, so
// closing sessions on reload made the save request fail with "Failed to fetch".
func TestRelaySurvivesReload(t *testing.T) {
	// Upstream echo server.
	up, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	go func() {
		for {
			c, err := up.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(c)
		}
	}()

	// Free port for the relay listener.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	hub := logx.NewHub(32)
	rt := &runtime{
		rule: config.STUNRule{
			ID: "t", Name: "reload", Enable: true, Proto: "tcp",
			Mode: config.ModePunch, LocalPort: port, ListenIP: "127.0.0.1",
			Target: up.Addr().String(), Heartbeat: config.Duration(5 * time.Second),
			// Unreachable server keeps the keepalive loop busy without touching
			// the network in a way that would make the test flaky.
			Servers: []string{"127.0.0.1:9"},
		},
		log:  logx.New(hub, "stun", ""),
		done: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	rt.cancel = cancel
	go func() {
		defer close(rt.done)
		_ = rt.serveTCP(ctx)
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	var conn net.Conn
	for i := 0; i < 60; i++ {
		if c, err := net.Dial("tcp", addr); err == nil {
			conn = c
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if conn == nil {
		t.Fatal("relay listener never came up")
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("first echo: %v", err)
	}

	// Simulate the user saving the rule: the runtime is cancelled.
	cancel()
	time.Sleep(300 * time.Millisecond)

	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("again")); err != nil {
		t.Fatalf("write after reload: %v", err)
	}
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("connection was cut on reload: %v", err)
	}
	if string(buf) != "again" {
		t.Fatalf("echo mismatch: %q", buf)
	}
}

// TestStickyServer keeps a symmetric NAT from handing out a new public port on
// every heartbeat: once a server answers, the same one must be reused.
func TestStickyServer(t *testing.T) {
	rt := &runtime{}
	servers := []string{"a:3478", "b:3478", "c:3478"}

	if got := rt.pickServer(servers); got != "a:3478" {
		t.Fatalf("first pick: %q", got)
	}
	rt.noteServerResult(servers, "b:3478", nil)
	if got := rt.pickServer(servers); got != "b:3478" {
		t.Fatalf("after success should stick to b: %q", got)
	}
	// Two failures must not be enough to move away.
	for i := 0; i < serverFailLimit-1; i++ {
		rt.noteServerResult(servers, "b:3478", errTest)
	}
	if got := rt.pickServer(servers); got != "b:3478" {
		t.Fatalf("should still stick to b: %q", got)
	}
	// The third one moves on, and must not bounce straight back to b.
	rt.noteServerResult(servers, "b:3478", errTest)
	if got := rt.pickServer(servers); got == "b:3478" {
		t.Fatalf("should rotate away from b, got %q", got)
	}
	if got := rt.pickServer(servers); got != "c:3478" {
		t.Fatalf("should rotate to c, got %q", got)
	}

	// A server that disappears from the list must not be reused.
	rt.noteServerResult(servers, "c:3478", nil)
	rt.pickServer([]string{"a:3478"})
	if got := rt.pickServer([]string{"a:3478"}); got != "a:3478" {
		t.Fatalf("stale pick: %q", got)
	}
}

var errTest = &testErr{}

type testErr struct{}

func (e *testErr) Error() string { return "boom" }
