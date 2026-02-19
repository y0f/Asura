package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/y0f/Asura/internal/storage"
)

const (
	requestLogChanSize      = 4096
	requestLogBatchSize     = 100
	requestLogFlushInterval = 5 * time.Second
)

var monitorIDRe = regexp.MustCompile(`(?:^/monitors/|^/api/v1/monitors/|^/api/v1/badge/)(\d+)`)

// RequestLogWriter batches request log entries and flushes them to the store.
type RequestLogWriter struct {
	ch     chan *storage.RequestLog
	store  storage.Store
	logger *slog.Logger
}

func NewRequestLogWriter(store storage.Store, logger *slog.Logger) *RequestLogWriter {
	return &RequestLogWriter{
		ch:     make(chan *storage.RequestLog, requestLogChanSize),
		store:  store,
		logger: logger,
	}
}

func (w *RequestLogWriter) Send(log *storage.RequestLog) {
	select {
	case w.ch <- log:
	default:
		// Drop on full — non-blocking to never slow down requests
	}
}

func (w *RequestLogWriter) Run(ctx context.Context) {
	ticker := time.NewTicker(requestLogFlushInterval)
	defer ticker.Stop()

	var batch []*storage.RequestLog

	flush := func(fctx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := w.store.InsertRequestLogBatch(fctx, batch); err != nil {
			w.logger.Error("request log batch insert failed", "count", len(batch), "error", err)
		}
		batch = nil
	}

	for {
		select {
		case <-ctx.Done():
			flush(context.Background())
			return
		case log := <-w.ch:
			batch = append(batch, log)
			if len(batch) >= requestLogBatchSize {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}

func requestLogMiddleware(writer *RequestLogWriter, basePath string, trustedNets []net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Skip paths that would create noise
			if shouldSkipLog(path, basePath) {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(sw, r)
			latency := time.Since(start).Milliseconds()

			// Strip basePath for classification
			trimmed := path
			if basePath != "" {
				trimmed = strings.TrimPrefix(path, basePath)
				if trimmed == "" {
					trimmed = "/"
				}
			}

			ip := extractIP(r, trustedNets)

			log := &storage.RequestLog{
				Method:     r.Method,
				Path:       path,
				StatusCode: sw.status,
				LatencyMs:  latency,
				ClientIP:   ip,
				UserAgent:  truncate(r.UserAgent(), 512),
				Referer:    truncate(r.Referer(), 512),
				RouteGroup: classifyRoute(trimmed),
				CreatedAt:  start.UTC(),
			}

			if mid := extractMonitorID(trimmed); mid > 0 {
				log.MonitorID = &mid
			}

			writer.Send(log)
		})
	}
}

func shouldSkipLog(path, basePath string) bool {
	trimmed := path
	if basePath != "" {
		trimmed = strings.TrimPrefix(path, basePath)
	}
	if strings.HasPrefix(trimmed, "/static/") {
		return true
	}
	if trimmed == "/api/v1/health" {
		return true
	}
	return false
}

func classifyRoute(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/v1/badge/"):
		return "badge"
	case strings.HasPrefix(path, "/api/v1/"):
		return "api"
	case path == "/login" || path == "/logout":
		return "auth"
	case strings.HasPrefix(path, "/metrics"):
		return "metrics"
	case path == "/status" || strings.HasPrefix(path, "/status/"):
		return "status"
	default:
		return "web"
	}
}

func extractMonitorID(path string) int64 {
	matches := monitorIDRe.FindStringSubmatch(path)
	if len(matches) < 2 {
		return 0
	}
	id, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
