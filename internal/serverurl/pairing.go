package serverurl

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"nfc-time-tracking-server/internal/config"
)

// PairingBaseURL returns the server URL the mobile app should use for pairing (field "u" in QR).
// When the admin UI is opened via localhost, the LAN IPv4 of this machine is used instead.
func PairingBaseURL(r *http.Request, srv config.ServerConfig) string {
	if h := strings.TrimSpace(srv.PairingAdvertiseHost); h != "" {
		if ip := parseUsableIPv4(h); ip != "" {
			return formatBaseURL(ip, srv.Port, srv.TLS.Enabled)
		}
		if !isLoopbackHostname(h) {
			return formatBaseURL(h, srv.Port, srv.TLS.Enabled)
		}
	}
	if u := baseURLFromRequest(r); u != "" {
		return u
	}
	ip := resolveAdvertiseIPv4(strings.TrimSpace(srv.Host))
	if ip == "" {
		return ""
	}
	return formatBaseURL(ip, srv.Port, srv.TLS.Enabled)
}

func baseURLFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	}
	if host == "" {
		return ""
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	if isLoopbackHostname(hostname) {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func resolveAdvertiseIPv4(listenHost string) string {
	if ip := parseUsableIPv4(listenHost); ip != "" {
		return ip
	}
	if ip := preferredOutboundIPv4(); ip != "" {
		return ip
	}
	return bestPrivateIPv4FromInterfaces()
}

func preferredOutboundIPv4() string {
	targets := []string{
		"192.168.255.255:1",
		"10.255.255.255:1",
		"172.31.255.255:1",
		"8.8.8.8:80",
	}
	for _, target := range targets {
		conn, err := net.Dial("udp4", target)
		if err != nil {
			continue
		}
		_ = conn.Close()
		ua, ok := conn.LocalAddr().(*net.UDPAddr)
		if !ok || ua.IP == nil {
			continue
		}
		if ip := usablePrivateIPv4(ua.IP); ip != "" {
			return ip
		}
	}
	return ""
}

type lanCandidate struct {
	ip    string
	score int
}

func bestPrivateIPv4FromInterfaces() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var candidates []lanCandidate
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		if isVirtualInterface(name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ipStr := usablePrivateIPv4(ip)
			if ipStr == "" {
				continue
			}
			candidates = append(candidates, lanCandidate{ip: ipStr, score: interfaceScore(name, ipStr)})
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].ip < candidates[j].ip
	})
	return candidates[0].ip
}

func interfaceScore(name, ip string) int {
	score := 0
	switch {
	case strings.HasPrefix(name, "wlan"), strings.HasPrefix(name, "wlp"), strings.HasPrefix(name, "wl"):
		score += 100
	case strings.HasPrefix(name, "en"), strings.HasPrefix(name, "eth"):
		score += 90
	default:
		score += 10
	}
	if strings.HasPrefix(ip, "192.168.") {
		score += 20
	} else if strings.HasPrefix(ip, "10.") {
		score += 10
	}
	return score
}

func isVirtualInterface(name string) bool {
	prefixes := []string{
		"docker", "veth", "br-", "virbr", "tun", "tap", "wg", "tailscale", "cni", "flannel",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func parseUsableIPv4(host string) string {
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		return ""
	}
	return usablePrivateIPv4(net.ParseIP(host))
}

func usablePrivateIPv4(ip net.IP) string {
	if ip == nil {
		return ""
	}
	ip4 := ip.To4()
	if ip4 == nil || ip4.IsLoopback() || !ip4.IsPrivate() {
		return ""
	}
	if ip4[0] == 169 && ip4[1] == 254 {
		return ""
	}
	return ip4.String()
}

func formatBaseURL(host string, port int, tlsEnabled bool) string {
	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	if port <= 0 {
		port = 8080
	}
	if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		return fmt.Sprintf("%s://%s", scheme, host)
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}

func isLoopbackHostname(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// IsLoopbackURL reports whether a base URL points at localhost (unsuitable for QR field u).
func IsLoopbackURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		lower := strings.ToLower(raw)
		return strings.Contains(lower, "localhost") || strings.Contains(lower, "127.0.0.1")
	}
	host := u.Hostname()
	return isLoopbackHostname(host)
}
