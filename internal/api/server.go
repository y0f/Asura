package api

import (
	"context"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"

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
	reqLogWriter      *RequestLogWriter
	statusSlugMu      sync.RWMutex
	statusSlug        string
	statusSlugsMu     sync.RWMutex
	statusSlugs       map[string]int64
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
		reqLogWriter:      NewRequestLogWriter(store, logger),
		statusSlugs:       make(map[string]int64),
	}

	s.loadTemplates()
	s.loadStatusSlug()
	s.refreshStatusSlugs()

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	var handler http.Handler = mux
	if cfg.IsWebUIEnabled() {
		handler = s.statusPageRouter(handler)
	}
	handler = bodyLimit(cfg.Server.MaxBodySize)(handler)
	rl := newRateLimiter(cfg.Server.RateLimitPerSec, cfg.Server.RateLimitBurst)
	handler = rl.middleware(cfg.TrustedNets())(handler)
	handler = cors(cfg.Server.CORSOrigins)(handler)
	handler = secureHeaders(cfg.Server.FrameAncestors)(handler)
	handler = requestLogMiddleware(s.reqLogWriter, cfg.Server.BasePath, cfg.TrustedNets())(handler)
	handler = logging(logger)(handler)
	handler = requestID()(handler)
	handler = recovery(logger)(handler)

	s.handler = handler
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// RequestLogWriter returns the writer so the caller can start its Run goroutine.
func (s *Server) RequestLogWriter() *RequestLogWriter {
	return s.reqLogWriter
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
		webAuth := s.webAuth
		webPerm := func(perm string, h http.HandlerFunc) http.Handler {
			return webAuth(s.webRequirePerm(perm, http.HandlerFunc(h)))
		}

		mux.Handle("POST "+s.p("/logout"), webAuth(http.HandlerFunc(s.handleWebLogout)))

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
		mux.Handle("GET "+s.p("/monitors/{id}/chart"), webAuth(http.HandlerFunc(s.handleMonitorChart)))

		mux.Handle("GET "+s.p("/incidents"), webAuth(http.HandlerFunc(s.handleWebIncidents)))
		mux.Handle("GET "+s.p("/incidents/{id}"), webAuth(http.HandlerFunc(s.handleWebIncidentDetail)))
		mux.Handle("POST "+s.p("/incidents/{id}/ack"), webPerm("incidents.write", s.handleWebIncidentAck))
		mux.Handle("POST "+s.p("/incidents/{id}/resolve"), webPerm("incidents.write", s.handleWebIncidentResolve))
		mux.Handle("POST "+s.p("/incidents/{id}/delete"), webPerm("incidents.write", s.handleWebIncidentDelete))

		mux.Handle("GET "+s.p("/groups"), webAuth(http.HandlerFunc(s.handleWebGroups)))
		mux.Handle("POST "+s.p("/groups"), webPerm("monitors.write", s.handleWebGroupCreate))
		mux.Handle("POST "+s.p("/groups/{id}"), webPerm("monitors.write", s.handleWebGroupUpdate))
		mux.Handle("POST "+s.p("/groups/{id}/delete"), webPerm("monitors.write", s.handleWebGroupDelete))

		mux.Handle("GET "+s.p("/notifications"), webAuth(http.HandlerFunc(s.handleWebNotifications)))
		mux.Handle("POST "+s.p("/notifications"), webPerm("notifications.write", s.handleWebNotificationCreate))
		mux.Handle("POST "+s.p("/notifications/{id}"), webPerm("notifications.write", s.handleWebNotificationUpdate))
		mux.Handle("POST "+s.p("/notifications/{id}/delete"), webPerm("notifications.write", s.handleWebNotificationDelete))
		mux.Handle("POST "+s.p("/notifications/{id}/test"), webPerm("notifications.write", s.handleWebNotificationTest))

		mux.Handle("GET "+s.p("/maintenance"), webAuth(http.HandlerFunc(s.handleWebMaintenance)))
		mux.Handle("POST "+s.p("/maintenance"), webPerm("maintenance.write", s.handleWebMaintenanceCreate))
		mux.Handle("POST "+s.p("/maintenance/{id}"), webPerm("maintenance.write", s.handleWebMaintenanceUpdate))
		mux.Handle("POST "+s.p("/maintenance/{id}/delete"), webPerm("maintenance.write", s.handleWebMaintenanceDelete))

		mux.Handle("GET "+s.p("/logs"), webAuth(http.HandlerFunc(s.handleWebRequestLogs)))

		// Status pages admin
		mux.Handle("GET "+s.p("/status-pages"), webAuth(http.HandlerFunc(s.handleWebStatusPages)))
		mux.Handle("GET "+s.p("/status-pages/new"), webAuth(http.HandlerFunc(s.handleWebStatusPageForm)))
		mux.Handle("GET "+s.p("/status-pages/{id}/edit"), webAuth(http.HandlerFunc(s.handleWebStatusPageForm)))
		mux.Handle("POST "+s.p("/status-pages"), webPerm("monitors.write", s.handleWebStatusPageCreate))
		mux.Handle("POST "+s.p("/status-pages/{id}"), webPerm("monitors.write", s.handleWebStatusPageUpdate))
		mux.Handle("POST "+s.p("/status-pages/{id}/delete"), webPerm("monitors.write", s.handleWebStatusPageDelete))

		// Legacy status settings redirect
		mux.Handle("GET "+s.p("/status-settings"), webAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.redirect(w, r, "/status-pages")
		})))
	}

	mux.HandleFunc("GET "+s.p("/api/v1/status"), s.handlePublicStatus)
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
	mux.Handle("GET "+s.p("/api/v1/monitors/{id}/chart"), monRead(http.HandlerFunc(s.handleMonitorChart)))

	mux.Handle("GET "+s.p("/api/v1/incidents"), incRead(http.HandlerFunc(s.handleListIncidents)))
	mux.Handle("GET "+s.p("/api/v1/incidents/{id}"), incRead(http.HandlerFunc(s.handleGetIncident)))

	mux.Handle("GET "+s.p("/api/v1/notifications"), notifRead(http.HandlerFunc(s.handleListNotifications)))
	mux.Handle("GET "+s.p("/api/v1/maintenance"), maintRead(http.HandlerFunc(s.handleListMaintenance)))
	mux.Handle("GET "+s.p("/api/v1/groups"), monRead(http.HandlerFunc(s.handleListGroups)))
	mux.Handle("POST "+s.p("/api/v1/groups"), monWrite(http.HandlerFunc(s.handleCreateGroup)))
	mux.Handle("PUT "+s.p("/api/v1/groups/{id}"), monWrite(http.HandlerFunc(s.handleUpdateGroup)))
	mux.Handle("DELETE "+s.p("/api/v1/groups/{id}"), monWrite(http.HandlerFunc(s.handleDeleteGroup)))
	mux.Handle("GET "+s.p("/api/v1/overview"), monRead(http.HandlerFunc(s.handleOverview)))
	mux.Handle("GET "+s.p("/api/v1/tags"), monRead(http.HandlerFunc(s.handleListTags)))

	mux.Handle("POST "+s.p("/api/v1/monitors"), monWrite(http.HandlerFunc(s.handleCreateMonitor)))
	mux.Handle("PUT "+s.p("/api/v1/monitors/{id}"), monWrite(http.HandlerFunc(s.handleUpdateMonitor)))
	mux.Handle("DELETE "+s.p("/api/v1/monitors/{id}"), monWrite(http.HandlerFunc(s.handleDeleteMonitor)))
	mux.Handle("POST "+s.p("/api/v1/monitors/{id}/pause"), monWrite(http.HandlerFunc(s.handlePauseMonitor)))
	mux.Handle("POST "+s.p("/api/v1/monitors/{id}/resume"), monWrite(http.HandlerFunc(s.handleResumeMonitor)))

	mux.Handle("POST "+s.p("/api/v1/incidents/{id}/ack"), incWrite(http.HandlerFunc(s.handleAckIncident)))
	mux.Handle("POST "+s.p("/api/v1/incidents/{id}/resolve"), incWrite(http.HandlerFunc(s.handleResolveIncident)))
	mux.Handle("DELETE "+s.p("/api/v1/incidents/{id}"), incWrite(http.HandlerFunc(s.handleDeleteIncident)))

	mux.Handle("POST "+s.p("/api/v1/notifications"), notifWrite(http.HandlerFunc(s.handleCreateNotification)))
	mux.Handle("PUT "+s.p("/api/v1/notifications/{id}"), notifWrite(http.HandlerFunc(s.handleUpdateNotification)))
	mux.Handle("DELETE "+s.p("/api/v1/notifications/{id}"), notifWrite(http.HandlerFunc(s.handleDeleteNotification)))
	mux.Handle("POST "+s.p("/api/v1/notifications/{id}/test"), notifWrite(http.HandlerFunc(s.handleTestNotification)))

	mux.Handle("POST "+s.p("/api/v1/maintenance"), maintWrite(http.HandlerFunc(s.handleCreateMaintenance)))
	mux.Handle("PUT "+s.p("/api/v1/maintenance/{id}"), maintWrite(http.HandlerFunc(s.handleUpdateMaintenance)))
	mux.Handle("DELETE "+s.p("/api/v1/maintenance/{id}"), maintWrite(http.HandlerFunc(s.handleDeleteMaintenance)))

	// Legacy status config API (backward compat)
	mux.Handle("GET "+s.p("/api/v1/status/config"), monRead(http.HandlerFunc(s.handleGetStatusConfig)))
	mux.Handle("PUT "+s.p("/api/v1/status/config"), monWrite(http.HandlerFunc(s.handleUpdateStatusConfig)))

	// Status pages API
	mux.Handle("GET "+s.p("/api/v1/status-pages"), monRead(http.HandlerFunc(s.handleListStatusPages)))
	mux.Handle("GET "+s.p("/api/v1/status-pages/{id}"), monRead(http.HandlerFunc(s.handleGetStatusPage)))
	mux.Handle("POST "+s.p("/api/v1/status-pages"), monWrite(http.HandlerFunc(s.handleCreateStatusPage)))
	mux.Handle("PUT "+s.p("/api/v1/status-pages/{id}"), monWrite(http.HandlerFunc(s.handleUpdateStatusPage)))
	mux.Handle("DELETE "+s.p("/api/v1/status-pages/{id}"), monWrite(http.HandlerFunc(s.handleDeleteStatusPage)))
	mux.HandleFunc("GET "+s.p("/api/v1/status-pages/{id}/public"), s.handlePublicStatusPage)

	mux.Handle("GET "+s.p("/api/v1/request-logs"), metricsRead(http.HandlerFunc(s.handleListRequestLogs)))
	mux.Handle("GET "+s.p("/api/v1/request-logs/stats"), metricsRead(http.HandlerFunc(s.handleRequestLogStats)))
}

