package api

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/asura-monitor/asura/internal/config"
	"github.com/asura-monitor/asura/internal/monitor"
	"github.com/asura-monitor/asura/internal/notifier"
	"github.com/asura-monitor/asura/internal/storage"
	"github.com/asura-monitor/asura/web"
)

type Server struct {
	cfg       *config.Config
	store     storage.Store
	pipeline  *monitor.Pipeline
	notifier  *notifier.Dispatcher
	logger    *slog.Logger
	handler   http.Handler
	version   string
	templates map[string]*template.Template
}

func NewServer(cfg *config.Config, store storage.Store, pipeline *monitor.Pipeline, dispatcher *notifier.Dispatcher, logger *slog.Logger, version string) *Server {
	s := &Server{
		cfg:      cfg,
		store:    store,
		pipeline: pipeline,
		notifier: dispatcher,
		logger:   logger,
		version:  version,
	}

	s.loadTemplates()

	mux := http.NewServeMux()
	s.registerRoutes(mux)

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

	// Static files
	staticFS, _ := fs.Sub(web.FS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Web UI
	mux.HandleFunc("GET /login", s.handleWebLogin)
	mux.HandleFunc("POST /login", s.handleWebLoginPost)
	mux.HandleFunc("POST /logout", s.handleWebLogout)

	webAuth := s.webAuth
	webAdmin := func(h http.HandlerFunc) http.Handler {
		return webAuth(s.webAdminOnly(http.HandlerFunc(h)))
	}

	mux.Handle("GET /{$}", webAuth(http.HandlerFunc(s.handleWebDashboard)))
	mux.Handle("GET /monitors", webAuth(http.HandlerFunc(s.handleWebMonitors)))
	mux.Handle("GET /monitors/new", webAuth(http.HandlerFunc(s.handleWebMonitorForm)))
	mux.Handle("GET /monitors/{id}", webAuth(http.HandlerFunc(s.handleWebMonitorDetail)))
	mux.Handle("GET /monitors/{id}/edit", webAuth(http.HandlerFunc(s.handleWebMonitorForm)))
	mux.Handle("POST /monitors", webAdmin(s.handleWebMonitorCreate))
	mux.Handle("POST /monitors/{id}", webAdmin(s.handleWebMonitorUpdate))
	mux.Handle("POST /monitors/{id}/delete", webAdmin(s.handleWebMonitorDelete))
	mux.Handle("POST /monitors/{id}/pause", webAdmin(s.handleWebMonitorPause))
	mux.Handle("POST /monitors/{id}/resume", webAdmin(s.handleWebMonitorResume))

	mux.Handle("GET /incidents", webAuth(http.HandlerFunc(s.handleWebIncidents)))
	mux.Handle("GET /incidents/{id}", webAuth(http.HandlerFunc(s.handleWebIncidentDetail)))
	mux.Handle("POST /incidents/{id}/ack", webAdmin(s.handleWebIncidentAck))
	mux.Handle("POST /incidents/{id}/resolve", webAdmin(s.handleWebIncidentResolve))

	mux.Handle("GET /notifications", webAuth(http.HandlerFunc(s.handleWebNotifications)))
	mux.Handle("POST /notifications", webAdmin(s.handleWebNotificationCreate))
	mux.Handle("POST /notifications/{id}", webAdmin(s.handleWebNotificationUpdate))
	mux.Handle("POST /notifications/{id}/delete", webAdmin(s.handleWebNotificationDelete))
	mux.Handle("POST /notifications/{id}/test", webAdmin(s.handleWebNotificationTest))

	mux.Handle("GET /maintenance", webAuth(http.HandlerFunc(s.handleWebMaintenance)))
	mux.Handle("POST /maintenance", webAdmin(s.handleWebMaintenanceCreate))
	mux.Handle("POST /maintenance/{id}", webAdmin(s.handleWebMaintenanceUpdate))
	mux.Handle("POST /maintenance/{id}/delete", webAdmin(s.handleWebMaintenanceDelete))

	// API
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.Handle("GET /metrics", readAuth(http.HandlerFunc(s.handleMetrics)))
	mux.HandleFunc("POST /api/v1/heartbeat/{token}", s.handleHeartbeatPing)
	mux.HandleFunc("GET /api/v1/heartbeat/{token}", s.handleHeartbeatPing)
	mux.HandleFunc("GET /api/v1/badge/{id}/status", s.handleBadgeStatus)
	mux.HandleFunc("GET /api/v1/badge/{id}/uptime", s.handleBadgeUptime)
	mux.HandleFunc("GET /api/v1/badge/{id}/response", s.handleBadgeResponseTime)

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
