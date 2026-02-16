package api

import (
	"net/http"
	"time"
)

func (s *Server) handleMonitorMetrics(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Default to last 24 hours
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)

	if f := r.URL.Query().Get("from"); f != "" {
		if t, err := time.Parse(time.RFC3339, f); err == nil {
			from = t
		}
	}
	to := now
	if t := r.URL.Query().Get("to"); t != "" {
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			to = parsed
		}
	}

	uptime, err := s.store.GetUptimePercent(r.Context(), id, from, to)
	if err != nil {
		s.logger.Error("get uptime", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get uptime")
		return
	}

	p50, p95, p99, err := s.store.GetResponseTimePercentiles(r.Context(), id, from, to)
	if err != nil {
		s.logger.Error("get percentiles", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get percentiles")
		return
	}

	total, up, down, degraded, err := s.store.GetCheckCounts(r.Context(), id, from, to)
	if err != nil {
		s.logger.Error("get check counts", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get check counts")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"monitor_id":    id,
		"from":          from.Format(time.RFC3339),
		"to":            to.Format(time.RFC3339),
		"uptime_pct":    uptime,
		"response_time": map[string]float64{"p50": p50, "p95": p95, "p99": p99},
		"checks":        map[string]int64{"total": total, "up": up, "down": down, "degraded": degraded},
	})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	up, down, degraded, paused, err := s.store.CountMonitorsByStatus(r.Context())
	if err != nil {
		s.logger.Error("overview", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get overview")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"monitors": map[string]int64{
			"up":       up,
			"down":     down,
			"degraded": degraded,
			"paused":   paused,
			"total":    up + down + degraded + paused,
		},
	})
}
