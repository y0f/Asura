package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/y0f/Asura/internal/notifier"
	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleWebNotifications(w http.ResponseWriter, r *http.Request) {
	channels, err := s.store.ListNotificationChannels(r.Context())
	if err != nil {
		s.logger.Error("web: list notifications", "error", err)
	}

	pd := s.newPageData(r, "Notifications", "notifications")
	pd.Data = channels
	s.render(w, "notifications.html", pd)
}

func (s *Server) handleWebNotificationCreate(w http.ResponseWriter, r *http.Request) {
	ch := s.parseNotificationForm(r)

	if err := validateNotificationChannel(ch); err != nil {
		s.setFlash(w, err.Error())
		s.redirect(w, r, "/notifications")
		return
	}

	if err := s.store.CreateNotificationChannel(r.Context(), ch); err != nil {
		s.logger.Error("web: create notification", "error", err)
		s.setFlash(w, "Failed to create channel")
		s.redirect(w, r, "/notifications")
		return
	}

	s.setFlash(w, "Notification channel created")
	s.redirect(w, r, "/notifications")
}

func (s *Server) handleWebNotificationUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	ch := s.parseNotificationForm(r)
	ch.ID = id

	if err := validateNotificationChannel(ch); err != nil {
		s.setFlash(w, err.Error())
		s.redirect(w, r, "/notifications")
		return
	}

	if err := s.store.UpdateNotificationChannel(r.Context(), ch); err != nil {
		s.logger.Error("web: update notification", "error", err)
		s.setFlash(w, "Failed to update channel")
		s.redirect(w, r, "/notifications")
		return
	}

	s.setFlash(w, "Notification channel updated")
	s.redirect(w, r, "/notifications")
}

func (s *Server) handleWebNotificationDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	if err := s.store.DeleteNotificationChannel(r.Context(), id); err != nil {
		s.logger.Error("web: delete notification", "error", err)
	}
	s.setFlash(w, "Notification channel deleted")
	s.redirect(w, r, "/notifications")
}

func (s *Server) handleWebNotificationTest(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	ch, err := s.store.GetNotificationChannel(r.Context(), id)
	if err != nil {
		s.setFlash(w, "Channel not found")
		s.redirect(w, r, "/notifications")
		return
	}

	testInc := &storage.Incident{
		ID:          0,
		MonitorName: "Test Monitor",
		Status:      "open",
		Cause:       "Test notification from Asura",
	}

	if err := s.notifier.SendTest(ch, testInc); err != nil {
		s.setFlash(w, "Test failed: "+err.Error())
	} else {
		s.setFlash(w, "Test notification sent")
	}
	s.redirect(w, r, "/notifications")
}

func (s *Server) parseNotificationForm(r *http.Request) *storage.NotificationChannel {
	r.ParseForm()

	ch := &storage.NotificationChannel{
		Name:    r.FormValue("name"),
		Type:    r.FormValue("type"),
		Enabled: r.FormValue("enabled") == "on",
	}

	if r.FormValue("notif_settings_mode") == "json" {
		if raw := strings.TrimSpace(r.FormValue("settings_json")); raw != "" && json.Valid([]byte(raw)) {
			ch.Settings = json.RawMessage(raw)
		}
	} else {
		ch.Settings = assembleNotificationSettings(r, ch.Type)
	}

	ch.Events = parseNotificationEvents(r)

	return ch
}

func parseNotificationEvents(r *http.Request) []string {
	var events []string
	eventKeys := []string{
		"event_incident_created",
		"event_incident_resolved",
		"event_incident_acknowledged",
		"event_content_changed",
	}
	eventValues := []string{
		"incident.created",
		"incident.resolved",
		"incident.acknowledged",
		"content.changed",
	}
	for i, key := range eventKeys {
		if r.FormValue(key) == "on" {
			events = append(events, eventValues[i])
		}
	}
	if len(events) > 0 {
		return events
	}

	if csv := r.FormValue("events"); csv != "" {
		for _, e := range strings.Split(csv, ",") {
			if trimmed := strings.TrimSpace(e); trimmed != "" {
				events = append(events, trimmed)
			}
		}
	}
	return events
}

func assembleNotificationSettings(r *http.Request, chType string) json.RawMessage {
	switch chType {
	case "webhook":
		s := notifier.WebhookSettings{
			URL:    r.FormValue("notif_webhook_url"),
			Secret: r.FormValue("notif_webhook_secret"),
		}
		b, _ := json.Marshal(s)
		return b
	case "telegram":
		s := notifier.TelegramSettings{
			BotToken: r.FormValue("notif_telegram_bot_token"),
			ChatID:   r.FormValue("notif_telegram_chat_id"),
		}
		b, _ := json.Marshal(s)
		return b
	case "discord":
		s := notifier.DiscordSettings{
			WebhookURL: r.FormValue("notif_discord_webhook_url"),
		}
		b, _ := json.Marshal(s)
		return b
	case "slack":
		s := notifier.SlackSettings{
			WebhookURL: r.FormValue("notif_slack_webhook_url"),
			Channel:    r.FormValue("notif_slack_channel"),
		}
		b, _ := json.Marshal(s)
		return b
	case "email":
		port, _ := strconv.Atoi(r.FormValue("notif_email_port"))
		s := notifier.EmailSettings{
			Host:     r.FormValue("notif_email_host"),
			Port:     port,
			Username: r.FormValue("notif_email_username"),
			Password: r.FormValue("notif_email_password"),
			From:     r.FormValue("notif_email_from"),
		}
		if toStr := strings.TrimSpace(r.FormValue("notif_email_to")); toStr != "" {
			for _, addr := range strings.Split(toStr, ",") {
				if trimmed := strings.TrimSpace(addr); trimmed != "" {
					s.To = append(s.To, trimmed)
				}
			}
		}
		b, _ := json.Marshal(s)
		return b
	default:
		return json.RawMessage("{}")
	}
}
