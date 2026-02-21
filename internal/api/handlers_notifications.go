package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/y0f/Asura/internal/incident"
	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	channels, err := s.store.ListNotificationChannels(r.Context())
	if err != nil {
		s.logger.Error("list notifications", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list notification channels")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": channels})
}

func (s *Server) handleCreateNotification(w http.ResponseWriter, r *http.Request) {
	var ch storage.NotificationChannel
	if err := readJSON(r, &ch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ch.Enabled = true

	if err := validateNotificationChannel(&ch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.CreateNotificationChannel(r.Context(), &ch); err != nil {
		s.logger.Error("create notification", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create notification channel")
		return
	}

	s.audit(r, "create", "notification_channel", ch.ID, "")
	writeJSON(w, http.StatusCreated, ch)
}

func (s *Server) handleUpdateNotification(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = s.store.GetNotificationChannel(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "notification channel not found")
			return
		}
		s.logger.Error("get notification for update", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get notification channel")
		return
	}

	var ch storage.NotificationChannel
	if err := readJSON(r, &ch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ch.ID = id

	if err := validateNotificationChannel(&ch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.UpdateNotificationChannel(r.Context(), &ch); err != nil {
		s.logger.Error("update notification", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update notification channel")
		return
	}

	s.audit(r, "update", "notification_channel", ch.ID, "")
	writeJSON(w, http.StatusOK, ch)
}

func (s *Server) handleDeleteNotification(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = s.store.GetNotificationChannel(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "notification channel not found")
			return
		}
		s.logger.Error("get notification for delete", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get notification channel")
		return
	}

	if err := s.store.DeleteNotificationChannel(r.Context(), id); err != nil {
		s.logger.Error("delete notification", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete notification channel")
		return
	}

	s.audit(r, "delete", "notification_channel", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ch, err := s.store.GetNotificationChannel(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "notification channel not found")
			return
		}
		s.logger.Error("get notification for test", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get notification channel")
		return
	}

	if s.notifier == nil {
		writeError(w, http.StatusServiceUnavailable, "notification system not available")
		return
	}

	testIncident := &storage.Incident{
		ID:          0,
		MonitorID:   0,
		MonitorName: "Test Monitor",
		Status:      incident.StatusOpen,
		Cause:       "This is a test notification",
	}

	if err := s.notifier.SendTest(ch, testIncident); err != nil {
		writeError(w, http.StatusBadGateway, "test failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}
