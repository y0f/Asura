package api

import (
	"net/http"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleWebDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	monitors, err := s.store.ListMonitors(ctx, storage.Pagination{Page: 1, PerPage: 100})
	if err != nil {
		s.logger.Error("web: list monitors", "error", err)
	}

	incidents, err := s.store.ListIncidents(ctx, 0, "open", storage.Pagination{Page: 1, PerPage: 10})
	if err != nil {
		s.logger.Error("web: list incidents", "error", err)
	}

	var monitorList []*storage.Monitor
	if monitors != nil {
		if ml, ok := monitors.Data.([]*storage.Monitor); ok {
			monitorList = ml
		}
	}

	var incidentList []*storage.Incident
	if incidents != nil {
		if il, ok := incidents.Data.([]*storage.Incident); ok {
			incidentList = il
		}
	}

	up, down, degraded, paused := 0, 0, 0, 0
	for _, m := range monitorList {
		if !m.Enabled {
			paused++
			continue
		}
		switch m.Status {
		case "up":
			up++
		case "down":
			down++
		case "degraded":
			degraded++
		default:
			up++
		}
	}

	responseTimes, _ := s.store.GetLatestResponseTimes(ctx)
	if responseTimes == nil {
		responseTimes = make(map[int64]int64)
	}

	pd := s.newPageData(r, "Dashboard", "dashboard")
	pd.Data = map[string]interface{}{
		"Monitors":      monitorList,
		"Incidents":     incidentList,
		"Total":         len(monitorList),
		"Up":            up,
		"Down":          down,
		"Degraded":      degraded,
		"Paused":        paused,
		"OpenIncidents": len(incidentList),
		"ResponseTimes": responseTimes,
	}
	s.render(w, "dashboard.html", pd)
}
