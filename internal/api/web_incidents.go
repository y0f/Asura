package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/asura-monitor/asura/internal/notifier"
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
		http.Redirect(w, r, "/incidents", http.StatusSeeOther)
		return
	}

	inc, err := s.store.GetIncident(r.Context(), id)
	if err != nil {
		http.Redirect(w, r, "/incidents", http.StatusSeeOther)
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
			http.Redirect(w, r, "/incidents", http.StatusSeeOther)
			return
		}
		s.logger.Error("web: get incident for ack", "error", err)
		http.Redirect(w, r, "/incidents", http.StatusSeeOther)
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
	http.Redirect(w, r, "/incidents/"+r.PathValue("id"), http.StatusSeeOther)
}

func (s *Server) handleWebIncidentResolve(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	ctx := r.Context()

	inc, err := s.store.GetIncident(ctx, id)
	if err != nil {
		http.Redirect(w, r, "/incidents", http.StatusSeeOther)
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
	http.Redirect(w, r, "/incidents/"+r.PathValue("id"), http.StatusSeeOther)
}
