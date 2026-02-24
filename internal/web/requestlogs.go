package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/y0f/asura/internal/httputil"
	"github.com/y0f/asura/internal/storage"
)

func (h *Handler) RequestLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := httputil.ParsePagination(r)

	f := storage.RequestLogFilter{
		RouteGroup: q.Get("group"),
		Method:     q.Get("method"),
		ClientIP:   q.Get("ip"),
	}

	if sc := q.Get("status_code"); sc != "" {
		if code, err := strconv.Atoi(sc); err == nil {
			f.StatusCode = code
		}
	}

	now := time.Now().UTC()
	switch q.Get("range") {
	case "1h":
		f.From = now.Add(-1 * time.Hour)
	case "12h":
		f.From = now.Add(-12 * time.Hour)
	case "7d":
		f.From = now.AddDate(0, 0, -7)
	default:
		f.From = now.Add(-24 * time.Hour)
	}
	f.To = now

	result, err := h.store.ListRequestLogs(r.Context(), f, p)
	if err != nil {
		h.logger.Error("web: list request logs", "error", err)
	}

	stats, err := h.store.GetRequestLogStats(r.Context(), f.From, f.To)
	if err != nil {
		h.logger.Error("web: get request log stats", "error", err)
	}

	topIPs, err := h.store.ListTopClientIPs(r.Context(), f.From, f.To, 50)
	if err != nil {
		h.logger.Error("web: list top client IPs", "error", err)
	}
	if f.ClientIP != "" {
		found := false
		for _, ip := range topIPs {
			if ip == f.ClientIP {
				found = true
				break
			}
		}
		if !found {
			topIPs = append([]string{f.ClientIP}, topIPs...)
		}
	}

	timeRange := q.Get("range")
	if timeRange == "" {
		timeRange = "24h"
	}

	pd := h.newPageData(r, "Request Logs", "logs")
	pd.Data = map[string]any{
		"Result":     result,
		"Stats":      stats,
		"Filter":     f.RouteGroup,
		"Method":     f.Method,
		"StatusCode": f.StatusCode,
		"ClientIP":   f.ClientIP,
		"TimeRange":  timeRange,
		"TopIPs":     topIPs,
	}
	h.render(w, "requestlogs/list.html", pd)
}