func (s *Server) loadStatusSlug() {
	cfg, err := s.store.GetStatusPageConfig(context.Background())
	if err != nil {
		s.statusSlug = "status"
		return
	}
	if cfg.Slug == "" {
		s.statusSlug = "status"
		return
	}
	s.statusSlug = cfg.Slug
}

func (s *Server) getStatusSlug() string {
	s.statusSlugMu.RLock()
	defer s.statusSlugMu.RUnlock()
	return s.statusSlug
}

func (s *Server) setStatusSlug(slug string) {
	s.statusSlugMu.Lock()
	defer s.statusSlugMu.Unlock()
	s.statusSlug = slug
}

func (s *Server) refreshStatusSlugs() {
	pages, err := s.store.ListStatusPages(context.Background())
	if err != nil {
		s.logger.Error("refresh status slugs", "error", err)
		return
	}
	slugs := make(map[string]int64, len(pages))
	for _, p := range pages {
		if p.Enabled {
			slugs[p.Slug] = p.ID
		}
	}
	s.statusSlugsMu.Lock()
	s.statusSlugs = slugs
	s.statusSlugsMu.Unlock()
}

func (s *Server) getStatusPageIDBySlug(slug string) (int64, bool) {
	s.statusSlugsMu.RLock()
	defer s.statusSlugsMu.RUnlock()
	id, ok := s.statusSlugs[slug]
	return id, ok
}

func (s *Server) statusPageRouter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			path := r.URL.Path
			prefix := s.cfg.Server.BasePath + "/"
			if strings.HasPrefix(path, prefix) {
				slug := strings.TrimPrefix(path, prefix)
				slug = strings.TrimSuffix(slug, "/")
				// Only match single-segment slugs (no slashes)
				if slug != "" && !strings.Contains(slug, "/") {
					if pageID, ok := s.getStatusPageIDBySlug(slug); ok {
						s.handleWebStatusPageByID(w, r, pageID)
						return
					}
					// Fallback: legacy single slug
					if slug == s.getStatusSlug() {
						s.handleWebStatusPage(w, r)
						return
					}
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
