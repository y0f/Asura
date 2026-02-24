package checker

import (
	"testing"
	"time"
)

func TestExtractTLD(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{"example.com", ".com"},
		{"sub.example.org", ".org"},
		{"test.io", ".io"},
		{"a.b.c.dev", ".dev"},
		{"localhost", "localhost"},
	}
	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := extractTLD(tt.domain)
			if got != tt.want {
				t.Errorf("extractTLD(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}

func TestWhoisServerForTLD(t *testing.T) {
	tests := []struct {
		tld  string
		want string
	}{
		{".com", "whois.verisign-grs.com"},
		{".net", "whois.verisign-grs.com"},
		{".org", "whois.pir.org"},
		{".io", "whois.nic.io"},
		{".dev", "whois.nic.google"},
		{".xyz", "whois.nic.xyz"},
		{".invalid", ""},
	}
	for _, tt := range tests {
		t.Run(tt.tld, func(t *testing.T) {
			got := whoisServerForTLD(tt.tld)
			if got != tt.want {
				t.Errorf("whoisServerForTLD(%q) = %q, want %q", tt.tld, got, tt.want)
			}
		})
	}
}

func TestParseWhoisExpiry(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantYear int
		wantErr  bool
	}{
		{
			name:     "ICANN standard format",
			response: "Registry Expiry Date: 2025-08-13T04:00:00Z\nother stuff",
			wantYear: 2025,
		},
		{
			name:     "Registrar expiration",
			response: "Registrar Registration Expiration Date: 2026-03-15T12:00:00Z\n",
			wantYear: 2026,
		},
		{
			name:     "Expiry Date format",
			response: "Expiry Date: 2027-01-01\n",
			wantYear: 2027,
		},
		{
			name:     "Expiration Date format",
			response: "Expiration Date: 2028-06-30\n",
			wantYear: 2028,
		},
		{
			name:     "paid-till format",
			response: "paid-till: 2025-12-31T00:00:00Z\n",
			wantYear: 2025,
		},
		{
			name:     "expires format",
			response: "expires: 2029-11-20\n",
			wantYear: 2029,
		},
		{
			name:     "date with dots",
			response: "Expiry Date: 2026.05.10\n",
			wantYear: 2026,
		},
		{
			name:     "no expiry found",
			response: "Domain Name: example.com\nRegistrar: Example Inc\n",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWhoisExpiry(tt.response)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseWhoisExpiry() expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWhoisExpiry() unexpected error: %v", err)
			}
			if got.Year() != tt.wantYear {
				t.Errorf("parseWhoisExpiry() year = %d, want %d", got.Year(), tt.wantYear)
			}
		})
	}
}

func TestDomainExpiryThresholds(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		expiry     time.Time
		warnDays   int
		wantStatus string
	}{
		{
			name:       "expired",
			expiry:     now.Add(-24 * time.Hour),
			warnDays:   30,
			wantStatus: "down",
		},
		{
			name:       "expiring within threshold",
			expiry:     now.Add(15 * 24 * time.Hour),
			warnDays:   30,
			wantStatus: "degraded",
		},
		{
			name:       "valid and far from expiry",
			expiry:     now.Add(365 * 24 * time.Hour),
			warnDays:   30,
			wantStatus: "up",
		},
		{
			name:       "exactly at threshold",
			expiry:     now.Add(30 * 24 * time.Hour),
			warnDays:   30,
			wantStatus: "degraded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			daysUntil := int(time.Until(tt.expiry).Hours() / 24)
			var status string
			if daysUntil <= 0 {
				status = "down"
			} else if daysUntil <= tt.warnDays {
				status = "degraded"
			} else {
				status = "up"
			}
			if status != tt.wantStatus {
				t.Errorf("threshold check: days=%d, warnDays=%d, got %q, want %q", daysUntil, tt.warnDays, status, tt.wantStatus)
			}
		})
	}
}
