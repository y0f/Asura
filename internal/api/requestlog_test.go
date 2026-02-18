package api

import (
	"testing"
)

func TestClassifyRoute(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/", "web"},
		{"/monitors", "web"},
		{"/monitors/1", "web"},
		{"/incidents", "web"},
		{"/logs", "web"},
		{"/api/v1/monitors", "api"},
		{"/api/v1/monitors/1", "api"},
		{"/api/v1/incidents", "api"},
		{"/api/v1/badge/1/status", "badge"},
		{"/api/v1/badge/1/uptime", "badge"},
		{"/login", "auth"},
		{"/logout", "auth"},
		{"/metrics", "metrics"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := classifyRoute(tt.path)
			if got != tt.want {
				t.Errorf("classifyRoute(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractMonitorID(t *testing.T) {
	tests := []struct {
		path string
		want int64
	}{
		{"/monitors/4", 4},
		{"/monitors/123", 123},
		{"/api/v1/monitors/7", 7},
		{"/api/v1/monitors/7/checks", 7},
		{"/api/v1/badge/42/status", 42},
		{"/api/v1/badge/42/uptime", 42},
		{"/", 0},
		{"/monitors", 0},
		{"/api/v1/monitors", 0},
		{"/incidents/5", 0},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractMonitorID(tt.path)
			if got != tt.want {
				t.Errorf("extractMonitorID(%q) = %d, want %d", tt.path, got, tt.want)
			}
		})
	}
}

func TestShouldSkipLog(t *testing.T) {
	tests := []struct {
		path     string
		basePath string
		want     bool
	}{
		{"/static/htmx.min.js", "", true},
		{"/static/logo.gif", "", true},
		{"/api/v1/health", "", true},
		{"/myapp/static/htmx.min.js", "/myapp", true},
		{"/myapp/api/v1/health", "/myapp", true},
		{"/", "", false},
		{"/monitors", "", false},
		{"/api/v1/monitors", "", false},
		{"/logs", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := shouldSkipLog(tt.path, tt.basePath)
			if got != tt.want {
				t.Errorf("shouldSkipLog(%q, %q) = %v, want %v", tt.path, tt.basePath, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Error("short string should not be truncated")
	}
	if truncate("hello world", 5) != "hello" {
		t.Error("long string should be truncated to maxLen")
	}
	if truncate("", 5) != "" {
		t.Error("empty string should stay empty")
	}
}
