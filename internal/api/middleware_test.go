package api

import (
	"net"
	"net/http"
	"testing"
)

func TestExtractIP(t *testing.T) {
	_, trusted, _ := net.ParseCIDR("127.0.0.0/8")

	tests := []struct {
		name        string
		remoteAddr  string
		xRealIP     string
		xff         string
		trustedNets []net.IPNet
		want        string
	}{
		{
			"direct untrusted",
			"1.2.3.4:1234", "", "", nil,
			"1.2.3.4",
		},
		{
			"trusted with X-Real-IP",
			"127.0.0.1:1234", "10.0.0.1", "",
			[]net.IPNet{*trusted},
			"10.0.0.1",
		},
		{
			"trusted with XFF",
			"127.0.0.1:1234", "", "10.0.0.1, 127.0.0.1",
			[]net.IPNet{*trusted},
			"10.0.0.1",
		},
		{
			"untrusted ignores X-Real-IP",
			"1.2.3.4:1234", "10.0.0.1", "",
			[]net.IPNet{*trusted},
			"1.2.3.4",
		},
		{
			"X-Real-IP takes priority over XFF",
			"127.0.0.1:1234", "10.0.0.1", "192.168.1.1",
			[]net.IPNet{*trusted},
			"10.0.0.1",
		},
		{
			"no port in remote addr",
			"1.2.3.4", "", "", nil,
			"1.2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.xRealIP != "" {
				r.Header.Set("X-Real-IP", tt.xRealIP)
			}
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			got := extractIP(r, tt.trustedNets)
			if got != tt.want {
				t.Errorf("extractIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsAllowedOrigin(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		allowed []string
		want    bool
	}{
		{"wildcard", "https://any.com", []string{"*"}, true},
		{"exact match", "https://example.com", []string{"https://example.com"}, true},
		{"no match", "https://evil.com", []string{"https://example.com"}, false},
		{"empty list", "https://example.com", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAllowedOrigin(tt.origin, tt.allowed)
			if got != tt.want {
				t.Errorf("isAllowedOrigin(%q, %v) = %v, want %v", tt.origin, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestBuildFrameAncestorsDirective(t *testing.T) {
	tests := []struct {
		name      string
		ancestors []string
		want      string
	}{
		{"empty", nil, "frame-ancestors 'none'"},
		{"self", []string{"self"}, "frame-ancestors 'self'"},
		{"custom", []string{"https://example.com"}, "frame-ancestors https://example.com"},
		{"mixed", []string{"self", "https://example.com"}, "frame-ancestors 'self' https://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildFrameAncestorsDirective(tt.ancestors)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRateLimiterAllow(t *testing.T) {
	rl := newRateLimiter(10, 10)

	for i := 0; i < 10; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	if rl.allow("1.2.3.4") {
		t.Fatal("11th request should be rate limited")
	}

	if !rl.allow("5.6.7.8") {
		t.Fatal("different IP should be allowed")
	}
}
