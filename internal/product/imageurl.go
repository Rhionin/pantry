package product

import (
	"net"
	"net/url"
	"strings"
)

// SafeImageURL returns raw when a browser can load it as an image without
// being sent to a loopback, link-local, or private address. Anything else
// becomes an empty string so the page shows no picture instead of contacting
// that address. An empty input stays empty.
func SafeImageURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	host := parsed.Hostname()
	if host == "" || !publicImageHost(host) {
		return ""
	}
	return raw
}

func publicImageHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return publicImageIP(ip)
	}
	lower := strings.ToLower(strings.TrimSuffix(host, "."))
	switch lower {
	case "localhost", "localhost.localdomain", "metadata.google.internal":
		return false
	}
	if strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".localhost") || strings.HasSuffix(lower, ".internal") {
		return false
	}
	// A name with no letter is a numeric address browsers may still treat as
	// an IP (127.1, or a single decimal such as 2130706433).
	if !strings.ContainsAny(lower, "abcdefghijklmnopqrstuvwxyz") {
		return false
	}
	return strings.Contains(lower, ".")
}

func publicImageIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || isCGNAT(ip) {
		return false
	}
	return ip.IsGlobalUnicast()
}

func isCGNAT(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 100 && ip4[1]&0xc0 == 64
}
