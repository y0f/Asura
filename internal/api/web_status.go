package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/y0f/Asura/internal/incident"
	"github.com/y0f/Asura/internal/storage"
)

type dailyBar struct {
	Date      string
	UptimePct float64
	HasData   bool
	Label     string
}

type monitorWithUptime struct {
	Monitor     *storage.Monitor
	DailyBars   []dailyBar
	Uptime90d   float64
	UptimeLabel string
	GroupName   string
}

func (s *Server) handleWebStatusPageByID(w http.ResponseWriter, r *http.Request, pageID int64) {
	ctx := r.Context()

	sp, err := s.store.GetStatusPage(ctx, pageID)
	if err != nil || sp == nil || !sp.Enabled {
		http.NotFound(w, r)
		return
	}

	monitors, spms, err := s.store.ListStatusPageMonitorsWithStatus(ctx, sp.ID)
	if err != nil {
		s.logger.Error("web: status page monitors", "error", err)
		monitors = []*storage.Monitor{}
		spms = []storage.StatusPageMonitor{}
	}

	now := time.Now().UTC()
	from := now.AddDate(0, 0, -90)

	groupNameMap := make(map[int64]string, len(spms))
	for _, spm := range spms {
		groupNameMap[spm.MonitorID] = spm.GroupName
	}

	var monitorData []monitorWithUptime
	for _, m := range monitors {
		bars := s.buildDailyBars(ctx, m.ID, from, now)
		uptime, err := s.store.GetUptimePercent(ctx, m.ID, from, now)
		if err != nil {
			s.logger.Error("web: status uptime percent", "monitor_id", m.ID, "error", err)
			uptime = 100
		}

		monitorData = append(monitorData, monitorWithUptime{
			Monitor:     m,
			DailyBars:   bars,
			Uptime90d:   uptime,
			UptimeLabel: formatPct(uptime),
			GroupName:   groupNameMap[m.ID],
		})
	}

	// Build grouped structure for template
	type monitorGroup struct {
		Name     string
		Monitors []monitorWithUptime
	}
	var groups []monitorGroup
	groupIdx := make(map[string]int)
	for _, md := range monitorData {
		gn := md.GroupName
		if idx, ok := groupIdx[gn]; ok {
			groups[idx].Monitors = append(groups[idx].Monitors, md)
		} else {
			groupIdx[gn] = len(groups)
			groups = append(groups, monitorGroup{Name: gn, Monitors: []monitorWithUptime{md}})
		}
	}

	overall := overallStatus(monitors)

	incidents := s.publicIncidentsForPage(ctx, sp, monitors, now)

	pd := pageData{
		Title:    sp.Title,
		BasePath: s.cfg.Server.BasePath,
		Data: map[string]interface{}{
			"Config":       sp,
			"Monitors":     monitorData,
			"Groups":       groups,
			"HasGroups":    len(groups) > 1 || (len(groups) == 1 && groups[0].Name != ""),
			"Overall":      overall,
			"Incidents":    incidents,
			"HasIncidents": len(incidents) > 0,
		},
	}
	s.renderStatusPage(w, pd)
}

func (s *Server) buildDailyBars(ctx context.Context, monitorID int64, from, now time.Time) []dailyBar {
	daily, err := s.store.GetDailyUptime(ctx, monitorID, from, now)
	if err != nil {
		s.logger.Error("web: status daily uptime", "monitor_id", monitorID, "error", err)
	}

	dayMap := make(map[string]*storage.DailyUptime)
	for _, d := range daily {
		dayMap[d.Date] = d
	}

	bars := make([]dailyBar, 0, 90)
	for i := 89; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		dateStr := day.Format("2006-01-02")
		label := day.Format("Jan 2, 2006")
		if d, ok := dayMap[dateStr]; ok {
			bars = append(bars, dailyBar{
				Date:      dateStr,
				UptimePct: d.UptimePct,
				HasData:   true,
				Label:     label,
			})
		} else {
			bars = append(bars, dailyBar{
				Date:    dateStr,
				HasData: false,
				Label:   label,
			})
		}
	}
	return bars
}

func (s *Server) renderStatusPage(w http.ResponseWriter, data pageData) {
	t, ok := s.templates["status_page.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
	if err := t.ExecuteTemplate(w, "status_page", data); err != nil {
		s.logger.Error("template render", "template", "status_page.html", "error", err)
	}
}

func overallStatus(monitors []*storage.Monitor) string {
	overall := "operational"
	for _, m := range monitors {
		if m.Status == "down" {
			return "major_outage"
		}
		if m.Status == "degraded" {
			overall = "degraded"
		}
	}
	return overall
}

func (s *Server) publicIncidentsForPage(ctx context.Context, sp *storage.StatusPage, monitors []*storage.Monitor, now time.Time) []*storage.Incident {
	if !sp.ShowIncidents {
		return []*storage.Incident{}
	}

	monitorIDs := make(map[int64]bool, len(monitors))
	for _, m := range monitors {
		monitorIDs[m.ID] = true
	}

	incResult, err := s.store.ListIncidents(ctx, 0, "", "", storage.Pagination{Page: 1, PerPage: 20})
	if err != nil || incResult == nil {
		return []*storage.Incident{}
	}

	all, ok := incResult.Data.([]*storage.Incident)
	if !ok {
		return []*storage.Incident{}
	}

	cutoff := now.AddDate(0, 0, -7)
	var filtered []*storage.Incident
	for _, inc := range all {
		if !monitorIDs[inc.MonitorID] {
			continue
		}
		if inc.Status == incident.StatusResolved && inc.ResolvedAt != nil && inc.ResolvedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, inc)
		if len(filtered) >= 10 {
			break
		}
	}
	if filtered == nil {
		filtered = []*storage.Incident{}
	}
	return filtered
}

