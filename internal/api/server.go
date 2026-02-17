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
	handler = rl.middleware(cfg.TrustedNets())(handler)
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

func (s *Server) p(path string) string {
	return s.cfg.Server.BasePath + path
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

	if s.cfg.Server.BasePath != "" {
		mux.HandleFunc("GET "+s.cfg.Server.BasePath, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, s.cfg.Server.BasePath+"/", http.StatusMovedPermanently)
		})
	}

	if s.cfg.IsWebUIEnabled() {
		// Static files
		staticFS, _ := fs.Sub(web.FS, "static")
		staticPrefix := s.p("/static/")
		mux.Handle("GET "+staticPrefix, http.StripPrefix(staticPrefix, http.FileServer(http.FS(staticFS))))

		// Web UI
		mux.HandleFunc("GET "+s.p("/login"), s.handleWebLogin)
		mux.HandleFunc("POST "+s.p("/login"), s.handleWebLoginPost)
		mux.HandleFunc("POST "+s.p("/logout"), s.handleWebLogout)

		webAuth := s.webAuth
		webPerm := func(perm string, h http.HandlerFunc) http.Handler {
			return webAuth(s.webRequirePerm(perm, http.HandlerFunc(h)))
		}

		mux.Handle("GET "+s.p("/{$}"), webAuth(http.HandlerFunc(s.handleWebDashboard)))
		mux.Handle("GET "+s.p("/monitors"), webAuth(http.HandlerFunc(s.handleWebMonitors)))
		mux.Handle("GET "+s.p("/monitors/new"), webAuth(http.HandlerFunc(s.handleWebMonitorForm)))
		mux.Handle("GET "+s.p("/monitors/{id}"), webAuth(http.HandlerFunc(s.handleWebMonitorDetail)))
		mux.Handle("GET "+s.p("/monitors/{id}/edit"), webAuth(http.HandlerFunc(s.handleWebMonitorForm)))
		mux.Handle("POST "+s.p("/monitors"), webPerm("monitors.write", s.handleWebMonitorCreate))
		mux.Handle("POST "+s.p("/monitors/{id}"), webPerm("monitors.write", s.handleWebMonitorUpdate))
		mux.Handle("POST "+s.p("/monitors/{id}/delete"), webPerm("monitors.write", s.handleWebMonitorDelete))
		mux.Handle("POST "+s.p("/monitors/{id}/pause"), webPerm("monitors.write", s.handleWebMonitorPause))
		mux.Handle("POST "+s.p("/monitors/{id}/resume"), webPerm("monitors.write", s.handleWebMonitorResume))

		mux.Handle("GET "+s.p("/incidents"), webAuth(http.HandlerFunc(s.handleWebIncidents)))
		mux.Handle("GET "+s.p("/incidents/{id}"), webAuth(http.HandlerFunc(s.handleWebIncidentDetail)))
		mux.Handle("POST "+s.p("/incidents/{id}/ack"), webPerm("incidents.write", s.handleWebIncidentAck))
		mux.Handle("POST "+s.p("/incidents/{id}/resolve"), webPerm("incidents.write", s.handleWebIncidentResolve))

		mux.Handle("GET "+s.p("/notifications"), webAuth(http.HandlerFunc(s.handleWebNotifications)))
		mux.Handle("POST "+s.p("/notifications"), webPerm("notifications.write", s.handleWebNotificationCreate))
		mux.Handle("POST "+s.p("/notifications/{id}"), webPerm("notifications.write", s.handleWebNotificationUpdate))
		mux.Handle("POST "+s.p("/notifications/{id}/delete"), webPerm("notifications.write", s.handleWebNotificationDelete))
		mux.Handle("POST "+s.p("/notifications/{id}/test"), webPerm("notifications.write", s.handleWebNotificationTest))

		mux.Handle("GET "+s.p("/maintenance"), webAuth(http.HandlerFunc(s.handleWebMaintenance)))
		mux.Handle("POST "+s.p("/maintenance"), webPerm("maintenance.write", s.handleWebMaintenanceCreate))
		mux.Handle("POST "+s.p("/maintenance/{id}"), webPerm("maintenance.write", s.handleWebMaintenanceUpdate))
		mux.Handle("POST "+s.p("/maintenance/{id}/delete"), webPerm("maintenance.write", s.handleWebMaintenanceDelete))
	}

	mux.HandleFunc("GET "+s.p("/api/v1/health"), s.handleHealth)
	mux.Handle("GET "+s.p("/metrics"), metricsRead(http.HandlerFunc(s.handleMetrics)))
	mux.HandleFunc("POST "+s.p("/api/v1/heartbeat/{token}"), s.handleHeartbeatPing)
	mux.HandleFunc("GET "+s.p("/api/v1/heartbeat/{token}"), s.handleHeartbeatPing)
	mux.HandleFunc("GET "+s.p("/api/v1/badge/{id}/status"), s.handleBadgeStatus)
	mux.HandleFunc("GET "+s.p("/api/v1/badge/{id}/uptime"), s.handleBadgeUptime)
	mux.HandleFunc("GET "+s.p("/api/v1/badge/{id}/response"), s.handleBadgeResponseTime)

	mux.Handle("GET "+s.p("/api/v1/monitors"), monRead(http.HandlerFunc(s.handleListMonitors)))
	mux.Handle("GET "+s.p("/api/v1/monitors/{id}"), monRead(http.HandlerFunc(s.handleGetMonitor)))
	mux.Handle("GET "+s.p("/api/v1/monitors/{id}/checks"), monRead(http.HandlerFunc(s.handleListChecks)))
	mux.Handle("GET "+s.p("/api/v1/monitors/{id}/metrics"), monRead(http.HandlerFunc(s.handleMonitorMetrics)))
	mux.Handle("GET "+s.p("/api/v1/monitors/{id}/changes"), monRead(http.HandlerFunc(s.handleListChanges)))

	mux.Handle("GET "+s.p("/api/v1/incidents"), incRead(http.HandlerFunc(s.handleListIncidents)))
	mux.Handle("GET "+s.p("/api/v1/incidents/{id}"), incRead(http.HandlerFunc(s.handleGetIncident)))

	mux.Handle("GET "+s.p("/api/v1/notifications"), notifRead(http.HandlerFunc(s.handleListNotifications)))
	mux.Handle("GET "+s.p("/api/v1/maintenance"), maintRead(http.HandlerFunc(s.handleListMaintenance)))
	mux.Handle("GET "+s.p("/api/v1/overview"), monRead(http.HandlerFunc(s.handleOverview)))
	mux.Handle("GET "+s.p("/api/v1/tags"), monRead(http.HandlerFunc(s.handleListTags)))

	mux.Handle("POST "+s.p("/api/v1/monitors"), monWrite(http.HandlerFunc(s.handleCreateMonitor)))
	mux.Handle("PUT "+s.p("/api/v1/monitors/{id}"), monWrite(http.HandlerFunc(s.handleUpdateMonitor)))
	mux.Handle("DELETE "+s.p("/api/v1/monitors/{id}"), monWrite(http.HandlerFunc(s.handleDeleteMonitor)))
	mux.Handle("POST "+s.p("/api/v1/monitors/{id}/pause"), monWrite(http.HandlerFunc(s.handlePauseMonitor)))
	mux.Handle("POST "+s.p("/api/v1/monitors/{id}/resume"), monWrite(http.HandlerFunc(s.handleResumeMonitor)))

	mux.Handle("POST "+s.p("/api/v1/incidents/{id}/ack"), incWrite(http.HandlerFunc(s.handleAckIncident)))
	mux.Handle("POST "+s.p("/api/v1/incidents/{id}/resolve"), incWrite(http.HandlerFunc(s.handleResolveIncident)))

	mux.Handle("POST "+s.p("/api/v1/notifications"), notifWrite(http.HandlerFunc(s.handleCreateNotification)))
	mux.Handle("PUT "+s.p("/api/v1/notifications/{id}"), notifWrite(http.HandlerFunc(s.handleUpdateNotification)))
	mux.Handle("DELETE "+s.p("/api/v1/notifications/{id}"), notifWrite(http.HandlerFunc(s.handleDeleteNotification)))
	mux.Handle("POST "+s.p("/api/v1/notifications/{id}/test"), notifWrite(http.HandlerFunc(s.handleTestNotification)))

	mux.Handle("POST "+s.p("/api/v1/maintenance"), maintWrite(http.HandlerFunc(s.handleCreateMaintenance)))
	mux.Handle("PUT "+s.p("/api/v1/maintenance/{id}"), maintWrite(http.HandlerFunc(s.handleUpdateMaintenance)))
	mux.Handle("DELETE "+s.p("/api/v1/maintenance/{id}"), maintWrite(http.HandlerFunc(s.handleDeleteMaintenance)))
}
