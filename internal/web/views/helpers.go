package views

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/y0f/asura/internal/storage"
)

func StatusColor(status string) string {
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
}

func StatusBg(status string) string {
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
}

func StatusDot(status string) string {
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
}

func TimeAgo(t any) string {
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
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

func FormatMs(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

func IncidentDuration(started time.Time, resolved *time.Time) string {
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
}

func UptimeFmt(pct float64) string {
	if pct >= 99.995 {
		return "100%"
	}
	return fmt.Sprintf("%.2f%%", pct)
}

func UptimeColor(pct float64) string {
	if pct >= 99.9 {
		return "text-emerald-400"
	}
	if pct >= 99 {
		return "text-yellow-400"
	}
	return "text-red-400"
}

func UptimeBarColor(pct float64, hasData bool) string {
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
}

func UptimeBarTooltip(pct float64, hasData bool, label string) string {
	safe := jsSingleQuoteEscaper.Replace(label)
	if !hasData {
		return safe + " — No data"
	}
	if pct >= 99.995 {
		return safe + " — 100% uptime"
	}
	return fmt.Sprintf("%s — %.2f%% uptime", safe, pct)
}

func JSEscapeString(s string) string {
	return jsSingleQuoteEscaper.Replace(s)
}

func HttpStatusColor(code int) string {
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
}

func CertDays(t *time.Time) int {
	if t == nil {
		return -1
	}
	return int(time.Until(*t).Hours() / 24)
}

func CertColor(t *time.Time) string {
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
}

func TypeLabel(t string) string {
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
}

func FormatFloat(f float64) string {
	if f == 0 {
		return "-"
	}
	if f < 1000 {
		return fmt.Sprintf("%.0fms", f)
	}
	return fmt.Sprintf("%.1fs", f/1000)
}

var jsEscaper = strings.NewReplacer(
	"</", `<\/`,
	"<!--", `<\!--`,
)

var jsSingleQuoteEscaper = strings.NewReplacer(
	`\`, `\\`,
	`'`, `\'`,
	"\n", `\n`,
	"\r", `\r`,
	"</", `<\/`,
)

func ToJSON(v any) string {
	b, _ := json.Marshal(v)
	return jsEscaper.Replace(string(b))
}

func InSlice(needle int64, haystack []int64) bool {
	return slices.Contains(haystack, needle)
}

func ParseDNS(s string) []string {
	if s == "" {
		return nil
	}
	var records []string
	json.Unmarshal([]byte(s), &records)
	return records
}

func statusPageMonitorSort(data map[int64]storage.StatusPageMonitor, monID int64) string {
	if spm, ok := data[monID]; ok && spm.SortOrder != 0 {
		return strconv.Itoa(spm.SortOrder)
	}
	return ""
}

func statusPageMonitorGroup(data map[int64]storage.StatusPageMonitor, monID int64) string {
	if spm, ok := data[monID]; ok {
		return spm.GroupName
	}
	return ""
}
