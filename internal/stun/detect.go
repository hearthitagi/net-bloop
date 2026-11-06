package stun

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/pion/stun/v3"

	"net-bloop/internal/config"
)

// probeOnce asks a single STUN server for the mapped address, over the
// protocol the rule uses. It returns the public IP and the port that server
// observed. Nothing is bound or forwarded.
func probeOnce(ctx context.Context, server, proto string) (string, int, error) {
	addr, err := resolveSTUN(server, false)
	if err != nil {
		return "", 0, err
	}
	network := "udp"
	if strings.EqualFold(proto, "tcp") {
		network = "tcp"
	}
	d := net.Dialer{Timeout: 6 * time.Second}
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return "", 0, err
	}
	defer conn.Close()
	mapped, err := exchange(conn, 6*time.Second)
	if err != nil {
		return "", 0, err
	}
	return mapped.IP.String(), mapped.Port, nil
}

func exchange(conn net.Conn, timeout time.Duration) (*net.UDPAddr, error) {
	req := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(req.Raw); err != nil {
		return nil, err
	}
	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		m := stun.New()
		m.Raw = buf[:n]
		if err := m.Decode(); err != nil {
			continue
		}
		if m.Type != stun.BindingSuccess || m.TransactionID != req.TransactionID {
			continue
		}
		var addr stun.XORMappedAddress
		if err := addr.GetFrom(m); err != nil {
			var mapped stun.MappedAddress
			if err2 := mapped.GetFrom(m); err2 != nil {
				return nil, fmt.Errorf("STUN 响应缺少映射地址")
			}
			return &net.UDPAddr{IP: mapped.IP, Port: mapped.Port}, nil
		}
		return &net.UDPAddr{IP: addr.IP, Port: addr.Port}, nil
	}
}

// resolveSTUN resolves a STUN server to an address of the wanted IP family.
// IP literals are checked without DNS so a mismatched family fails fast.
func resolveSTUN(server string, ipv6 bool) (string, error) {
	addr := serverAddr(server)
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if ip := net.ParseIP(host); ip != nil {
		if (ip.To4() == nil) != ipv6 {
			return "", fmt.Errorf("%s 不是 %s 地址", host, familyLabel(ipv6))
		}
		return addr, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", err
	}
	for _, cand := range ips {
		if (cand.IP.To4() == nil) == ipv6 {
			return net.JoinHostPort(cand.IP.String(), port), nil
		}
	}
	return "", fmt.Errorf("%s 没有 %s 地址", host, familyLabel(ipv6))
}

// ProbeResult describes what the current network looks like from the outside.
type ProbeResult struct {
	LocalIP    string `json:"local_ip"`
	MappedIP   string `json:"mapped_ip"`
	MappedPort int    `json:"mapped_port"`
	NATType    string `json:"nat_type"`
	Compatible bool   `json:"compatible"`
	Advice     string `json:"advice"`
	SecondPort int    `json:"second_port,omitempty"`
	ElapsedMS  int64  `json:"elapsed_ms"`
}

// Probe performs a simplified RFC 5780 mapping-behaviour test with two
// different STUN servers. If the mapping stays identical for both servers the
// NAT uses endpoint-independent mapping, which is what inbound hole punching
// needs. It is a diagnostic only: the panel does not need it to work.
func Probe(ctx context.Context, servers []string) (*ProbeResult, error) {
	if len(servers) == 0 {
		servers = append([]string(nil), config.DefaultSTUNServers...)
	}
	if len(servers) == 1 {
		servers = append(servers, servers[0])
	}
	start := time.Now()
	d := net.Dialer{Timeout: 4 * time.Second}
	conn, err := d.DialContext(ctx, "udp", serverAddr(servers[0]))
	if err != nil {
		return nil, fmt.Errorf("连接 STUN 服务器失败: %w", err)
	}
	defer conn.Close()
	res := &ProbeResult{}
	if local, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		res.LocalIP = local.IP.String()
	}

	first, err := exchange(conn, 4*time.Second)
	if err != nil {
		return nil, fmt.Errorf("STUN 服务器 %s 无响应: %w", servers[0], err)
	}
	res.MappedIP, res.MappedPort = first.IP.String(), first.Port

	second, err := exchange(conn, 4*time.Second)
	if err != nil {
		res.NATType = "未知（仅测到一个服务器）"
		res.Advice = "第二个 STUN 服务器不可达，无法判断 NAT 映射类型。可以换一组服务器重试。"
		res.ElapsedMS = time.Since(start).Milliseconds()
		return res, nil
	}
	res.SecondPort = second.Port
	switch {
	case first.IP.Equal(second.IP) && first.Port == second.Port:
		res.NATType = "Full Cone (NAT1 / EIM)"
		res.Compatible = true
		res.Advice = "映射与目标无关。如果路由器已经做了端口转发，公网端口就是映射表里那个数字，不会变。"
	case first.IP.Equal(second.IP):
		res.NATType = "Symmetric / 端口受限"
		res.Advice = "对称型 NAT 下打洞通常失败，公网端口也会变。请在路由器上做静态端口转发，并在规则里选“路由器已转发”。"
	default:
		res.NATType = "多出口 / 未知"
		res.Advice = "两次映射的公网 IP 不同，可能是多线负载均衡。建议固定出口或使用端口映射。"
	}
	res.ElapsedMS = time.Since(start).Milliseconds()
	return res, nil
}

// ServeSTUNRequest answers a binding request, letting external tools measure
// the address they see. The second return value is false when the packet is not
// a STUN binding request.
func ServeSTUNRequest(pkt []byte, from net.Addr) ([]byte, bool) {
	if !stun.IsMessage(pkt) {
		return nil, false
	}
	m := stun.New()
	m.Raw = pkt
	if err := m.Decode(); err != nil {
		return nil, false
	}
	if m.Type != stun.BindingRequest {
		return nil, false
	}
	res := stun.MustBuild(stun.NewTransactionIDSetter(m.TransactionID), stun.BindingSuccess)
	ua, ok := from.(*net.UDPAddr)
	if !ok || ua == nil {
		return nil, false
	}
	xor := stun.XORMappedAddress{IP: ua.IP, Port: ua.Port}
	if err := xor.AddTo(res); err != nil {
		return nil, false
	}
	return res.Raw, true
}
