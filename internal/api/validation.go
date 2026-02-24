package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/y0f/Asura/internal/incident"
	"github.com/y0f/Asura/internal/storage"
)

var validMonitorTypes = map[string]bool{
	"http": true, "tcp": true, "dns": true,
	"icmp": true, "tls": true, "websocket": true, "command": true,
	"heartbeat": true, "docker": true, "domain": true,
	"grpc": true, "mqtt": true,
}

var validIncidentStatuses = map[string]bool{
	incident.StatusOpen: true, incident.StatusAcknowledged: true, incident.StatusResolved: true,
}

var validNotificationTypes = map[string]bool{
	"webhook": true, "email": true, "telegram": true,
	"discord": true, "slack": true, "ntfy": true,
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
		return fmt.Errorf("type must be one of: http, tcp, dns, icmp, tls, websocket, command, heartbeat, docker, domain, grpc, mqtt")
	}
	if m.Type == "heartbeat" {
		return nil
	}
	if strings.TrimSpace(m.Target) == "" {
		return fmt.Errorf("target is required")
	}
	if len(m.Target) > 2048 {
		return fmt.Errorf("target must be at most 2048 characters")
	}
	return validateMonitorLimits(m)
}

func validateMonitorLimits(m *storage.Monitor) error {
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
	return validateMonitorJSON(m)
}

func validateMonitorJSON(m *storage.Monitor) error {
	if len(m.Settings) > 0 && string(m.Settings) != "{}" {
		var s map[string]interface{}
		if err := json.Unmarshal(m.Settings, &s); err != nil {
			return fmt.Errorf("settings must be a valid JSON object")
		}
	}
	if len(m.Assertions) > 0 && string(m.Assertions) != "[]" {
		var a []interface{}
		if err := json.Unmarshal(m.Assertions, &a); err != nil {
			return fmt.Errorf("assertions must be a valid JSON array")
		}
	}
	if m.Type == "docker" {
		return validateDockerSettings(m)
	}
	return nil
}

func validateDockerSettings(m *storage.Monitor) error {
	var ds storage.DockerSettings
	if len(m.Settings) > 0 {
		if err := json.Unmarshal(m.Settings, &ds); err != nil {
			return fmt.Errorf("invalid docker settings: %w", err)
		}
	}
	name := ds.ContainerName
	if name == "" {
		name = m.Target
	}
	if strings.ContainsAny(name, "/\\..") {
		return fmt.Errorf("container name contains invalid characters")
	}
	if ds.SocketPath != "" {
		if strings.Contains(ds.SocketPath, "..") {
			return fmt.Errorf("socket path must not contain path traversal")
		}
		if !strings.HasPrefix(ds.SocketPath, "/") {
			return fmt.Errorf("socket path must be an absolute path")
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
		return fmt.Errorf("type must be one of: webhook, email, telegram, discord, slack, ntfy")
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

func validateMonitorGroup(g *storage.MonitorGroup) error {
	if strings.TrimSpace(g.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(g.Name) > 255 {
		return fmt.Errorf("name must be at most 255 characters")
	}
	return nil
}

func validateStatusPage(sp *storage.StatusPage) error {
	if strings.TrimSpace(sp.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if len(sp.Title) > 200 {
		return fmt.Errorf("title must be at most 200 characters")
	}
	if strings.TrimSpace(sp.Slug) == "" {
		return fmt.Errorf("slug is required")
	}
	sp.Slug = validateSlug(sp.Slug)
	if len(sp.Description) > 1000 {
		return fmt.Errorf("description must be at most 1000 characters")
	}
	sp.CustomCSS = sanitizeCSS(sp.CustomCSS)
	return nil
}

func validateProxy(p *storage.Proxy) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(p.Name) > 255 {
		return fmt.Errorf("name must be at most 255 characters")
	}
	if p.Protocol != "http" && p.Protocol != "socks5" {
		return fmt.Errorf("protocol must be http or socks5")
	}
	if strings.TrimSpace(p.Host) == "" {
		return fmt.Errorf("host is required")
	}
	if len(p.Host) > 255 {
		return fmt.Errorf("host must be at most 255 characters")
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
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
