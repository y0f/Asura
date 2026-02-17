package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleWebMonitors(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	result, err := s.store.ListMonitors(r.Context(), p)
	if err != nil {
		s.logger.Error("web: list monitors", "error", err)
	}

	pd := s.newPageData(r, "Monitors", "monitors")
	pd.Data = result
	s.render(w, "monitors.html", pd)
}

func (s *Server) handleWebMonitorDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Redirect(w, r, "/monitors", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	mon, err := s.store.GetMonitor(ctx, id)
	if err != nil {
		http.Redirect(w, r, "/monitors", http.StatusSeeOther)
		return
	}

	now := time.Now().UTC()
	checks, _ := s.store.ListCheckResults(ctx, id, storage.Pagination{Page: 1, PerPage: 50})
	if checks == nil {
		checks = &storage.PaginatedResult{}
	}
	changes, _ := s.store.ListContentChanges(ctx, id, storage.Pagination{Page: 1, PerPage: 10})
	if changes == nil {
		changes = &storage.PaginatedResult{}
	}

	uptime24h, _ := s.store.GetUptimePercent(ctx, id, now.Add(-24*time.Hour), now)
	uptime7d, _ := s.store.GetUptimePercent(ctx, id, now.Add(-7*24*time.Hour), now)
	uptime30d, _ := s.store.GetUptimePercent(ctx, id, now.Add(-30*24*time.Hour), now)
	p50, p95, p99, _ := s.store.GetResponseTimePercentiles(ctx, id, now.Add(-24*time.Hour), now)
	totalChecks, upChecks, downChecks, _, _ := s.store.GetCheckCounts(ctx, id, now.Add(-24*time.Hour), now)
	latestCheck, _ := s.store.GetLatestCheckResult(ctx, id)
	openIncident, _ := s.store.GetOpenIncident(ctx, id)

	pd := s.newPageData(r, mon.Name, "monitors")
	pd.Data = map[string]interface{}{
		"Monitor":      mon,
		"Checks":       checks,
		"Changes":      changes,
		"Uptime24h":    uptime24h,
		"Uptime7d":     uptime7d,
		"Uptime30d":    uptime30d,
		"P50":          p50,
		"P95":          p95,
		"P99":          p99,
		"TotalChecks":  totalChecks,
		"UpChecks":     upChecks,
		"DownChecks":   downChecks,
		"LatestCheck":  latestCheck,
		"OpenIncident": openIncident,
	}
	s.render(w, "monitor_detail.html", pd)
}

func (s *Server) handleWebMonitorForm(w http.ResponseWriter, r *http.Request) {
	pd := s.newPageData(r, "New Monitor", "monitors")

	idStr := r.PathValue("id")
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Redirect(w, r, "/monitors", http.StatusSeeOther)
			return
		}
		mon, err := s.store.GetMonitor(r.Context(), id)
		if err != nil {
			http.Redirect(w, r, "/monitors", http.StatusSeeOther)
			return
		}
		pd.Title = "Edit " + mon.Name
		pd.Data = mon
	}

	s.render(w, "monitor_form.html", pd)
}

func (s *Server) handleWebMonitorCreate(w http.ResponseWriter, r *http.Request) {
	mon := s.parseMonitorForm(r)

	s.applyMonitorDefaults(mon)

	if err := validateMonitor(mon); err != nil {
		pd := s.newPageData(r, "New Monitor", "monitors")
		pd.Error = err.Error()
		pd.Data = mon
		s.render(w, "monitor_form.html", pd)
		return
	}

	if err := s.store.CreateMonitor(r.Context(), mon); err != nil {
		s.logger.Error("web: create monitor", "error", err)
		pd := s.newPageData(r, "New Monitor", "monitors")
		pd.Error = "Failed to create monitor"
		pd.Data = mon
		s.render(w, "monitor_form.html", pd)
		return
	}

	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}

	s.setFlash(w, "Monitor created")
	http.Redirect(w, r, "/monitors", http.StatusSeeOther)
}

func (s *Server) handleWebMonitorUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Redirect(w, r, "/monitors", http.StatusSeeOther)
		return
	}

	mon := s.parseMonitorForm(r)
	mon.ID = id

	if err := validateMonitor(mon); err != nil {
		pd := s.newPageData(r, "Edit Monitor", "monitors")
		pd.Error = err.Error()
		pd.Data = mon
		s.render(w, "monitor_form.html", pd)
		return
	}

	if err := s.store.UpdateMonitor(r.Context(), mon); err != nil {
		s.logger.Error("web: update monitor", "error", err)
		pd := s.newPageData(r, "Edit Monitor", "monitors")
		pd.Error = "Failed to update monitor"
		pd.Data = mon
		s.render(w, "monitor_form.html", pd)
		return
	}

	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}

	s.setFlash(w, "Monitor updated")
	http.Redirect(w, r, "/monitors/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *Server) handleWebMonitorDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Redirect(w, r, "/monitors", http.StatusSeeOther)
		return
	}

	if err := s.store.DeleteMonitor(r.Context(), id); err != nil {
		s.logger.Error("web: delete monitor", "error", err)
	}

	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}

	s.setFlash(w, "Monitor deleted")
	http.Redirect(w, r, "/monitors", http.StatusSeeOther)
}

func (s *Server) handleWebMonitorPause(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	s.store.SetMonitorEnabled(r.Context(), id, false)
	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}
	s.setFlash(w, "Monitor paused")
	http.Redirect(w, r, "/monitors/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *Server) handleWebMonitorResume(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	s.store.SetMonitorEnabled(r.Context(), id, true)
	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}
	s.setFlash(w, "Monitor resumed")
	http.Redirect(w, r, "/monitors/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *Server) applyMonitorDefaults(m *storage.Monitor) {
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
	if m.Type == "heartbeat" && m.Target == "" {
		m.Target = "heartbeat"
	}
}

func (s *Server) parseMonitorForm(r *http.Request) *storage.Monitor {
	r.ParseForm()

	interval, _ := strconv.Atoi(r.FormValue("interval"))
	timeout, _ := strconv.Atoi(r.FormValue("timeout"))
	failThreshold, _ := strconv.Atoi(r.FormValue("failure_threshold"))
	successThreshold, _ := strconv.Atoi(r.FormValue("success_threshold"))

	mon := &storage.Monitor{
		Name:             r.FormValue("name"),
		Type:             r.FormValue("type"),
		Target:           r.FormValue("target"),
		Interval:         interval,
		Timeout:          timeout,
		Enabled:          true,
		TrackChanges:     r.FormValue("track_changes") == "on",
		Public:           r.FormValue("public") == "on",
		FailureThreshold: failThreshold,
		SuccessThreshold: successThreshold,
	}

	if tags := strings.TrimSpace(r.FormValue("tags")); tags != "" {
		for _, t := range strings.Split(tags, ",") {
			if trimmed := strings.TrimSpace(t); trimmed != "" {
				mon.Tags = append(mon.Tags, trimmed)
			}
		}
	}

	if settings := r.FormValue("settings"); settings != "" {
		mon.Settings = json.RawMessage(settings)
	}

	if assertions := r.FormValue("assertions"); assertions != "" {
		mon.Assertions = json.RawMessage(assertions)
	}

	return mon
}
