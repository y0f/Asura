package api

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/y0f/Asura/internal/config"
	"github.com/y0f/Asura/internal/monitor"
	"github.com/y0f/Asura/internal/notifier"
	"github.com/y0f/Asura/internal/storage"
	"github.com/y0f/Asura/web"
)

type Server struct {
	cfg               *config.Config
	store             storage.Store
	pipeline          *monitor.Pipeline
	notifier          *notifier.Dispatcher
	logger            *slog.Logger
	handler           http.Handler
	version           string
	templates         map[string]*template.Template
	loginRL           *rateLimiter
	cspFrameDirective string
}

func NewServer(cfg *config.Config, store storage.Store, pipeline *monitor.Pipeline, dispatcher *notifier.Dispatcher, logger *slog.Logger, version string) *Server {
	s := &Server{
		cfg:               cfg,
		store:             store,
		pipeline:          pipeline,
		notifier:          dispatcher,
		logger:            logger,
		version:           version,
		loginRL:           newRateLimiter(cfg.Auth.Login.RateLimitPerSec, cfg.Auth.Login.RateLimitBurst),
		cspFrameDirective: buildFrameAncestorsDirective(cfg.Server.FrameAncestors),
	}

	s.loadTemplates()

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	var handler http.Handler = mux
	handler = bodyLimit(cfg.Server.MaxBodySize)(handler)
	rl := newRateLimiter(cfg.Server.RateLimitPerSec, cfg.Server.RateLimitBurst)
	handler = rl.middleware()(handler)
	handler = cors(cfg.Server.CORSOrigins)(handler)
	handler = secureHeaders(cfg.Server.FrameAncestors)(handler)
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
	monRead := auth(s.cfg, "monitors.read")
	monWrite := auth(s.cfg, "monitors.write")
	incRead := auth(s.cfg, "incidents.read")
	incWrite := auth(s.cfg, "incidents.write")
	notifRead := auth(s.cfg, "notifications.read")
	notifWrite := auth(s.cfg, "notifications.write")
	maintRead := auth(s.cfg, "maintenance.read")
	maintWrite := auth(s.cfg, "maintenance.write")
	metricsRead := auth(s.cfg, "metrics.read")

	if s.cfg.IsWebUIEnabled() {
		// Static files
		staticFS, _ := fs.Sub(web.FS, "static")
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

		// Web UI
		mux.HandleFunc("GET /login", s.handleWebLogin)
		mux.HandleFunc("POST /login", s.handleWebLoginPost)
		mux.HandleFunc("POST /logout", s.handleWebLogout)

		webAuth := s.webAuth
		webPerm := func(perm string, h http.HandlerFunc) http.Handler {
			return webAuth(s.webRequirePerm(perm, http.HandlerFunc(h)))
		}

		mux.Handle("GET /{$}", webAuth(http.HandlerFunc(s.handleWebDashboard)))
		mux.Handle("GET /monitors", webAuth(http.HandlerFunc(s.handleWebMonitors)))
		mux.Handle("GET /monitors/new", webAuth(http.HandlerFunc(s.handleWebMonitorForm)))
		mux.Handle("GET /monitors/{id}", webAuth(http.HandlerFunc(s.handleWebMonitorDetail)))
		mux.Handle("GET /monitors/{id}/edit", webAuth(http.HandlerFunc(s.handleWebMonitorForm)))
		mux.Handle("POST /monitors", webPerm("monitors.write", s.handleWebMonitorCreate))
		mux.Handle("POST /monitors/{id}", webPerm("monitors.write", s.handleWebMonitorUpdate))
		mux.Handle("POST /monitors/{id}/delete", webPerm("monitors.write", s.handleWebMonitorDelete))
		mux.Handle("POST /monitors/{id}/pause", webPerm("monitors.write", s.handleWebMonitorPause))
		mux.Handle("POST /monitors/{id}/resume", webPerm("monitors.write", s.handleWebMonitorResume))

		mux.Handle("GET /incidents", webAuth(http.HandlerFunc(s.handleWebIncidents)))
		mux.Handle("GET /incidents/{id}", webAuth(http.HandlerFunc(s.handleWebIncidentDetail)))
		mux.Handle("POST /incidents/{id}/ack", webPerm("incidents.write", s.handleWebIncidentAck))
		mux.Handle("POST /incidents/{id}/resolve", webPerm("incidents.write", s.handleWebIncidentResolve))

		mux.Handle("GET /notifications", webAuth(http.HandlerFunc(s.handleWebNotifications)))
		mux.Handle("POST /notifications", webPerm("notifications.write", s.handleWebNotificationCreate))
		mux.Handle("POST /notifications/{id}", webPerm("notifications.write", s.handleWebNotificationUpdate))
		mux.Handle("POST /notifications/{id}/delete", webPerm("notifications.write", s.handleWebNotificationDelete))
		mux.Handle("POST /notifications/{id}/test", webPerm("notifications.write", s.handleWebNotificationTest))

		mux.Handle("GET /maintenance", webAuth(http.HandlerFunc(s.handleWebMaintenance)))
		mux.Handle("POST /maintenance", webPerm("maintenance.write", s.handleWebMaintenanceCreate))
		mux.Handle("POST /maintenance/{id}", webPerm("maintenance.write", s.handleWebMaintenanceUpdate))
		mux.Handle("POST /maintenance/{id}/delete", webPerm("maintenance.write", s.handleWebMaintenanceDelete))
	}

	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.Handle("GET /metrics", metricsRead(http.HandlerFunc(s.handleMetrics)))
	mux.HandleFunc("POST /api/v1/heartbeat/{token}", s.handleHeartbeatPing)
	mux.HandleFunc("GET /api/v1/heartbeat/{token}", s.handleHeartbeatPing)
	mux.HandleFunc("GET /api/v1/badge/{id}/status", s.handleBadgeStatus)
	mux.HandleFunc("GET /api/v1/badge/{id}/uptime", s.handleBadgeUptime)
	mux.HandleFunc("GET /api/v1/badge/{id}/response", s.handleBadgeResponseTime)

	mux.Handle("GET /api/v1/monitors", monRead(http.HandlerFunc(s.handleListMonitors)))
	mux.Handle("GET /api/v1/monitors/{id}", monRead(http.HandlerFunc(s.handleGetMonitor)))
	mux.Handle("GET /api/v1/monitors/{id}/checks", monRead(http.HandlerFunc(s.handleListChecks)))
	mux.Handle("GET /api/v1/monitors/{id}/metrics", monRead(http.HandlerFunc(s.handleMonitorMetrics)))
	mux.Handle("GET /api/v1/monitors/{id}/changes", monRead(http.HandlerFunc(s.handleListChanges)))

	mux.Handle("GET /api/v1/incidents", incRead(http.HandlerFunc(s.handleListIncidents)))
	mux.Handle("GET /api/v1/incidents/{id}", incRead(http.HandlerFunc(s.handleGetIncident)))

	mux.Handle("GET /api/v1/notifications", notifRead(http.HandlerFunc(s.handleListNotifications)))
	mux.Handle("GET /api/v1/maintenance", maintRead(http.HandlerFunc(s.handleListMaintenance)))
	mux.Handle("GET /api/v1/overview", monRead(http.HandlerFunc(s.handleOverview)))
	mux.Handle("GET /api/v1/tags", monRead(http.HandlerFunc(s.handleListTags)))

	mux.Handle("POST /api/v1/monitors", monWrite(http.HandlerFunc(s.handleCreateMonitor)))
	mux.Handle("PUT /api/v1/monitors/{id}", monWrite(http.HandlerFunc(s.handleUpdateMonitor)))
	mux.Handle("DELETE /api/v1/monitors/{id}", monWrite(http.HandlerFunc(s.handleDeleteMonitor)))
	mux.Handle("POST /api/v1/monitors/{id}/pause", monWrite(http.HandlerFunc(s.handlePauseMonitor)))
	mux.Handle("POST /api/v1/monitors/{id}/resume", monWrite(http.HandlerFunc(s.handleResumeMonitor)))

	mux.Handle("POST /api/v1/incidents/{id}/ack", incWrite(http.HandlerFunc(s.handleAckIncident)))
	mux.Handle("POST /api/v1/incidents/{id}/resolve", incWrite(http.HandlerFunc(s.handleResolveIncident)))

	mux.Handle("POST /api/v1/notifications", notifWrite(http.HandlerFunc(s.handleCreateNotification)))
	mux.Handle("PUT /api/v1/notifications/{id}", notifWrite(http.HandlerFunc(s.handleUpdateNotification)))
	mux.Handle("DELETE /api/v1/notifications/{id}", notifWrite(http.HandlerFunc(s.handleDeleteNotification)))
	mux.Handle("POST /api/v1/notifications/{id}/test", notifWrite(http.HandlerFunc(s.handleTestNotification)))

	mux.Handle("POST /api/v1/maintenance", maintWrite(http.HandlerFunc(s.handleCreateMaintenance)))
	mux.Handle("PUT /api/v1/maintenance/{id}", maintWrite(http.HandlerFunc(s.handleUpdateMaintenance)))
	mux.Handle("DELETE /api/v1/maintenance/{id}", maintWrite(http.HandlerFunc(s.handleDeleteMaintenance)))
}
