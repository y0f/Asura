package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var sb strings.Builder

	up, down, degraded, paused, err := s.store.CountMonitorsByStatus(ctx)
	if err != nil {
		s.logger.Error("metrics: count monitors", "error", err)
		writeError(w, http.StatusInternalServerError, "metrics error")
		return
	}

	sb.WriteString("# HELP asura_monitors_total Total number of monitors by status.\n")
	sb.WriteString("# TYPE asura_monitors_total gauge\n")
	fmt.Fprintf(&sb, "asura_monitors_total{status=\"up\"} %d\n", up)
	fmt.Fprintf(&sb, "asura_monitors_total{status=\"down\"} %d\n", down)
	fmt.Fprintf(&sb, "asura_monitors_total{status=\"degraded\"} %d\n", degraded)
	fmt.Fprintf(&sb, "asura_monitors_total{status=\"paused\"} %d\n", paused)

	monitors, err := s.store.ListMonitors(ctx, storage.Pagination{Page: 1, PerPage: 1000})
	if err != nil {
		s.logger.Error("metrics: list monitors", "error", err)
		writeError(w, http.StatusInternalServerError, "metrics error")
		return
	}

	monList, ok := monitors.Data.([]*storage.Monitor)
	if !ok || len(monList) == 0 {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Write([]byte(sb.String()))
		return
	}

	sb.WriteString("\n# HELP asura_monitor_up Whether the monitor is up (1) or down (0).\n")
	sb.WriteString("# TYPE asura_monitor_up gauge\n")
	for _, m := range monList {
		val := 0
		if m.Status == "up" {
			val = 1
		}
		fmt.Fprintf(&sb, "asura_monitor_up{id=\"%d\",name=\"%s\",type=\"%s\"} %d\n",
			m.ID, escapeProm(m.Name), m.Type, val)
	}

	// Single query instead of per-monitor lookups
	rtMap, err := s.store.GetLatestResponseTimes(ctx)
	if err == nil && len(rtMap) > 0 {
		sb.WriteString("\n# HELP asura_monitor_response_time_ms Last response time in milliseconds.\n")
		sb.WriteString("# TYPE asura_monitor_response_time_ms gauge\n")
		for _, m := range monList {
			if rt, ok := rtMap[m.ID]; ok {
				fmt.Fprintf(&sb, "asura_monitor_response_time_ms{id=\"%d\",name=\"%s\"} %d\n",
					m.ID, escapeProm(m.Name), rt)
			}
		}
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Write([]byte(sb.String()))
}

func escapeProm(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
