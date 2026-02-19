package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/y0f/Asura/internal/storage"
)

func validMonitor() *storage.Monitor {
	return &storage.Monitor{
		Name:             "Test",
		Type:             "http",
		Target:           "https://example.com",
		Interval:         60,
		Timeout:          10,
		FailureThreshold: 3,
		SuccessThreshold: 1,
		Tags:             []string{},
	}
}

func TestValidateMonitor(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(m *storage.Monitor)
		wantErr string
	}{
		{"valid", func(m *storage.Monitor) {}, ""},
		{"empty name", func(m *storage.Monitor) { m.Name = "" }, "name is required"},
		{"blank name", func(m *storage.Monitor) { m.Name = "   " }, "name is required"},
		{"name too long", func(m *storage.Monitor) { m.Name = strings.Repeat("a", 256) }, "at most 255"},
		{"invalid type", func(m *storage.Monitor) { m.Type = "ftp" }, "type must be one of"},
		{"empty target", func(m *storage.Monitor) { m.Target = "" }, "target is required"},
		{"target too long", func(m *storage.Monitor) { m.Target = strings.Repeat("x", 2049) }, "at most 2048"},
		{"heartbeat no target", func(m *storage.Monitor) { m.Type = "heartbeat"; m.Target = "" }, ""},
		{"interval too low", func(m *storage.Monitor) { m.Interval = 4 }, "at least 5"},
		{"interval too high", func(m *storage.Monitor) { m.Interval = 86401 }, "at most 86400"},
		{"timeout too low", func(m *storage.Monitor) { m.Timeout = 0 }, "at least 1"},
		{"timeout too high", func(m *storage.Monitor) { m.Timeout = 301 }, "at most 300"},
		{"failure threshold zero", func(m *storage.Monitor) { m.FailureThreshold = 0 }, "at least 1"},
		{"success threshold zero", func(m *storage.Monitor) { m.SuccessThreshold = 0 }, "at least 1"},
		{"tag too long", func(m *storage.Monitor) { m.Tags = []string{strings.Repeat("t", 51)} }, "at most 50"},
		{"too many tags", func(m *storage.Monitor) {
			m.Tags = make([]string, 21)
			for i := range m.Tags {
				m.Tags[i] = "t"
			}
		}, "at most 20 tags"},
		{"invalid settings json", func(m *storage.Monitor) { m.Settings = json.RawMessage("not json") }, "valid JSON object"},
		{"invalid assertions json", func(m *storage.Monitor) { m.Assertions = json.RawMessage("not json") }, "valid JSON array"},
		{"valid settings", func(m *storage.Monitor) { m.Settings = json.RawMessage(`{"method":"GET"}`) }, ""},
		{"valid assertions", func(m *storage.Monitor) { m.Assertions = json.RawMessage(`[{"type":"status_code"}]`) }, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validMonitor()
			tt.modify(m)
			err := validateMonitor(m)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidateNotificationChannel(t *testing.T) {
	tests := []struct {
		name    string
		ch      *storage.NotificationChannel
		wantErr string
	}{
		{
			"valid",
			&storage.NotificationChannel{
				Name: "Hook", Type: "webhook",
				Settings: json.RawMessage(`{"url":"https://example.com"}`),
				Events:   []string{"incident.created"},
			},
			"",
		},
		{
			"empty name",
			&storage.NotificationChannel{Name: "", Type: "webhook", Settings: json.RawMessage("{}")},
			"name is required",
		},
		{
			"name too long",
			&storage.NotificationChannel{Name: strings.Repeat("n", 256), Type: "webhook", Settings: json.RawMessage("{}")},
			"at most 255",
		},
		{
			"invalid type",
			&storage.NotificationChannel{Name: "X", Type: "sms", Settings: json.RawMessage("{}")},
			"type must be one of",
		},
		{
			"empty settings",
			&storage.NotificationChannel{Name: "X", Type: "webhook"},
			"settings is required",
		},
		{
			"invalid event",
			&storage.NotificationChannel{
				Name: "X", Type: "webhook",
				Settings: json.RawMessage("{}"),
				Events:   []string{"bad.event"},
			},
			"invalid event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNotificationChannel(tt.ch)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidateMaintenanceWindow(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)

	tests := []struct {
		name    string
		mw      *storage.MaintenanceWindow
		wantErr string
	}{
		{
			"valid",
			&storage.MaintenanceWindow{Name: "MW", StartTime: now, EndTime: later},
			"",
		},
		{
			"valid recurring",
			&storage.MaintenanceWindow{Name: "MW", StartTime: now, EndTime: later, Recurring: "daily"},
			"",
		},
		{
			"empty name",
			&storage.MaintenanceWindow{StartTime: now, EndTime: later},
			"name is required",
		},
		{
			"zero start",
			&storage.MaintenanceWindow{Name: "MW", EndTime: later},
			"start_time is required",
		},
		{
			"zero end",
			&storage.MaintenanceWindow{Name: "MW", StartTime: now},
			"end_time is required",
		},
		{
			"end before start",
			&storage.MaintenanceWindow{Name: "MW", StartTime: later, EndTime: now},
			"end_time must be after",
		},
		{
			"invalid recurring",
			&storage.MaintenanceWindow{Name: "MW", StartTime: now, EndTime: later, Recurring: "yearly"},
			"recurring must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMaintenanceWindow(tt.mw)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}
