package netx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var publicIPCandidates = []string{
	"https://api.ipify.org",
	"https://ipv4.icanhazip.com",
	"https://ip.sb",
	"https://myip.ipip.net",
}

func LocalIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if ua, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return ua.IP, nil
	}
	return nil, errors.New("no local address")
}

func InterfaceIP(name string, v6 bool) (net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		if name != "" && iface.Name != name {
			continue
		}
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP
			if v6 {
				if ip.To4() == nil && ip.IsGlobalUnicast() {
					return ip, nil
				}
			} else if ip4 := ip.To4(); ip4 != nil && ip4.IsGlobalUnicast() && !ip4.IsLinkLocalUnicast() {
				return ip4, nil
			}
		}
	}
	return nil, fmt.Errorf("interface %q has no usable address", name)
}

// Client builds an HTTP client that honours an explicit outbound proxy. An
// empty proxy falls back to the environment, so HTTP_PROXY still works.
func Client(proxy string, timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if p := strings.TrimSpace(proxy); p != "" {
		if !strings.Contains(p, "://") {
			p = "http://" + p
		}
		if u, err := url.Parse(p); err == nil && u.Host != "" {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

func PublicIP(ctx context.Context, proxy string) (string, error) {
	client := Client(proxy, 8*time.Second)
	var lastErr error
	for _, u := range publicIPCandidates {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "net-bloop/1.0")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		resp.Body.Close()
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("%s: %s", u, resp.Status)
			continue
		}
		// Some endpoints answer "当前 IP：1.2.3.4 来自：..." instead of a bare IP.
		for _, f := range strings.Fields(string(body)) {
			if ip := net.ParseIP(strings.Trim(f, "[](),;：")); ip != nil && ip.To4() != nil {
				return ip.String(), nil
			}
		}
		lastErr = fmt.Errorf("%s: bad response %q", u, strings.TrimSpace(string(body)))
	}
	if lastErr == nil {
		lastErr = errors.New("no public ip source")
	}
	return "", lastErr
}

func JoinHostPort(s string, defaultPort int) string {
	if _, _, err := net.SplitHostPort(s); err == nil {
		return s
	}
	return net.JoinHostPort(s, fmt.Sprint(defaultPort))
}
