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

	"github.com/y0f/Asura/internal/config"
	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	result, err := s.store.ListMonitors(r.Context(), storage.MonitorListFilter{}, p)
	if err != nil {
		s.logger.Error("list monitors", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list monitors")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := s.store.GetMonitor(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "monitor not found")
			return
		}
		s.logger.Error("get monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get monitor")
		return
	}

	channelIDs, _ := s.store.GetMonitorNotificationChannelIDs(r.Context(), m.ID)
	if channelIDs != nil {
		m.NotificationChannelIDs = channelIDs
	}

	if m.Type == "heartbeat" {
		hb, err := s.store.GetHeartbeatByMonitorID(r.Context(), m.ID)
		if err == nil && hb != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"monitor":   m,
				"heartbeat": hb,
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleCreateMonitor(w http.ResponseWriter, r *http.Request) {
	var m storage.Monitor
	if err := readJSON(r, &m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	applyMonitorDefaults(&m, s.cfg.Monitor)

	if err := validateMonitor(&m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.CreateMonitor(r.Context(), &m); err != nil {
		s.logger.Error("create monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create monitor")
		return
	}

	if len(m.NotificationChannelIDs) > 0 {
		if err := s.store.SetMonitorNotificationChannels(r.Context(), m.ID, m.NotificationChannelIDs); err != nil {
			s.logger.Error("set monitor notification channels", "error", err)
		}
	}

	var heartbeat *storage.Heartbeat
	if m.Type == "heartbeat" {
		var err error
		heartbeat, err = s.createHeartbeat(r.Context(), &m)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	s.audit(r, "create", "monitor", m.ID, "")

	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}

	if heartbeat != nil {
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"monitor":   m,
			"heartbeat": heartbeat,
		})
		return
	}
	writeJSON(w, http.StatusCreated, m)
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

func (s *Server) createHeartbeat(ctx context.Context, m *storage.Monitor) (*storage.Heartbeat, error) {
	token, err := generateToken()
	if err != nil {
		s.logger.Error("generate heartbeat token", "error", err)
		return nil, fmt.Errorf("generate heartbeat token: %w", err)
	}
	grace := parseGraceFromSettings(m.Settings)
	hb := &storage.Heartbeat{
		MonitorID: m.ID,
		Token:     token,
		Grace:     grace,
		Status:    "pending",
	}
	if err := s.store.CreateHeartbeat(ctx, hb); err != nil {
		s.logger.Error("create heartbeat", "error", err)
		return nil, fmt.Errorf("create heartbeat: %w", err)
	}
	return hb, nil
}

func parseGraceFromSettings(settings json.RawMessage) int {
	if settings == nil {
		return 0
	}
	var s map[string]interface{}
	if err := readJSONRaw(settings, &s); err != nil {
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

func (s *Server) handleUpdateMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := s.store.GetMonitor(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "monitor not found")
			return
		}
		s.logger.Error("get monitor for update", "error", err)
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

	if err := validateMonitor(&m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.UpdateMonitor(r.Context(), &m); err != nil {
		s.logger.Error("update monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update monitor")
		return
	}

	if err := s.store.SetMonitorNotificationChannels(r.Context(), m.ID, m.NotificationChannelIDs); err != nil {
		s.logger.Error("set monitor notification channels", "error", err)
	}

	s.audit(r, "update", "monitor", m.ID, "")

	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}

	updated, _ := s.store.GetMonitor(r.Context(), id)
	if updated != nil {
		writeJSON(w, http.StatusOK, updated)
	} else {
		writeJSON(w, http.StatusOK, m)
	}
}

func (s *Server) handleDeleteMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = s.store.GetMonitor(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "monitor not found")
			return
		}
		s.logger.Error("get monitor for delete", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get monitor")
		return
	}

	if err := s.store.DeleteMonitor(r.Context(), id); err != nil {
		s.logger.Error("delete monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete monitor")
		return
	}

	s.audit(r, "delete", "monitor", id, "")

	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handlePauseMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetMonitorEnabled(r.Context(), id, false); err != nil {
		s.logger.Error("pause monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to pause monitor")
		return
	}
	s.audit(r, "pause", "monitor", id, "")
	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) handleResumeMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetMonitorEnabled(r.Context(), id, true); err != nil {
		s.logger.Error("resume monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to resume monitor")
		return
	}
	s.audit(r, "resume", "monitor", id, "")
	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (s *Server) handleListChecks(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p := parsePagination(r)
	result, err := s.store.ListCheckResults(r.Context(), id, p)
	if err != nil {
		s.logger.Error("list checks", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list checks")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleListChanges(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p := parsePagination(r)
	result, err := s.store.ListContentChanges(r.Context(), id, p)
	if err != nil {
		s.logger.Error("list changes", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list changes")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func readJSONRaw(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (s *Server) audit(r *http.Request, action, entity string, entityID int64, detail string) {
	entry := &storage.AuditEntry{
		Action:     action,
		Entity:     entity,
		EntityID:   entityID,
		APIKeyName: getAPIKeyName(r.Context()),
		Detail:     detail,
	}
	if err := s.store.InsertAudit(r.Context(), entry); err != nil {
		s.logger.Error("audit log failed", "error", err)
	}
}
