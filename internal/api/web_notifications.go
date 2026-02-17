package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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

	if settings := r.FormValue("settings"); settings != "" {
		ch.Settings = json.RawMessage(settings)
	}

	if events := r.FormValue("events"); events != "" {
		for _, e := range strings.Split(events, ",") {
			if trimmed := strings.TrimSpace(e); trimmed != "" {
				ch.Events = append(ch.Events, trimmed)
			}
		}
	}

	return ch
}

func parseNotifID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}
