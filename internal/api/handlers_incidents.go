package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/asura-monitor/asura/internal/storage"
)

func (s *Server) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	monitorID, _ := strconv.ParseInt(r.URL.Query().Get("monitor_id"), 10, 64)
	status := r.URL.Query().Get("status")

	result, err := s.store.ListIncidents(r.Context(), monitorID, status, p)
	if err != nil {
		s.logger.Error("list incidents", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list incidents")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetIncident(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	inc, err := s.store.GetIncident(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "incident not found")
			return
		}
		s.logger.Error("get incident", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get incident")
		return
	}

	events, err := s.store.ListIncidentEvents(r.Context(), id)
	if err != nil {
		s.logger.Error("list incident events", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list incident events")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"incident": inc,
		"timeline": events,
	})
}

func (s *Server) handleAckIncident(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	inc, err := s.store.GetIncident(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "incident not found")
			return
		}
		s.logger.Error("get incident for ack", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get incident")
		return
	}

	if inc.Status != "open" {
		writeError(w, http.StatusConflict, "incident is not open")
		return
	}

	now := time.Now().UTC()
	inc.Status = "acknowledged"
	inc.AcknowledgedAt = &now
	inc.AcknowledgedBy = getAPIKeyName(r.Context())

	if err := s.store.UpdateIncident(r.Context(), inc); err != nil {
		s.logger.Error("ack incident", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to acknowledge incident")
		return
	}

	s.store.InsertIncidentEvent(r.Context(), newIncidentEvent(inc.ID, "acknowledged", "Acknowledged by "+inc.AcknowledgedBy))

	s.audit(r, "acknowledge", "incident", id, "")

	if s.notifier != nil {
		s.notifier.Notify("incident.acknowledged", inc)
	}

	writeJSON(w, http.StatusOK, inc)
}

func (s *Server) handleResolveIncident(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	inc, err := s.store.GetIncident(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "incident not found")
			return
		}
		s.logger.Error("get incident for resolve", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get incident")
		return
	}

	if inc.Status == "resolved" {
		writeError(w, http.StatusConflict, "incident is already resolved")
		return
	}

	now := time.Now().UTC()
	inc.Status = "resolved"
	inc.ResolvedAt = &now
	inc.ResolvedBy = getAPIKeyName(r.Context())

	if err := s.store.UpdateIncident(r.Context(), inc); err != nil {
		s.logger.Error("resolve incident", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to resolve incident")
		return
	}

	s.store.InsertIncidentEvent(r.Context(), newIncidentEvent(inc.ID, "resolved", "Manually resolved by "+inc.ResolvedBy))

	s.audit(r, "resolve", "incident", id, "")

	if s.notifier != nil {
		s.notifier.Notify("incident.resolved", inc)
	}

	writeJSON(w, http.StatusOK, inc)
}

func newIncidentEvent(incidentID int64, eventType, message string) *storage.IncidentEvent {
	return &storage.IncidentEvent{
		IncidentID: incidentID,
		Type:       eventType,
		Message:    message,
	}
}
