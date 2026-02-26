package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/y0f/asura/internal/config"
	"github.com/y0f/asura/internal/httputil"
	"github.com/y0f/asura/internal/monitor"
	"github.com/y0f/asura/internal/notifier"
	"github.com/y0f/asura/internal/storage"
	webfs "github.com/y0f/asura/web"
)

type Handler struct {
	cfg                *config.Config
	store              storage.Store
	pipeline           *monitor.Pipeline
	notifier           *notifier.Dispatcher
	logger             *slog.Logger
	version            string
	cspFrameDirective  string
	OnStatusPageChange func()
	templates          map[string]*template.Template
	loginRL            *httputil.RateLimiter
	totpMu             sync.Mutex
	totpChallenges     map[string]*totpChallenge
	done               chan struct{}
}

func (h *Handler) Stop() {
	close(h.done)
	h.loginRL.Stop()
}

func New(cfg *config.Config, store storage.Store, pipeline *monitor.Pipeline,
	dispatcher *notifier.Dispatcher, logger *slog.Logger, version, cspDirective string) *Handler {
	h := &Handler{
		cfg:               cfg,
		store:             store,
		pipeline:          pipeline,
		notifier:          dispatcher,
		logger:            logger,
		version:           version,
		cspFrameDirective: cspDirective,
		loginRL:           httputil.NewRateLimiter(cfg.Auth.Login.RateLimitPerSec, cfg.Auth.Login.RateLimitBurst),
		totpChallenges:    make(map[string]*totpChallenge),
		done:              make(chan struct{}),
	}
	go h.cleanupTOTPChallenges()
	h.loadTemplates()
	return h
}

var _jsEscaper = strings.NewReplacer(
	"</", `<\/`,
	"<!--", `<\!--`,
)

func safeJS(data []byte) template.JS {
	return template.JS(_jsEscaper.Replace(string(data)))
}

type pageData struct {
	Title    string
	Active   string
	Perms    map[string]bool
	Version  string
	Flash    string
	Error    string
	BasePath string
	Data     any
}

func (h *Handler) newPageData(r *http.Request, title, active string) pageData {
	perms := make(map[string]bool)
	if k := httputil.GetAPIKey(r.Context()); k != nil {
		perms = k.PermissionMap()
	}
	flash := ""
	if c, err := r.Cookie("flash"); err == nil {
		flash, _ = url.QueryUnescape(c.Value)
	}
	return pageData{
		Title:    title,
		Active:   active,
		Perms:    perms,
		Version:  h.version,
		Flash:    flash,
		BasePath: h.cfg.Server.BasePath,
	}
}

var _templateFuncs = template.FuncMap{
	"statusColor": func(status string) string {
		switch status {
		case "up":
			return "text-emerald-400"
		case "down":
			return "text-red-400"
		case "degraded", "paused":
			return "text-yellow-400"
		default:
			return "text-gray-500"
		}
	},
	"statusBg": func(status string) string {
		switch status {
		case "up":
			return "bg-emerald-500/10 text-emerald-400 border-emerald-500/20"
		case "down":
			return "bg-red-500/10 text-red-400 border-red-500/20"
		case "degraded", "paused":
			return "bg-yellow-500/10 text-yellow-400 border-yellow-500/20"
		case "open":
			return "bg-red-500/10 text-red-400 border-red-500/20"
		case "acknowledged":
			return "bg-yellow-500/10 text-yellow-400 border-yellow-500/20"
		case "resolved":
			return "bg-emerald-500/10 text-emerald-400 border-emerald-500/20"
		default:
			return "bg-gray-500/10 text-gray-400 border-gray-500/20"
		}
	},
	"statusDot": func(status string) string {
		switch status {
		case "up", "resolved":
			return "bg-emerald-400"
		case "down", "created":
			return "bg-red-400"
		case "degraded", "acknowledged", "paused":
			return "bg-yellow-400"
		default:
			return "bg-gray-500"
		}
	},
	"timeAgo": func(t any) string {
		var tm time.Time
		switch v := t.(type) {
		case time.Time:
			tm = v
		case *time.Time:
			if v == nil {
				return "never"
			}
			tm = *v
		default:
			return ""
		}
		d := time.Since(tm)
		switch {
		case d < time.Minute:
			return "just now"
		case d < time.Hour:
			m := int(d.Minutes())
			if m == 1 {
				return "1m ago"
			}
			return template.HTMLEscapeString(fmt.Sprintf("%dm ago", m))
		case d < 24*time.Hour:
			h := int(d.Hours())
			if h == 1 {
				return "1h ago"
			}
			return template.HTMLEscapeString(fmt.Sprintf("%dh ago", h))
		default:
			days := int(d.Hours() / 24)
			if days == 1 {
				return "1d ago"
			}
			return template.HTMLEscapeString(fmt.Sprintf("%dd ago", days))
		}
	},
	"formatMs": func(ms int64) string {
		if ms < 1000 {
			return fmt.Sprintf("%dms", ms)
		}
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	},
	"incidentDuration": func(started time.Time, resolved *time.Time) string {
		end := time.Now()
		if resolved != nil {
			end = *resolved
		}
		d := end.Sub(started)
		if d < time.Minute {
			return "< 1m"
		}
		if d < time.Hour {
			return fmt.Sprintf("%dm", int(d.Minutes()))
		}
		if d < 24*time.Hour {
			return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
		}
		return fmt.Sprintf("%dd %dh", int(d.Hours()/24), int(d.Hours())%24)
	},
	"uptimeFmt": func(pct float64) string {
		if pct >= 99.995 {
			return "100%"
		}
		return fmt.Sprintf("%.2f%%", pct)
	},
	"uptimeColor": func(pct float64) string {
		if pct >= 99.9 {
			return "text-emerald-400"
		}
		if pct >= 99 {
			return "text-yellow-400"
		}
		return "text-red-400"
	},
	"httpStatusColor": func(code int) string {
		switch {
		case code >= 200 && code < 300:
			return "text-emerald-400"
		case code >= 300 && code < 400:
			return "text-blue-400"
		case code >= 400 && code < 500:
			return "text-yellow-400"
		case code >= 500:
			return "text-red-400"
		default:
			return "text-gray-500"
		}
	},
	"certDays": func(t *time.Time) int {
		if t == nil {
			return -1
		}
		return int(time.Until(*t).Hours() / 24)
	},
	"certColor": func(t *time.Time) string {
		if t == nil {
			return "text-gray-500"
		}
		days := int(time.Until(*t).Hours() / 24)
		if days < 7 {
			return "text-red-400"
		}
		if days < 30 {
			return "text-yellow-400"
		}
		return "text-emerald-400"
	},
	"typeLabel": func(t string) string {
		switch t {
		case "http":
			return "HTTP"
		case "tcp":
			return "TCP"
		case "dns":
			return "DNS"
		case "icmp":
			return "ICMP"
		case "tls":
			return "TLS"
		case "websocket":
			return "WebSocket"
		case "command":
			return "Command"
		case "heartbeat":
			return "Heartbeat"
		case "docker":
			return "Docker"
		case "domain":
			return "Domain"
		case "grpc":
			return "gRPC"
		case "mqtt":
			return "MQTT"
		default:
			return t
		}
	},
	"formatFloat": func(f float64) string {
		if f == 0 {
			return "-"
		}
		if f < 1000 {
			return fmt.Sprintf("%.0fms", f)
		}
		return fmt.Sprintf("%.1fs", f/1000)
	},
	"uptimeBarColor": func(pct float64, hasData bool) string {
		if !hasData {
			return "bg-muted/20"
		}
		if pct >= 99 {
			return "bg-emerald-500"
		}
		if pct >= 95 {
			return "bg-yellow-500"
		}
		return "bg-red-500"
	},
	"uptimeBarTooltip": func(pct float64, hasData bool, label string) string {
		if !hasData {
			return label + " — No data"
		}
		if pct >= 99.995 {
			return label + " — 100% uptime"
		}
		return fmt.Sprintf("%s — %.2f%% uptime", label, pct)
	},
	"toJSON": func(v any) template.JS {
		b, _ := json.Marshal(v)
		return safeJS(b)
	},
	"safeCSS": func(s string) template.CSS { return template.CSS(s) },
	"list":    func(args ...string) []string { return args },
	"deref": func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	},
	"inSlice": func(needle int64, haystack []int64) bool {
		for _, v := range haystack {
			if v == needle {
				return true
			}
		}
		return false
	},
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"parseDNS": func(s string) []string {
		if s == "" {
			return nil
		}
		var records []string
		json.Unmarshal([]byte(s), &records)
		return records
	},
}

