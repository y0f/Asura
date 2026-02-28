package api

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"time"

	"github.com/y0f/asura/internal/httputil"
)

const (
	colorGreen  = "#4c1"
	colorYellow = "#dfb317"
	colorRed    = "#e05d44"
	colorGrey   = "#9f9f9f"
)

func (h *Handler) BadgeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeBadgeSVG(w, "status", "error", colorGrey)
		return
	}

	ctx := r.Context()
	m, err := h.store.GetMonitor(ctx, id)
	if err != nil {
		writeBadgeSVG(w, "status", "not found", colorGrey)
		return
	}

	visible, err := h.store.IsMonitorOnStatusPage(ctx, m.ID)
	if err != nil || !visible {
		writeBadgeSVG(w, "status", "not found", colorGrey)
		return
	}

	label := r.URL.Query().Get("label")
	if label == "" {
		label = m.Name
	}
	status := m.Status
	color := statusColor(status)
	writeBadgeSVG(w, label, status, color)
}

func (h *Handler) BadgeUptime(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeBadgeSVG(w, "uptime", "error", colorGrey)
		return
	}

	ctx := r.Context()
	m, err := h.store.GetMonitor(ctx, id)
	if err != nil {
		writeBadgeSVG(w, "uptime", "not found", colorGrey)
		return
	}

	visible, err := h.store.IsMonitorOnStatusPage(ctx, m.ID)
	if err != nil || !visible {
		writeBadgeSVG(w, "uptime", "not found", colorGrey)
		return
	}

	now := time.Now().UTC()
	from := now.Add(-30 * 24 * time.Hour)
	pct, err := h.store.GetUptimePercent(ctx, id, from, now)
	if err != nil {
		writeBadgeSVG(w, "uptime", "error", colorGrey)
		return
	}

	label := r.URL.Query().Get("label")
	if label == "" {
		label = "uptime"
	}
	value := fmt.Sprintf("%.2f%%", pct)
	color := colorGreen
	if pct < 99 {
		color = colorYellow
	}
	if pct < 95 {
		color = colorRed
	}
	writeBadgeSVG(w, label, value, color)
}

func (h *Handler) BadgeResponseTime(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeBadgeSVG(w, "response", "error", colorGrey)
		return
	}

	ctx := r.Context()
	m, err := h.store.GetMonitor(ctx, id)
	if err != nil {
		writeBadgeSVG(w, "response", "not found", colorGrey)
		return
	}

	visible, err := h.store.IsMonitorOnStatusPage(ctx, m.ID)
	if err != nil || !visible {
		writeBadgeSVG(w, "response", "not found", colorGrey)
		return
	}

	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	p50, _, _, err := h.store.GetResponseTimePercentiles(ctx, id, from, now)
	if err != nil {
		writeBadgeSVG(w, "response", "error", colorGrey)
		return
	}

	label := r.URL.Query().Get("label")
	if label == "" {
		label = "response time"
	}
	value := fmt.Sprintf("%.0fms", p50)
	color := colorGreen
	if p50 > 500 {
		color = colorYellow
	}
	if p50 > 2000 {
		color = colorRed
	}
	writeBadgeSVG(w, label, value, color)
}

func (h *Handler) BadgeCert(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		writeBadgeSVG(w, "cert", "error", colorGrey)
		return
	}

	ctx := r.Context()
	m, err := h.store.GetMonitor(ctx, id)
	if err != nil {
		writeBadgeSVG(w, "cert", "not found", colorGrey)
		return
	}

	visible, err := h.store.IsMonitorOnStatusPage(ctx, m.ID)
	if err != nil || !visible {
		writeBadgeSVG(w, "cert", "not found", colorGrey)
		return
	}

	cr, err := h.store.GetLatestCheckResult(ctx, id)
	if err != nil || cr == nil || cr.CertExpiry == nil {
		writeBadgeSVG(w, "cert", "n/a", colorGrey)
		return
	}

	label := r.URL.Query().Get("label")
	if label == "" {
		label = "cert expiry"
	}
	days := int(time.Until(*cr.CertExpiry).Hours() / 24)
	value := fmt.Sprintf("%dd", days)
	color := colorGreen
	if days < 30 {
		color = colorYellow
	}
	if days < 7 {
		color = colorRed
	}
	writeBadgeSVG(w, label, value, color)
}

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

	io.WriteString(w, svg)
}
