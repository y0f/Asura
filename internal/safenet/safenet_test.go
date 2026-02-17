package safenet

import (
	"net"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"100.64.0.1", true},
		{"0.0.0.0", true},
		{"::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},
		{"2606:4700:4700::1111", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("failed to parse IP: %s", tt.ip)
		}
		got := IsPrivateIP(ip)
		if got != tt.private {
			t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
		}
	}
}

func TestDialControl(t *testing.T) {
	err := DialControl("tcp", "127.0.0.1:8080", nil)
	if err == nil {
		t.Fatal("expected error for private IP")
	}

	err = DialControl("tcp", "8.8.8.8:53", nil)
	if err != nil {
		t.Fatalf("unexpected error for public IP: %v", err)
	}
}

func TestMaybeDialControl(t *testing.T) {
	if MaybeDialControl(true) != nil {
		t.Fatal("expected nil when allowPrivate is true")
	}
	if MaybeDialControl(false) == nil {
		t.Fatal("expected non-nil when allowPrivate is false")
	}
}
