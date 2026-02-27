package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/y0f/asura/internal/httputil"
	"github.com/y0f/asura/internal/incident"
	"github.com/y0f/asura/internal/notifier"
	"github.com/y0f/asura/internal/storage"
	"github.com/y0f/asura/internal/validate"
)

func (h *Handler) Notifications(w http.ResponseWriter, r *http.Request) {
	channels, err := h.store.ListNotificationChannels(r.Context())
	if err != nil {
		h.logger.Error("web: list notifications", "error", err)
	}

	pd := h.newPageData(r, "Notifications", "notifications")
	pd.Data = channels
	h.render(w, "notifications/list.html", pd)
}

func (h *Handler) NotificationCreate(w http.ResponseWriter, r *http.Request) {
	ch := h.parseNotificationForm(r)

	if err := validate.ValidateNotificationChannel(ch); err != nil {
		h.setFlash(w, err.Error())
		h.redirect(w, r, "/notifications")
		return
	}

	if err := h.store.CreateNotificationChannel(r.Context(), ch); err != nil {
		h.logger.Error("web: create notification", "error", err)
		h.setFlash(w, "Failed to create channel")
		h.redirect(w, r, "/notifications")
		return
	}

	h.setFlash(w, "Notification channel created")
	h.redirect(w, r, "/notifications")
}

func (h *Handler) NotificationUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := httputil.ParseID(r)
	ch := h.parseNotificationForm(r)
	ch.ID = id

	if err := validate.ValidateNotificationChannel(ch); err != nil {
		h.setFlash(w, err.Error())
		h.redirect(w, r, "/notifications")
		return
	}

	if err := h.store.UpdateNotificationChannel(r.Context(), ch); err != nil {
		h.logger.Error("web: update notification", "error", err)
		h.setFlash(w, "Failed to update channel")
		h.redirect(w, r, "/notifications")
		return
	}

	h.setFlash(w, "Notification channel updated")
	h.redirect(w, r, "/notifications")
}

func (h *Handler) NotificationDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := httputil.ParseID(r)
	if err := h.store.DeleteNotificationChannel(r.Context(), id); err != nil {
		h.logger.Error("web: delete notification", "error", err)
	}
	h.setFlash(w, "Notification channel deleted")
	h.redirect(w, r, "/notifications")
}

func (h *Handler) NotificationTest(w http.ResponseWriter, r *http.Request) {
	id, _ := httputil.ParseID(r)
	ch, err := h.store.GetNotificationChannel(r.Context(), id)
	if err != nil {
		h.setFlash(w, "Channel not found")
		h.redirect(w, r, "/notifications")
		return
	}

	testInc := &storage.Incident{
		ID:          0,
		MonitorName: "Test Monitor",
		Status:      incident.StatusOpen,
		Cause:       "Test notification from Asura",
	}

	if err := h.notifier.SendTest(ch, testInc); err != nil {
		h.setFlash(w, "Test failed: "+err.Error())
	} else {
		h.setFlash(w, "Test notification sent")
	}
	h.redirect(w, r, "/notifications")
}

func (h *Handler) parseNotificationForm(r *http.Request) *storage.NotificationChannel {
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
		"event_incident_reminder",
		"event_content_changed",
	}
	eventValues := []string{
		"incident.created",
		"incident.resolved",
		"incident.acknowledged",
		"incident.reminder",
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
		return assembleEmailSettings(r)
	case "ntfy":
		return assembleNtfySettings(r)
	case "teams", "pagerduty", "opsgenie", "pushover":
		return assembleExtendedSettings(r, chType)
	default:
		return json.RawMessage("{}")
	}
}

func assembleEmailSettings(r *http.Request) json.RawMessage {
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
}

func assembleNtfySettings(r *http.Request) json.RawMessage {
	s := notifier.NtfySettings{
		ServerURL: r.FormValue("notif_ntfy_server_url"),
		Topic:     r.FormValue("notif_ntfy_topic"),
		Tags:      r.FormValue("notif_ntfy_tags"),
		ClickURL:  r.FormValue("notif_ntfy_click_url"),
	}
	if v := r.FormValue("notif_ntfy_priority"); v != "" {
		s.Priority, _ = strconv.Atoi(v)
	}
	b, _ := json.Marshal(s)
	return b
}

func assembleExtendedSettings(r *http.Request, chType string) json.RawMessage {
	var v any
	switch chType {
	case "teams":
		v = notifier.TeamsSettings{
			WebhookURL: r.FormValue("notif_teams_webhook_url"),
		}
	case "pagerduty":
		v = notifier.PagerDutySettings{
			RoutingKey: r.FormValue("notif_pagerduty_routing_key"),
		}
	case "opsgenie":
		v = notifier.OpsgenieSettings{
			APIKey: r.FormValue("notif_opsgenie_api_key"),
			Region: r.FormValue("notif_opsgenie_region"),
		}
	case "pushover":
		s := notifier.PushoverSettings{
			UserKey:  r.FormValue("notif_pushover_user_key"),
			AppToken: r.FormValue("notif_pushover_app_token"),
			Sound:    r.FormValue("notif_pushover_sound"),
			Device:   r.FormValue("notif_pushover_device"),
		}
		if p := r.FormValue("notif_pushover_priority"); p != "" {
			s.Priority, _ = strconv.Atoi(p)
		}
		v = s
	}
	b, _ := json.Marshal(v)
	return b
}
