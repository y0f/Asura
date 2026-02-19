package api

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"time"

	"github.com/y0f/Asura/web"
)

type pageData struct {
	Title    string
	Active   string
	Perms    map[string]bool
	Version  string
	Flash    string
	Error    string
	BasePath string
	Data     interface{}
}

func (s *Server) newPageData(r *http.Request, title, active string) pageData {
	perms := make(map[string]bool)
	if k := getAPIKey(r.Context()); k != nil {
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
		Version:  s.version,
		Flash:    flash,
		BasePath: s.cfg.Server.BasePath,
	}
}

var templateFuncs = template.FuncMap{
	"statusColor": func(status string) string {
		switch status {
		case "up":
			return "text-emerald-400"
		case "down":
			return "text-red-400"
		case "degraded":
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
		case "degraded":
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
		case "degraded", "acknowledged":
			return "bg-yellow-400"
		default:
			return "bg-gray-500"
		}
	},
	"timeAgo": func(t interface{}) string {
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
			return "bg-surface-300"
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
	"safeCSS": func(s string) template.CSS { return template.CSS(s) },
	"add":     func(a, b int) int { return a + b },
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

func (s *Server) loadTemplates() {
	templateFS, _ := fs.Sub(web.FS, "templates")
	layoutTmpl := template.Must(template.New("").Funcs(templateFuncs).ParseFS(templateFS, "layout.html"))

	s.templates = make(map[string]*template.Template)

	pages := []string{
		"dashboard.html",
		"monitors.html",
		"monitor_detail.html",
		"monitor_form.html",
		"incidents.html",
		"incident_detail.html",
		"notifications.html",
		"maintenance.html",
		"request_logs.html",
		"status_settings.html",
	}

	for _, page := range pages {
		t := template.Must(template.Must(layoutTmpl.Clone()).ParseFS(templateFS, page))
		s.templates[page] = t
	}

	s.templates["login.html"] = template.Must(template.New("").Funcs(templateFuncs).ParseFS(templateFS, "login.html"))
	s.templates["status_page.html"] = template.Must(template.New("").Funcs(templateFuncs).ParseFS(templateFS, "status_page.html"))
}

func (s *Server) render(w http.ResponseWriter, tmpl string, data pageData) {
	t, ok := s.templates[tmpl]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	layoutName := "layout"
	if tmpl == "login.html" {
		layoutName = "login"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.tailwindcss.com https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdn.jsdelivr.net; font-src https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; "+s.cspFrameDirective)
	if err := t.ExecuteTemplate(w, layoutName, data); err != nil {
		s.logger.Error("template render", "template", tmpl, "error", err)
	}
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, s.cfg.Server.BasePath+path, http.StatusSeeOther)
}

func (s *Server) setFlash(w http.ResponseWriter, msg string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "flash",
		Value:    url.QueryEscape(msg),
		Path:     s.cfg.Server.BasePath + "/",
		MaxAge:   5,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
