package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/asura-monitor/asura/internal/storage"
)

func (s *Server) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	result, err := s.store.ListMonitors(r.Context(), p)
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

	// Apply defaults
	if m.Interval == 0 {
		m.Interval = int(s.cfg.Monitor.DefaultInterval.Seconds())
	}
	if m.Timeout == 0 {
		m.Timeout = int(s.cfg.Monitor.DefaultTimeout.Seconds())
	}
	if m.FailureThreshold == 0 {
		m.FailureThreshold = s.cfg.Monitor.FailureThreshold
	}
	if m.SuccessThreshold == 0 {
		m.SuccessThreshold = s.cfg.Monitor.SuccessThreshold
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	m.Enabled = true

	// Heartbeat monitors don't need a target — set a placeholder
	if m.Type == "heartbeat" {
		if m.Target == "" {
			m.Target = "heartbeat"
		}
	}

	if err := validateMonitor(&m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.CreateMonitor(r.Context(), &m); err != nil {
		s.logger.Error("create monitor", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create monitor")
		return
	}

	// Create heartbeat record for heartbeat-type monitors
	var heartbeat *storage.Heartbeat
	if m.Type == "heartbeat" {
		token, err := generateToken()
		if err != nil {
			s.logger.Error("generate heartbeat token", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to generate heartbeat token")
			return
		}
		grace := 0
		if m.Settings != nil {
			var settings map[string]interface{}
			if err := readJSONRaw(m.Settings, &settings); err == nil {
				if g, ok := settings["grace"]; ok {
					if gf, ok := g.(float64); ok {
						grace = int(gf)
					}
				}
			}
		}
		heartbeat = &storage.Heartbeat{
			MonitorID: m.ID,
			Token:     token,
			Grace:     grace,
			Status:    "pending",
		}
		if err := s.store.CreateHeartbeat(r.Context(), heartbeat); err != nil {
			s.logger.Error("create heartbeat", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create heartbeat")
			return
		}
	}

	s.audit(r, "create", "monitor", m.ID, "")

	// Notify pipeline about new monitor
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
