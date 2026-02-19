package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/y0f/Asura/internal/notifier"
)

func (s *Server) handleWebIncidents(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	status := r.URL.Query().Get("status")
	result, err := s.store.ListIncidents(r.Context(), 0, status, p)
	if err != nil {
		s.logger.Error("web: list incidents", "error", err)
	}

	pd := s.newPageData(r, "Incidents", "incidents")
	pd.Data = map[string]interface{}{
		"Result": result,
		"Filter": status,
	}
	s.render(w, "incidents.html", pd)
}

func (s *Server) handleWebIncidentDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.redirect(w, r, "/incidents")
		return
	}

	inc, err := s.store.GetIncident(r.Context(), id)
	if err != nil {
		s.redirect(w, r, "/incidents")
		return
	}

	events, _ := s.store.ListIncidentEvents(r.Context(), id)

	pd := s.newPageData(r, "Incident #"+r.PathValue("id"), "incidents")
	pd.Data = map[string]interface{}{
		"Incident": inc,
		"Events":   events,
	}
	s.render(w, "incident_detail.html", pd)
}

func (s *Server) handleWebIncidentAck(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	ctx := r.Context()

	inc, err := s.store.GetIncident(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.redirect(w, r, "/incidents")
			return
		}
		s.logger.Error("web: get incident for ack", "error", err)
		s.redirect(w, r, "/incidents")
		return
	}

	now := time.Now().UTC()
	inc.Status = "acknowledged"
	inc.AcknowledgedAt = &now
	inc.AcknowledgedBy = getAPIKeyName(ctx)

	if err := s.store.UpdateIncident(ctx, inc); err != nil {
		s.logger.Error("web: ack incident", "error", err)
	}

	s.store.InsertIncidentEvent(ctx, newIncidentEvent(inc.ID, "acknowledged", "Acknowledged by "+inc.AcknowledgedBy))

	if s.notifier != nil {
		s.notifier.NotifyWithPayload(&notifier.Payload{
			EventType: "incident.acknowledged",
			Incident:  inc,
		})
	}

	s.setFlash(w, "Incident acknowledged")
	s.redirect(w, r, "/incidents/"+r.PathValue("id"))
}

func (s *Server) handleWebIncidentResolve(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	ctx := r.Context()

	inc, err := s.store.GetIncident(ctx, id)
	if err != nil {
		s.redirect(w, r, "/incidents")
		return
	}

	now := time.Now().UTC()
	inc.Status = "resolved"
	inc.ResolvedAt = &now
	inc.ResolvedBy = getAPIKeyName(ctx)

	if err := s.store.UpdateIncident(ctx, inc); err != nil {
		s.logger.Error("web: resolve incident", "error", err)
	}

	s.store.InsertIncidentEvent(ctx, newIncidentEvent(inc.ID, "resolved", "Manually resolved by "+inc.ResolvedBy))

	if s.notifier != nil {
		s.notifier.NotifyWithPayload(&notifier.Payload{
			EventType: "incident.resolved",
			Incident:  inc,
		})
	}

	s.setFlash(w, "Incident resolved")
	s.redirect(w, r, "/incidents/"+r.PathValue("id"))
}

func (s *Server) handleWebIncidentDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	if err := s.store.DeleteIncident(r.Context(), id); err != nil {
		s.logger.Error("web: delete incident", "error", err)
	}
	s.setFlash(w, "Incident deleted")
	s.redirect(w, r, "/incidents")
}
