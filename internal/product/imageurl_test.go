package product

import "testing"

func TestSafeImageURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "open food facts", raw: "https://images.openfoodfacts.org/thumb.jpg", want: "https://images.openfoodfacts.org/thumb.jpg"},
		{name: "public http", raw: "http://example.com/image.jpg", want: "http://example.com/image.jpg"},
		{name: "loopback", raw: "http://127.0.0.1/secret.jpg", want: ""},
		{name: "ipv6 loopback", raw: "https://[::1]/secret.jpg", want: ""},
		{name: "lan", raw: "http://192.168.1.1/router.jpg", want: ""},
		{name: "rfc1918 10", raw: "http://10.0.0.1/x", want: ""},
		{name: "link local metadata", raw: "http://169.254.169.254/latest/meta-data", want: ""},
		{name: "cgnat", raw: "https://100.64.0.1/x", want: ""},
		{name: "decimal loopback", raw: "http://2130706433/", want: ""},
		{name: "short loopback", raw: "http://127.1/", want: ""},
		{name: "javascript", raw: "javascript:alert(1)", want: ""},
		{name: "localhost", raw: "https://localhost/x", want: ""},
		{name: "local tld", raw: "https://printer.local/x", want: ""},
		{name: "userinfo", raw: "https://user:pass@example.com/x", want: ""},
		{name: "single label", raw: "https://intranet/x", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeImageURL(tt.raw); got != tt.want {
				t.Errorf("SafeImageURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
