package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asura-monitor/asura/internal/storage"
)

var validMonitorTypes = map[string]bool{
	"http": true, "tcp": true, "dns": true,
	"icmp": true, "tls": true, "websocket": true, "command": true,
}

var validNotificationTypes = map[string]bool{
	"webhook": true, "email": true, "telegram": true,
	"discord": true, "slack": true,
}

var validNotificationEvents = map[string]bool{
	"incident.created":      true,
	"incident.acknowledged": true,
	"incident.resolved":     true,
	"content.changed":       true,
}

func validateMonitor(m *storage.Monitor) error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(m.Name) > 255 {
		return fmt.Errorf("name must be at most 255 characters")
	}
	if !validMonitorTypes[m.Type] {
		return fmt.Errorf("type must be one of: http, tcp, dns, icmp, tls, websocket, command")
	}
	if strings.TrimSpace(m.Target) == "" {
		return fmt.Errorf("target is required")
	}
	if len(m.Target) > 2048 {
		return fmt.Errorf("target must be at most 2048 characters")
	}
	if m.Interval < 5 {
		return fmt.Errorf("interval must be at least 5 seconds")
	}
	if m.Interval > 86400 {
		return fmt.Errorf("interval must be at most 86400 seconds")
	}
	if m.Timeout < 1 {
		return fmt.Errorf("timeout must be at least 1 second")
	}
	if m.Timeout > 300 {
		return fmt.Errorf("timeout must be at most 300 seconds")
	}
	if m.FailureThreshold < 1 {
		return fmt.Errorf("failure_threshold must be at least 1")
	}
	if m.SuccessThreshold < 1 {
		return fmt.Errorf("success_threshold must be at least 1")
	}

	for _, tag := range m.Tags {
		if len(tag) > 50 {
			return fmt.Errorf("tag must be at most 50 characters")
		}
	}
	if len(m.Tags) > 20 {
		return fmt.Errorf("at most 20 tags allowed")
	}

	// Validate settings JSON if present
	if len(m.Settings) > 0 && string(m.Settings) != "{}" {
		var s map[string]interface{}
		if err := json.Unmarshal(m.Settings, &s); err != nil {
			return fmt.Errorf("settings must be a valid JSON object")
		}
	}

	// Validate assertions JSON if present
	if len(m.Assertions) > 0 && string(m.Assertions) != "[]" {
		var a []interface{}
		if err := json.Unmarshal(m.Assertions, &a); err != nil {
			return fmt.Errorf("assertions must be a valid JSON array")
		}
	}

	return nil
}

func validateNotificationChannel(ch *storage.NotificationChannel) error {
	if strings.TrimSpace(ch.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(ch.Name) > 255 {
		return fmt.Errorf("name must be at most 255 characters")
	}
	if !validNotificationTypes[ch.Type] {
		return fmt.Errorf("type must be one of: webhook, email, telegram, discord, slack")
	}
	if len(ch.Settings) == 0 {
		return fmt.Errorf("settings is required")
	}
	for _, ev := range ch.Events {
		if !validNotificationEvents[ev] {
			return fmt.Errorf("invalid event: %s", ev)
		}
	}
	return nil
}

func validateMaintenanceWindow(mw *storage.MaintenanceWindow) error {
	if strings.TrimSpace(mw.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if mw.StartTime.IsZero() {
		return fmt.Errorf("start_time is required")
	}
	if mw.EndTime.IsZero() {
		return fmt.Errorf("end_time is required")
	}
	if !mw.EndTime.After(mw.StartTime) {
		return fmt.Errorf("end_time must be after start_time")
	}
	if mw.Recurring != "" && mw.Recurring != "daily" && mw.Recurring != "weekly" && mw.Recurring != "monthly" {
		return fmt.Errorf("recurring must be one of: daily, weekly, monthly")
	}
	return nil
}
