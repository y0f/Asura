package server

import (
	"testing"
)

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