func (h *Handler) loadTemplates() {
	templateFS, _ := fs.Sub(webfs.FS, "templates")
	layoutTmpl := template.Must(template.New("").Funcs(_templateFuncs).ParseFS(templateFS, "layouts/base.html"))

	h.templates = make(map[string]*template.Template)

	pages := []string{
		"dashboard.html",
		"requestlogs/list.html",
		"monitors/list.html",
		"monitors/detail.html",
		"monitors/form.html",
		"groups/list.html",
		"groups/detail.html",
		"incidents/list.html",
		"incidents/detail.html",
		"notifications/list.html",
		"maintenance/list.html",
		"status/list.html",
		"status/form.html",
		"proxies/list.html",
		"proxies/form.html",
		"settings/index.html",
	}

	for _, page := range pages {
		t := template.Must(template.Must(layoutTmpl.Clone()).ParseFS(templateFS, page))
		h.templates[page] = t
	}

	h.templates["auth/login.html"] = template.Must(template.New("").Funcs(_templateFuncs).ParseFS(templateFS, "auth/login.html"))
	h.templates["auth/totp.html"] = template.Must(template.New("").Funcs(_templateFuncs).ParseFS(templateFS, "auth/totp.html"))
	h.templates["status/page.html"] = template.Must(template.New("").Funcs(_templateFuncs).ParseFS(templateFS, "status/page.html"))
}

func (h *Handler) render(w http.ResponseWriter, tmpl string, data pageData) {
	t, ok := h.templates[tmpl]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	layoutName := "layout"
	if tmpl == "auth/login.html" {
		layoutName = "login"
	} else if tmpl == "auth/totp.html" {
		layoutName = "login_totp"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; "+h.cspFrameDirective)
	if err := t.ExecuteTemplate(w, layoutName, data); err != nil {
		h.logger.Error("template render", "template", tmpl, "error", err)
	}
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, h.cfg.Server.BasePath+path, http.StatusSeeOther)
}

func (h *Handler) setFlash(w http.ResponseWriter, msg string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "flash",
		Value:    url.QueryEscape(msg),
		Path:     h.cfg.Server.BasePath + "/",
		MaxAge:   5,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) audit(r *http.Request, action, entity string, entityID int64, detail string) {
	entry := &storage.AuditEntry{
		Action:     action,
		Entity:     entity,
		EntityID:   entityID,
		APIKeyName: httputil.GetAPIKeyName(r.Context()),
		Detail:     detail,
	}
	if err := h.store.InsertAudit(r.Context(), entry); err != nil {
		h.logger.Error("audit log failed", "error", err)
	}
}
