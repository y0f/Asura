package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/y0f/asura/internal/config"
	"github.com/y0f/asura/internal/httputil"
	"github.com/y0f/asura/internal/storage"
	"github.com/y0f/asura/internal/validate"
)

func (h *Handler) ListMonitors(w http.ResponseWriter, r *http.Request) {
	p := httputil.ParsePagination(r)
	result, err := h.store.ListMonitors(r.Context(), storage.MonitorListFilter{}, p)
	if err != nil {
		h.logger.Error("list monitors", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list monitors")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) GetMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := h.store.GetMonitor(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "monitor not found")
			return
		}
		h.logger.Error("get monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get monitor")
		return
	}

	channelIDs, _ := h.store.GetMonitorNotificationChannelIDs(r.Context(), m.ID)
	if channelIDs != nil {
		m.NotificationChannelIDs = channelIDs
	}

	if m.Type == "heartbeat" {
		hb, err := h.store.GetHeartbeatByMonitorID(r.Context(), m.ID)
		if err == nil && hb != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"monitor":   m,
				"heartbeat": hb,
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) CreateMonitor(w http.ResponseWriter, r *http.Request) {
	var m storage.Monitor
	if err := readJSON(r, &m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	applyMonitorDefaults(&m, h.cfg.Monitor)

	if err := validate.ValidateMonitor(&m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.CreateMonitor(r.Context(), &m); err != nil {
		h.logger.Error("create monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create monitor")
		return
	}

	if len(m.NotificationChannelIDs) > 0 {
		if err := h.store.SetMonitorNotificationChannels(r.Context(), m.ID, m.NotificationChannelIDs); err != nil {
			h.logger.Error("set monitor notification channels", "error", err)
		}
	}

	var heartbeat *storage.Heartbeat
	if m.Type == "heartbeat" {
		var err error
		heartbeat, err = h.createHeartbeat(r.Context(), &m)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	h.audit(r, "create", "monitor", m.ID, "")

	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}

	if heartbeat != nil {
		writeJSON(w, http.StatusCreated, map[string]any{
			"monitor":   m,
			"heartbeat": heartbeat,
		})
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handler) UpdateMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := h.store.GetMonitor(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "monitor not found")
			return
		}
		h.logger.Error("get monitor for update", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get monitor")
		return
	}

	var m storage.Monitor
	if err := readJSON(r, &m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	m.ID = existing.ID
	m.CreatedAt = existing.CreatedAt

	if m.Tags == nil {
		m.Tags = []string{}
	}

	if err := validate.ValidateMonitor(&m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.UpdateMonitor(r.Context(), &m); err != nil {
		h.logger.Error("update monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update monitor")
		return
	}

	if err := h.store.SetMonitorNotificationChannels(r.Context(), m.ID, m.NotificationChannelIDs); err != nil {
		h.logger.Error("set monitor notification channels", "error", err)
	}

	h.audit(r, "update", "monitor", m.ID, "")

	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}

	updated, _ := h.store.GetMonitor(r.Context(), id)
	if updated != nil {
		writeJSON(w, http.StatusOK, updated)
	} else {
		writeJSON(w, http.StatusOK, m)
	}
}

func (h *Handler) DeleteMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = h.store.GetMonitor(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "monitor not found")
			return
		}
		h.logger.Error("get monitor for delete", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get monitor")
		return
	}

	if err := h.store.DeleteMonitor(r.Context(), id); err != nil {
		h.logger.Error("delete monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete monitor")
		return
	}

	h.audit(r, "delete", "monitor", id, "")

	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) PauseMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.SetMonitorEnabled(r.Context(), id, false); err != nil {
		h.logger.Error("pause monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to pause monitor")
		return
	}
	h.audit(r, "pause", "monitor", id, "")
	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (h *Handler) ResumeMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.SetMonitorEnabled(r.Context(), id, true); err != nil {
		h.logger.Error("resume monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to resume monitor")
		return
	}
	h.audit(r, "resume", "monitor", id, "")
	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (h *Handler) ListChecks(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p := httputil.ParsePagination(r)
	result, err := h.store.ListCheckResults(r.Context(), id, p)
	if err != nil {
		h.logger.Error("list checks", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list checks")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) ListChanges(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p := httputil.ParsePagination(r)
	result, err := h.store.ListContentChanges(r.Context(), id, p)
	if err != nil {
		h.logger.Error("list changes", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list changes")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func applyMonitorDefaults(m *storage.Monitor, cfg config.MonitorConfig) {
	if m.Interval == 0 {
		m.Interval = int(cfg.DefaultInterval.Seconds())
	}
	if m.Timeout == 0 {
		m.Timeout = int(cfg.DefaultTimeout.Seconds())
	}
	if m.FailureThreshold == 0 {
		m.FailureThreshold = cfg.FailureThreshold
	}
	if m.SuccessThreshold == 0 {
		m.SuccessThreshold = cfg.SuccessThreshold
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	if m.Type == "heartbeat" && m.Target == "" {
		m.Target = "heartbeat"
	}
	m.Enabled = true
}

func (h *Handler) createHeartbeat(ctx context.Context, m *storage.Monitor) (*storage.Heartbeat, error) {
	token, err := generateToken()
	if err != nil {
		h.logger.Error("generate heartbeat token", "error", err)
		return nil, fmt.Errorf("generate heartbeat token: %w", err)
	}
	grace := parseGraceFromSettings(m.Settings)
	hb := &storage.Heartbeat{
		MonitorID: m.ID,
		Token:     token,
		Grace:     grace,
		Status:    "pending",
	}
	if err := h.store.CreateHeartbeat(ctx, hb); err != nil {
		h.logger.Error("create heartbeat", "error", err)
		return nil, fmt.Errorf("create heartbeat: %w", err)
	}
	return hb, nil
}

func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func parseGraceFromSettings(settings json.RawMessage) int {
	if settings == nil {
		return 0
	}
	var s map[string]any
	if err := json.Unmarshal(settings, &s); err != nil {
		return 0
	}
	g, ok := s["grace"]
	if !ok {
		return 0
	}
	gf, ok := g.(float64)
	if !ok {
		return 0
	}
	return int(gf)
}
