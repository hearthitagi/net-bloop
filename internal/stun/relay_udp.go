package stun

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/stun/v3"
)

type stunEvent struct {
	msg  *stun.Message
	from *net.UDPAddr
}

type udpSession struct {
	key      string
	client   *net.UDPAddr
	conn     *net.UDPConn
	lastSeen int64
}

type udpRelay struct {
	rt       *runtime
	conn     *net.UDPConn
	target   string
	mu       sync.Mutex
	sessions map[string]*udpSession
}

func (rt *runtime) serveUDP(ctx context.Context) error {
	lc := net.ListenConfig{Control: reuseControl}
	pc, err := lc.ListenPacket(ctx, "udp", rt.listenAddr())
	if err != nil {
		return fmt.Errorf("监听 UDP %s 失败: %w", rt.listenAddr(), err)
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return fmt.Errorf("UDP socket 类型异常")
	}
	defer conn.Close()

	relay := &udpRelay{rt: rt, conn: conn, target: rt.rule.Target, sessions: map[string]*udpSession{}}
	defer relay.closeAll()

	events := make(chan stunEvent, 32)
	go rt.udpKeepalive(ctx, conn, events)

	// Unblock ReadFromUDP on shutdown.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	rt.log.WithRule(rt.rule.Name).Info("UDP 已监听 %s，转发到 %s", rt.listenAddr(), rt.rule.Target)
	buf := make([]byte, 65535)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("UDP 读取失败: %w", err)
		}
		pkt := buf[:n]
		if stun.IsMessage(pkt) {
			m := stun.New()
			m.Raw = pkt
			if m.Decode() == nil {
				if m.Type == stun.BindingSuccess {
					select {
					case events <- stunEvent{msg: m, from: from}:
					default:
					}
					continue
				}
				if m.Type == stun.BindingRequest {
					if resp, ok := ServeSTUNRequest(pkt, from); ok {
						_, _ = conn.WriteToUDP(resp, from)
						continue
					}
				}
			}
		}
		relay.forward(from, pkt)
	}
}

func (r *udpRelay) closeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, s := range r.sessions {
		_ = s.conn.Close()
		delete(r.sessions, k)
	}
}

func (r *udpRelay) forward(client *net.UDPAddr, data []byte) {
	key := client.String()
	r.mu.Lock()
	s, ok := r.sessions[key]
	if !ok {
		conn, err := net.Dial("udp", r.target)
		if err != nil {
			r.mu.Unlock()
			r.rt.log.WithRule(r.rt.rule.Name).Warn("连接目标 %s 失败: %v", r.target, err)
			return
		}
		uc, _ := conn.(*net.UDPConn)
		s = &udpSession{key: key, client: client, conn: uc}
		r.sessions[key] = s
		go r.readBack(s)
	}
	s.lastSeen = time.Now().UnixNano()
	r.mu.Unlock()
	if _, err := s.conn.Write(data); err != nil {
		r.rt.log.WithRule(r.rt.rule.Name).Debug("转发到目标失败: %v", err)
	}
}

func (r *udpRelay) readBack(s *udpSession) {
	defer func() {
		_ = s.conn.Close()
		r.mu.Lock()
		if cur, ok := r.sessions[s.key]; ok && cur == s {
			delete(r.sessions, s.key)
		}
		r.mu.Unlock()
	}()
	buf := make([]byte, 65535)
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
		n, err := s.conn.Read(buf)
		if err != nil {
			return
		}
		if _, err := r.conn.WriteToUDP(buf[:n], s.client); err != nil {
			return
		}
	}
}

// ------------------------------------------------------------------ keepalive

func (rt *runtime) udpKeepalive(ctx context.Context, conn *net.UDPConn, events <-chan stunEvent) {
	lg := rt.log.WithRule(rt.rule.Name)
	interval := rt.keepaliveInterval()
	names := make([]string, 0, 8)
	resolved := make(map[string]*net.UDPAddr, 8)
	for _, s := range rt.stunServers() {
		if _, ok := resolved[s]; ok {
			continue
		}
		host, err := resolveSTUN(s, rt.wantIPv6())
		if err != nil {
			lg.Debug("跳过 STUN 服务器 %s: %v", s, err)
			continue
		}
		if addr, err := net.ResolveUDPAddr("udp", host); err == nil {
			resolved[s] = addr
			names = append(names, s)
		}
	}
	if len(names) == 0 {
		lg.Error("没有可用的 STUN 服务器")
		return
	}
	for {
		server := rt.pickServer(names)
		target := resolved[server]
		addr, err := rt.stunUDP(conn, target, events)
		if ctx.Err() != nil {
			return
		}
		rt.noteServerResult(names, server, err)
		if err != nil {
			lg.Warn("STUN(%s) 保活失败: %v", target, err)
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

func (rt *runtime) stunUDP(conn *net.UDPConn, server *net.UDPAddr, events <-chan stunEvent) (*net.UDPAddr, error) {
	req := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.WriteToUDP(req.Raw, server); err != nil {
		return nil, err
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.msg.TransactionID != req.TransactionID {
				continue
			}
			var xor stun.XORMappedAddress
			if err := xor.GetFrom(ev.msg); err == nil {
				return &net.UDPAddr{IP: xor.IP, Port: xor.Port}, nil
			}
			var mapped stun.MappedAddress
			if err := mapped.GetFrom(ev.msg); err == nil {
				return &net.UDPAddr{IP: mapped.IP, Port: mapped.Port}, nil
			}
			return nil, fmt.Errorf("STUN 响应缺少映射地址")
		case <-deadline:
			return nil, fmt.Errorf("等待响应超时")
		}
	}
}
