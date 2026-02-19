package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleWebDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	const perPage = 10

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	typeFilter := r.URL.Query().Get("type")
	if !validMonitorTypes[typeFilter] {
		typeFilter = ""
	}

	allResult, err := s.store.ListMonitors(ctx, storage.MonitorListFilter{}, storage.Pagination{Page: 1, PerPage: 1000})
	if err != nil {
		s.logger.Error("web: list monitors", "error", err)
	}

	var allMonitors []*storage.Monitor
	if allResult != nil {
		if ml, ok := allResult.Data.([]*storage.Monitor); ok {
			allMonitors = ml
		}
	}

	up, down, degraded, paused := countMonitorStats(allMonitors)
	filtered := filterMonitorsByType(allMonitors, typeFilter)

	total := len(filtered)
	totalPages := (total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * perPage
	end := start + perPage
	if end > total {
		end = total
	}
	displayMonitors := filtered[start:end]

	incidents, err := s.store.ListIncidents(ctx, 0, "open", "", storage.Pagination{Page: 1, PerPage: 10})
	if err != nil {
		s.logger.Error("web: list incidents", "error", err)
	}

	var incidentList []*storage.Incident
	if incidents != nil {
		if il, ok := incidents.Data.([]*storage.Incident); ok {
			incidentList = il
		}
	}

	responseTimes, _ := s.store.GetLatestResponseTimes(ctx)
	if responseTimes == nil {
		responseTimes = make(map[int64]int64)
	}

	now := time.Now().UTC()
	reqStats, err := s.store.GetRequestLogStats(ctx, now.Add(-24*time.Hour), now)
	if err != nil {
		s.logger.Error("web: request log stats", "error", err)
	}
	var requests24h, visitors24h int64
	if reqStats != nil {
		requests24h = reqStats.TotalRequests
		visitors24h = reqStats.UniqueVisitors
	}

	pd := s.newPageData(r, "Dashboard", "dashboard")
	pd.Data = map[string]interface{}{
		"Monitors":      displayMonitors,
		"Incidents":     incidentList,
		"Total":         len(allMonitors),
		"Up":            up,
		"Down":          down,
		"Degraded":      degraded,
		"Paused":        paused,
		"OpenIncidents": len(incidentList),
		"ResponseTimes": responseTimes,
		"Requests24h":   requests24h,
		"Visitors24h":   visitors24h,
		"Page":          page,
		"TotalPages":    totalPages,
		"Filter":        typeFilter,
		"FilteredTotal": total,
	}
	s.render(w, "dashboard.html", pd)
}

func countMonitorStats(monitors []*storage.Monitor) (up, down, degraded, paused int) {
	for _, m := range monitors {
		if !m.Enabled {
			paused++
			continue
		}
		switch m.Status {
		case "down":
			down++
		case "degraded":
			degraded++
		default:
			up++
		}
	}
	return
}

func filterMonitorsByType(monitors []*storage.Monitor, t string) []*storage.Monitor {
	if t == "" {
		return monitors
	}
	out := make([]*storage.Monitor, 0, len(monitors))
	for _, m := range monitors {
		if m.Type == t {
			out = append(out, m)
		}
	}
	return out
}
