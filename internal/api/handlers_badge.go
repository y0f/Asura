package api

import (
	"database/sql"
	"errors"
	"fmt"
	"html"
	"net/http"
	"time"
)

func (s *Server) handleBadgeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeBadgeSVG(w, "status", "error", colorGrey)
		return
	}

	m, err := s.store.GetMonitor(r.Context(), id)
	if err != nil || !m.Public {
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !m.Public) {
			writeBadgeSVG(w, "status", "not found", colorGrey)
			return
		}
		writeBadgeSVG(w, "status", "error", colorGrey)
		return
	}

	status := m.Status
	color := statusColor(status)
	writeBadgeSVG(w, m.Name, status, color)
}

func (s *Server) handleBadgeUptime(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeBadgeSVG(w, "uptime", "error", colorGrey)
		return
	}

	m, err := s.store.GetMonitor(r.Context(), id)
	if err != nil || !m.Public {
		writeBadgeSVG(w, "uptime", "not found", colorGrey)
		return
	}

	now := time.Now().UTC()
	from := now.Add(-30 * 24 * time.Hour)
	pct, err := s.store.GetUptimePercent(r.Context(), id, from, now)
	if err != nil {
		writeBadgeSVG(w, "uptime", "error", colorGrey)
		return
	}

	value := fmt.Sprintf("%.2f%%", pct)
	color := colorGreen
	if pct < 99 {
		color = colorYellow
	}
	if pct < 95 {
		color = colorRed
	}
	writeBadgeSVG(w, "uptime", value, color)
}

func (s *Server) handleBadgeResponseTime(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeBadgeSVG(w, "response", "error", colorGrey)
		return
	}

	m, err := s.store.GetMonitor(r.Context(), id)
	if err != nil || !m.Public {
		writeBadgeSVG(w, "response", "not found", colorGrey)
		return
	}

	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	p50, _, _, err := s.store.GetResponseTimePercentiles(r.Context(), id, from, now)
	if err != nil {
		writeBadgeSVG(w, "response", "error", colorGrey)
		return
	}

	value := fmt.Sprintf("%.0fms", p50)
	color := colorGreen
	if p50 > 500 {
		color = colorYellow
	}
	if p50 > 2000 {
		color = colorRed
	}
	writeBadgeSVG(w, "response time", value, color)
}

const (
	colorGreen  = "#4c1"
	colorYellow = "#dfb317"
	colorRed    = "#e05d44"
	colorGrey   = "#9f9f9f"
)

func statusColor(status string) string {
	switch status {
	case "up":
		return colorGreen
	case "degraded":
		return colorYellow
	case "down":
		return colorRed
	default:
		return colorGrey
	}
}

func writeBadgeSVG(w http.ResponseWriter, label, value, color string) {
	// Escape user-controlled values to prevent SVG/XSS injection
	label = html.EscapeString(label)
	value = html.EscapeString(value)

	labelWidth := len(label)*7 + 10
	valueWidth := len(value)*7 + 10
	totalWidth := labelWidth + valueWidth

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "max-age=300")

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20">
  <linearGradient id="b" x2="0" y2="100%%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="a">
    <rect width="%d" height="20" rx="3" fill="#fff"/>
  </clipPath>
  <g clip-path="url(#a)">
    <rect width="%d" height="20" fill="#555"/>
    <rect x="%d" width="%d" height="20" fill="%s"/>
    <rect width="%d" height="20" fill="url(#b)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
  </g>
</svg>`,
		totalWidth,
		totalWidth,
		labelWidth, labelWidth, valueWidth, color,
		totalWidth,
		labelWidth/2, label,
		labelWidth/2, label,
		labelWidth+valueWidth/2, value,
		labelWidth+valueWidth/2, value,
	)

	w.Write([]byte(svg))
}
