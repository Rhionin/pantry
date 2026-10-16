package push

import (
	"net/http"
	"net/url"
	"strings"
)

// canonicalHost is the hostname used to compare a subscription with
// PUBLIC_HOST and LEGACY_PUBLIC_HOST. Ports and a trailing dot are ignored.
func canonicalHost(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimSuffix(raw, "/")
	if raw == "" || strings.ContainsAny(raw, "/?#") {
		return ""
	}
	parsed, err := url.Parse("//" + raw)
	if err != nil {
		return ""
	}
	host := strings.TrimSuffix(parsed.Hostname(), ".")
	if host == "" {
		return ""
	}
	return host
}

// publicHost is the hostname whose browsers should receive alerts. A scheme
// is stripped so a value copied from a URL still matches the browser origin.
func (c Config) publicHost() string {
	return canonicalHost(c.PublicHost)
}

// legacyHost is the previous public hostname. A value equal to the current
// host, or one that includes a scheme, is ignored, matching the redirect setup.
func (c Config) legacyHost() string {
	raw := strings.TrimSpace(c.LegacyHost)
	if raw == "" || strings.Contains(raw, "://") {
		return ""
	}
	legacy := canonicalHost(raw)
	if legacy == "" || legacy == c.publicHost() {
		return ""
	}
	return legacy
}

// RejectsOrigin reports that this hostname is the previous public site.
func (c Config) RejectsOrigin(origin string) bool {
	legacy := c.legacyHost()
	return legacy != "" && canonicalHost(origin) == legacy
}

// DeliversTo reports whether an alert should be sent to a subscription from
// origin. An empty origin was stored before origins existed and still receives
// alerts. Any other hostname is skipped once PUBLIC_HOST is set.
func (c Config) DeliversTo(origin string) bool {
	if c.RejectsOrigin(origin) {
		return false
	}
	public := c.publicHost()
	host := canonicalHost(origin)
	if public == "" || host == "" {
		return true
	}
	return host == public
}

// OriginHost is the site hostname on a subscribe request. The browser Origin
// is the push subscription's origin. A proxy's forwarded host is next, then
// the request host. An empty result means the row is stored without an origin.
func OriginHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := ""
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && !strings.EqualFold(origin, "null") {
		if parsed, err := url.Parse(origin); err == nil {
			host = canonicalHost(parsed.Host)
		}
	}
	if host == "" {
		forwarded := r.Header.Get("X-Forwarded-Host")
		forwarded, _, _ = strings.Cut(forwarded, ",")
		host = canonicalHost(forwarded)
	}
	if host == "" {
		host = canonicalHost(r.Host)
	}
	if len(host) > 253 {
		return ""
	}
	return host
}
