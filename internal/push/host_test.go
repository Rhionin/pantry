package push

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostsIgnoreASchemeAndADuplicateName(t *testing.T) {
	cfg := ConfigFromEnv(func(key string) string {
		switch key {
		case "PUBLIC_HOST":
			return " stockbot.rhionin.com "
		case "LEGACY_PUBLIC_HOST":
			return "pantry.rhionin.com"
		default:
			return ""
		}
	})
	if cfg.publicHost() != "stockbot.rhionin.com" || cfg.legacyHost() != "pantry.rhionin.com" {
		t.Fatalf("hosts = %q %q", cfg.publicHost(), cfg.legacyHost())
	}
	if !cfg.RejectsOrigin("https://pantry.rhionin.com") || cfg.DeliversTo("pantry.rhionin.com") {
		t.Fatal("previous host was accepted")
	}
	if !cfg.DeliversTo("stockbot.rhionin.com:443") || !cfg.DeliversTo("") {
		t.Fatal("current host or a blank origin was skipped")
	}
	if cfg.DeliversTo("192.168.1.203") {
		t.Fatal("another host was selected while a public host is set")
	}

	cfg.LegacyHost = "https://pantry.rhionin.com"
	if cfg.legacyHost() != "" || cfg.RejectsOrigin("pantry.rhionin.com") {
		t.Fatal("a legacy value with a scheme was used")
	}
	cfg.LegacyHost = "Stockbot.rhionin.com."
	cfg.PublicHost = "stockbot.rhionin.com"
	if cfg.legacyHost() != "" || cfg.RejectsOrigin("stockbot.rhionin.com") {
		t.Fatal("the current host was treated as the previous one")
	}
	if ConfigFromEnv(nil).publicHost() != "" {
		t.Fatal("nil getenv produced a host")
	}
}

func TestOriginHostPrefersTheBrowserOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/push/subscriptions", nil)
	req.Host = "pantry:8080"
	req.Header.Set("X-Forwarded-Host", "stockbot.rhionin.com, evil.example")
	req.Header.Set("Origin", "https://stockbot.rhionin.com")
	if got := OriginHost(req); got != "stockbot.rhionin.com" {
		t.Fatalf("origin = %q", got)
	}

	req.Header.Del("Origin")
	if got := OriginHost(req); got != "stockbot.rhionin.com" {
		t.Fatalf("forwarded = %q", got)
	}
	req.Header.Del("X-Forwarded-Host")
	req.Host = "pantry.local:8080"
	if got := OriginHost(req); got != "pantry.local" {
		t.Fatalf("host = %q", got)
	}
	req.Header.Set("Origin", "null")
	if got := OriginHost(req); got != "pantry.local" {
		t.Fatalf("null origin = %q", got)
	}
	if OriginHost(nil) != "" {
		t.Fatal("nil request produced a host")
	}
}
