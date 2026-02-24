package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/y0f/Asura/internal/incident"
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

	monitors, err := s.store.ListMonitors(ctx, storage.MonitorListFilter{}, storage.Pagination{Page: 1, PerPage: 1000})
	if err != nil {
		s.logger.Error("metrics: list monitors", "error", err)
		writeError(w, http.StatusInternalServerError, "metrics error")
		return
	}

	monList, ok := monitors.Data.([]*storage.Monitor)
	if ok && len(monList) > 0 {
		s.writeMonitorMetrics(&sb, ctx, monList)
	}

	s.writeIncidentMetrics(&sb, ctx)
	s.writeRequestMetrics(&sb, ctx)

	if s.pipeline != nil {
		sb.WriteString("\n# HELP asura_scheduler_jobs_dropped_total Total scheduler jobs dropped due to full channel.\n")
		sb.WriteString("# TYPE asura_scheduler_jobs_dropped_total counter\n")
		fmt.Fprintf(&sb, "asura_scheduler_jobs_dropped_total %d\n", s.pipeline.DroppedJobs())

		sb.WriteString("\n# HELP asura_notifications_dropped_total Total notification events dropped due to full channel.\n")
		sb.WriteString("# TYPE asura_notifications_dropped_total counter\n")
		fmt.Fprintf(&sb, "asura_notifications_dropped_total %d\n", s.pipeline.DroppedNotifications())
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Write([]byte(sb.String()))
}

func (s *Server) writeMonitorMetrics(sb *strings.Builder, ctx context.Context, monList []*storage.Monitor) {
	sb.WriteString("\n# HELP asura_monitor_up Whether the monitor is up (1) or down (0).\n")
	sb.WriteString("# TYPE asura_monitor_up gauge\n")
	for _, m := range monList {
		val := 0
		if m.Status == "up" {
			val = 1
		}
		fmt.Fprintf(sb, "asura_monitor_up{id=\"%d\",name=\"%s\",type=\"%s\"} %d\n",
			m.ID, escapeProm(m.Name), m.Type, val)
	}

	rtMap, err := s.store.GetLatestResponseTimes(ctx)
	if err == nil && len(rtMap) > 0 {
		sb.WriteString("\n# HELP asura_monitor_response_time_ms Last response time in milliseconds.\n")
		sb.WriteString("# TYPE asura_monitor_response_time_ms gauge\n")
		for _, m := range monList {
			if rt, ok := rtMap[m.ID]; ok {
				fmt.Fprintf(sb, "asura_monitor_response_time_ms{id=\"%d\",name=\"%s\"} %d\n",
					m.ID, escapeProm(m.Name), rt)
			}
		}
	}

	sb.WriteString("\n# HELP asura_monitor_consecutive_failures Current consecutive failure count.\n")
	sb.WriteString("# TYPE asura_monitor_consecutive_failures gauge\n")
	for _, m := range monList {
		fmt.Fprintf(sb, "asura_monitor_consecutive_failures{id=\"%d\",name=\"%s\"} %d\n",
			m.ID, escapeProm(m.Name), m.ConsecFails)
	}

	sb.WriteString("\n# HELP asura_monitor_consecutive_successes Current consecutive success count.\n")
	sb.WriteString("# TYPE asura_monitor_consecutive_successes gauge\n")
	for _, m := range monList {
		fmt.Fprintf(sb, "asura_monitor_consecutive_successes{id=\"%d\",name=\"%s\"} %d\n",
			m.ID, escapeProm(m.Name), m.ConsecSuccesses)
	}

	typeCounts := make(map[string]int)
	for _, m := range monList {
		typeCounts[m.Type]++
	}
	sb.WriteString("\n# HELP asura_monitors_by_type Total monitors by type.\n")
	sb.WriteString("# TYPE asura_monitors_by_type gauge\n")
	for t, c := range typeCounts {
		fmt.Fprintf(sb, "asura_monitors_by_type{type=\"%s\"} %d\n", t, c)
	}
}

func (s *Server) writeIncidentMetrics(sb *strings.Builder, ctx context.Context) {
	sb.WriteString("\n# HELP asura_incidents_total Total incidents by status.\n")
	sb.WriteString("# TYPE asura_incidents_total gauge\n")
	onePageMin := storage.Pagination{Page: 1, PerPage: 1}
	for _, st := range []string{incident.StatusOpen, incident.StatusAcknowledged, incident.StatusResolved} {
		res, err := s.store.ListIncidents(ctx, 0, st, "", onePageMin)
		if err != nil {
			continue
		}
		fmt.Fprintf(sb, "asura_incidents_total{status=\"%s\"} %d\n", st, res.Total)
	}
}

func (s *Server) writeRequestMetrics(sb *strings.Builder, ctx context.Context) {
	now := time.Now().UTC()
	stats, err := s.store.GetRequestLogStats(ctx, now.Add(-24*time.Hour), now)
	if err != nil || stats == nil {
		return
	}

	sb.WriteString("\n# HELP asura_http_requests_24h Total HTTP requests in last 24 hours.\n")
	sb.WriteString("# TYPE asura_http_requests_24h gauge\n")
	fmt.Fprintf(sb, "asura_http_requests_24h %d\n", stats.TotalRequests)

	sb.WriteString("\n# HELP asura_http_unique_visitors_24h Unique visitors in last 24 hours.\n")
	sb.WriteString("# TYPE asura_http_unique_visitors_24h gauge\n")
	fmt.Fprintf(sb, "asura_http_unique_visitors_24h %d\n", stats.UniqueVisitors)

	sb.WriteString("\n# HELP asura_http_avg_latency_ms Average request latency in last 24 hours.\n")
	sb.WriteString("# TYPE asura_http_avg_latency_ms gauge\n")
	fmt.Fprintf(sb, "asura_http_avg_latency_ms %d\n", stats.AvgLatencyMs)
}

func escapeProm(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}
