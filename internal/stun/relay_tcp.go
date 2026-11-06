package stun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/pion/stun/v3"
)

const maxTCPConns = 2048

// relayGrace is how long an established connection may keep running after the
// rule has been reloaded or stopped. Without it, editing a rule would cut every
// session that is being served through this port — including the admin panel
// itself when the user is browsing it through the relay, which shows up in the
// browser as a bare "Failed to fetch" right after saving.
const relayGrace = 90 * time.Second

func (rt *runtime) serveTCP(ctx context.Context) error {
	lc := net.ListenConfig{Control: reuseControl}
	ln, err := lc.Listen(ctx, "tcp", rt.listenAddr())
	if err != nil {
		return fmt.Errorf("监听 TCP %s 失败: %w", rt.listenAddr(), err)
	}
	defer ln.Close()

	go rt.tcpKeepalive(ctx)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	rt.log.WithRule(rt.rule.Name).Info("TCP 已监听 %s，转发到 %s", rt.listenAddr(), rt.rule.Target)
	sem := make(chan struct{}, maxTCPConns)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return fmt.Errorf("TCP 接受连接失败: %w", err)
		}
		select {
		case sem <- struct{}{}:
			go func() {
				defer func() { <-sem }()
				rt.handleTCP(ctx, conn)
			}()
		default:
			_ = conn.Close()
		}
	}
}

func (rt *runtime) handleTCP(ctx context.Context, client net.Conn) {
	lg := rt.log.WithRule(rt.rule.Name)
	defer client.Close()
	if tc, ok := client.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(60 * time.Second)
	}
	upstream, err := net.DialTimeout("tcp", rt.rule.Target, 10*time.Second)
	if err != nil {
		lg.Warn("连接目标 %s 失败: %v", rt.rule.Target, err)
		return
	}
	defer upstream.Close()
	if tc, ok := upstream.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(60 * time.Second)
	}

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, client)
		if tc, ok := upstream.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		} else {
			_ = upstream.Close()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		if tc, ok := client.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		} else {
			_ = client.Close()
		}
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-ctx.Done():
		// Rule reloaded: stop accepting new connections but let this one
		// finish, so an in-flight download or a pending panel request is not
		// killed halfway through.
		lg.Debug("规则已重载，保持已有连接最多 %s", relayGrace)
		select {
		case <-done:
		case <-time.After(relayGrace):
		}
	}
}

func (rt *runtime) tcpKeepalive(ctx context.Context) {
	lg := rt.log.WithRule(rt.rule.Name)
	interval := rt.keepaliveInterval()
	servers := rt.stunServers()
	if len(servers) == 0 {
		lg.Error("没有可用的 STUN 服务器")
		return
	}
	// The relay listens on LocalPort, so the STUN binding must leave from that
	// same local port for the NAT pinhole to line up with the listener.
	for {
		server := rt.pickServer(servers)
		addr, err := rt.tcpSTUN(ctx, server, true)
		if ctx.Err() != nil {
			return
		}
		rt.noteServerResult(servers, server, err)
		if err != nil {
			if strings.Contains(err.Error(), "address already in use") {
				lg.Error("同端口 STUN 保活失败（当前系统不支持端口复用）：%v", err)
			} else {
				lg.Warn("STUN(%s) 保活失败: %v", server, err)
			}
			rt.setError(err)
		} else {
			rt.noteHeartbeat()
			rt.report(addr.IP.String(), addr.Port, "STUN 打洞")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (rt *runtime) tcpSTUN(ctx context.Context, server string, samePort bool) (*net.UDPAddr, error) {
	addr, err := resolveSTUN(server, rt.wantIPv6())
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 8 * time.Second, Control: reuseControl}
	if samePort {
		ip := net.ParseIP(rt.rule.ListenIP)
		if ip != nil && !ip.IsUnspecified() {
			d.LocalAddr = &net.TCPAddr{IP: ip, Port: rt.rule.LocalPort}
		} else {
			d.LocalAddr = &net.TCPAddr{Port: rt.rule.LocalPort}
		}
	}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	req := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := req.WriteTo(conn); err != nil {
		return nil, err
	}
	res := stun.New()
	res.Raw = make([]byte, 1500)
	if _, err := res.ReadFrom(conn); err != nil {
		return nil, err
	}
	if res.Type != stun.BindingSuccess {
		return nil, fmt.Errorf("STUN 返回 %s", res.Type)
	}
	var xor stun.XORMappedAddress
	if err := xor.GetFrom(res); err == nil {
		return &net.UDPAddr{IP: xor.IP, Port: xor.Port}, nil
	}
	var mapped stun.MappedAddress
	if err := mapped.GetFrom(res); err == nil {
		return &net.UDPAddr{IP: mapped.IP, Port: mapped.Port}, nil
	}
	return nil, fmt.Errorf("STUN 响应缺少映射地址")
}