func formatPct(pct float64) string {
	if pct >= 99.995 {
		return "100%"
	}
	return fmt.Sprintf("%.2f%%", pct)
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

var reservedSlugs = map[string]bool{
	"login": true, "logout": true, "monitors": true, "incidents": true,
	"notifications": true, "maintenance": true, "logs": true,
	"status-settings": true, "status-pages": true, "static": true, "api": true,
	"groups": true,
}

func validateSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return "status"
	}
	if len(slug) > 50 {
		slug = slug[:50]
	}
	if !slugPattern.MatchString(slug) {
		return "status"
	}
	if reservedSlugs[slug] {
		return "status"
	}
	return slug
}

var safeCSSProperties = map[string]bool{
	"color": true, "background": true, "background-color": true,
	"border": true, "border-color": true, "border-style": true,
	"border-width": true, "border-radius": true,
	"border-top": true, "border-right": true, "border-bottom": true, "border-left": true,
	"outline": true, "outline-color": true, "outline-style": true, "outline-width": true,
	"margin": true, "margin-top": true, "margin-right": true, "margin-bottom": true, "margin-left": true,
	"padding": true, "padding-top": true, "padding-right": true, "padding-bottom": true, "padding-left": true,
	"width": true, "height": true, "max-width": true, "max-height": true, "min-width": true, "min-height": true,
	"font-size": true, "font-weight": true, "font-family": true, "font-style": true,
	"text-align": true, "text-decoration": true, "text-transform": true,
	"line-height": true, "letter-spacing": true, "word-spacing": true,
	"display": true, "flex": true, "flex-direction": true, "flex-wrap": true,
	"justify-content": true, "align-items": true, "align-self": true, "gap": true,
	"grid-template-columns": true, "grid-template-rows": true, "grid-gap": true,
	"opacity": true, "visibility": true, "overflow": true, "overflow-x": true, "overflow-y": true,
	"position": true, "top": true, "right": true, "bottom": true, "left": true, "z-index": true,
	"box-shadow": true, "text-shadow": true,
	"transition": true, "transform": true,
	"cursor": true, "white-space": true, "word-break": true, "word-wrap": true,
	"list-style": true, "list-style-type": true,
	"vertical-align": true, "text-overflow": true,
	"content": true, "box-sizing": true, "float": true, "clear": true,
}

var dangerousValuePattern = regexp.MustCompile(`(?i)(javascript|vbscript|expression\s*\(|behavior\s*:|@import|@charset|-moz-binding|url\s*\(|data\s*:)`)

var cssCommentPattern = regexp.MustCompile(`/\*[\s\S]*?\*/`)

func sanitizeCSS(css string) string {
	if len(css) > 10000 {
		css = css[:10000]
	}

	css = strings.ReplaceAll(css, "<", "")
	css = strings.ReplaceAll(css, ">", "")
	css = strings.ReplaceAll(css, "\\", "")
	css = cssCommentPattern.ReplaceAllString(css, "")

	var result strings.Builder
	rules := splitCSSRules(css)

	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}

		if strings.HasPrefix(rule, "@") {
			continue
		}

		braceIdx := strings.Index(rule, "{")
		if braceIdx == -1 {
			continue
		}

		selector := strings.TrimSpace(rule[:braceIdx])
		body := strings.TrimSpace(rule[braceIdx+1:])
		body = strings.TrimSuffix(body, "}")
		body = strings.TrimSpace(body)

		if selector == "" || body == "" {
			continue
		}

		sanitized := sanitizeCSSDeclarations(body)
		if sanitized == "" {
			continue
		}

		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString(selector)
		result.WriteString(" { ")
		result.WriteString(sanitized)
		result.WriteString(" }")
	}

	return result.String()
}

func splitCSSRules(css string) []string {
	var rules []string
	depth := 0
	start := 0
	for i, ch := range css {
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				rules = append(rules, css[start:i+1])
				start = i + 1
			}
		}
	}
	return rules
}

func sanitizeCSSDeclarations(body string) string {
	declarations := strings.Split(body, ";")
	var safe []string

	for _, decl := range declarations {
		decl = strings.TrimSpace(decl)
		if decl == "" {
			continue
		}

		parts := strings.SplitN(decl, ":", 2)
		if len(parts) != 2 {
			continue
		}

		prop := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])

		if !safeCSSProperties[prop] {
			continue
		}

		if dangerousValuePattern.MatchString(value) {
			continue
		}

		safe = append(safe, prop+": "+value)
	}

	return strings.Join(safe, "; ")
}
