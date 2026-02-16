package api

import (
	"log/slog"
	"net/http"

	"github.com/asura-monitor/asura/internal/config"
	"github.com/asura-monitor/asura/internal/monitor"
	"github.com/asura-monitor/asura/internal/notifier"
	"github.com/asura-monitor/asura/internal/storage"
)

// Server holds all dependencies for the HTTP API.
type Server struct {
	cfg      *config.Config
	store    storage.Store
	pipeline *monitor.Pipeline
	notifier *notifier.Dispatcher
	logger   *slog.Logger
	handler  http.Handler
}

// NewServer creates the API server with all routes and middleware.
func NewServer(cfg *config.Config, store storage.Store, pipeline *monitor.Pipeline, dispatcher *notifier.Dispatcher, logger *slog.Logger) *Server {
	s := &Server{
		cfg:      cfg,
		store:    store,
		pipeline: pipeline,
		notifier: dispatcher,
		logger:   logger,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// Build middleware chain (applied in reverse order)
	var handler http.Handler = mux
	handler = bodyLimit(cfg.Server.MaxBodySize)(handler)
	rl := newRateLimiter(cfg.Server.RateLimitPerSec, cfg.Server.RateLimitBurst)
	handler = rl.middleware()(handler)
	handler = cors(cfg.Server.CORSOrigins)(handler)
	handler = secureHeaders()(handler)
	handler = logging(logger)(handler)
	handler = requestID()(handler)
	handler = recovery(logger)(handler)

	s.handler = handler
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	readAuth := auth(s.cfg, "readonly")
	adminAuth := auth(s.cfg, "admin")

	// No auth required
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("POST /api/v1/heartbeat/{token}", s.handleHeartbeatPing)
	mux.HandleFunc("GET /api/v1/heartbeat/{token}", s.handleHeartbeatPing)
	mux.HandleFunc("GET /api/v1/badge/{id}/status", s.handleBadgeStatus)
	mux.HandleFunc("GET /api/v1/badge/{id}/uptime", s.handleBadgeUptime)
	mux.HandleFunc("GET /api/v1/badge/{id}/response", s.handleBadgeResponseTime)

	// Read endpoints
	mux.Handle("GET /api/v1/monitors", readAuth(http.HandlerFunc(s.handleListMonitors)))
	mux.Handle("GET /api/v1/monitors/{id}", readAuth(http.HandlerFunc(s.handleGetMonitor)))
	mux.Handle("GET /api/v1/monitors/{id}/checks", readAuth(http.HandlerFunc(s.handleListChecks)))
	mux.Handle("GET /api/v1/monitors/{id}/metrics", readAuth(http.HandlerFunc(s.handleMonitorMetrics)))
	mux.Handle("GET /api/v1/monitors/{id}/changes", readAuth(http.HandlerFunc(s.handleListChanges)))

	mux.Handle("GET /api/v1/incidents", readAuth(http.HandlerFunc(s.handleListIncidents)))
	mux.Handle("GET /api/v1/incidents/{id}", readAuth(http.HandlerFunc(s.handleGetIncident)))

	mux.Handle("GET /api/v1/notifications", readAuth(http.HandlerFunc(s.handleListNotifications)))

	mux.Handle("GET /api/v1/maintenance", readAuth(http.HandlerFunc(s.handleListMaintenance)))

	mux.Handle("GET /api/v1/overview", readAuth(http.HandlerFunc(s.handleOverview)))
	mux.Handle("GET /api/v1/tags", readAuth(http.HandlerFunc(s.handleListTags)))

	// Admin endpoints
	mux.Handle("POST /api/v1/monitors", adminAuth(http.HandlerFunc(s.handleCreateMonitor)))
	mux.Handle("PUT /api/v1/monitors/{id}", adminAuth(http.HandlerFunc(s.handleUpdateMonitor)))
	mux.Handle("DELETE /api/v1/monitors/{id}", adminAuth(http.HandlerFunc(s.handleDeleteMonitor)))
	mux.Handle("POST /api/v1/monitors/{id}/pause", adminAuth(http.HandlerFunc(s.handlePauseMonitor)))
	mux.Handle("POST /api/v1/monitors/{id}/resume", adminAuth(http.HandlerFunc(s.handleResumeMonitor)))

	mux.Handle("POST /api/v1/incidents/{id}/ack", adminAuth(http.HandlerFunc(s.handleAckIncident)))
	mux.Handle("POST /api/v1/incidents/{id}/resolve", adminAuth(http.HandlerFunc(s.handleResolveIncident)))

	mux.Handle("POST /api/v1/notifications", adminAuth(http.HandlerFunc(s.handleCreateNotification)))
	mux.Handle("PUT /api/v1/notifications/{id}", adminAuth(http.HandlerFunc(s.handleUpdateNotification)))
	mux.Handle("DELETE /api/v1/notifications/{id}", adminAuth(http.HandlerFunc(s.handleDeleteNotification)))
	mux.Handle("POST /api/v1/notifications/{id}/test", adminAuth(http.HandlerFunc(s.handleTestNotification)))

	mux.Handle("POST /api/v1/maintenance", adminAuth(http.HandlerFunc(s.handleCreateMaintenance)))
	mux.Handle("PUT /api/v1/maintenance/{id}", adminAuth(http.HandlerFunc(s.handleUpdateMaintenance)))
	mux.Handle("DELETE /api/v1/maintenance/{id}", adminAuth(http.HandlerFunc(s.handleDeleteMaintenance)))
}
