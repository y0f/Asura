package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleListRequestLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := parsePagination(r)

	f := storage.RequestLogFilter{
		RouteGroup: q.Get("group"),
		Method:     q.Get("method"),
	}

	if sc := q.Get("status_code"); sc != "" {
		if code, err := strconv.Atoi(sc); err == nil {
			f.StatusCode = code
		}
	}
	if mid := q.Get("monitor_id"); mid != "" {
		if id, err := strconv.ParseInt(mid, 10, 64); err == nil {
			f.MonitorID = &id
		}
	}
	if from := q.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			f.From = t
		}
	}
	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			f.To = t
		}
	}

	result, err := s.store.ListRequestLogs(r.Context(), f, p)
	if err != nil {
		s.logger.Error("api: list request logs", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list request logs")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRequestLogStats(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	to := now

	if f := r.URL.Query().Get("from"); f != "" {
		if t, err := time.Parse(time.RFC3339, f); err == nil {
			from = t
		}
	}
	if t := r.URL.Query().Get("to"); t != "" {
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			to = parsed
		}
	}

	stats, err := s.store.GetRequestLogStats(r.Context(), from, to)
	if err != nil {
		s.logger.Error("api: request log stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get request log stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
