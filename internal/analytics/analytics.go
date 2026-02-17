package analytics

import (
	"context"
	"time"

	"github.com/asura-monitor/asura/internal/storage"
)

// MonitorMetrics holds computed metrics for a monitor.
type MonitorMetrics struct {
	MonitorID      int64   `json:"monitor_id"`
	UptimePct      float64 `json:"uptime_pct"`
	P50            float64 `json:"p50"`
	P95            float64 `json:"p95"`
	P99            float64 `json:"p99"`
	TotalChecks    int64   `json:"total_checks"`
	UpChecks       int64   `json:"up_checks"`
	DownChecks     int64   `json:"down_checks"`
	DegradedChecks int64   `json:"degraded_checks"`
}

// ComputeMetrics calculates metrics for a monitor over a time range.
func ComputeMetrics(ctx context.Context, store storage.Store, monitorID int64, from, to time.Time) (*MonitorMetrics, error) {
	uptime, err := store.GetUptimePercent(ctx, monitorID, from, to)
	if err != nil {
		return nil, err
	}

	p50, p95, p99, err := store.GetResponseTimePercentiles(ctx, monitorID, from, to)
	if err != nil {
		return nil, err
	}

	total, up, down, degraded, err := store.GetCheckCounts(ctx, monitorID, from, to)
	if err != nil {
		return nil, err
	}

	return &MonitorMetrics{
		MonitorID:      monitorID,
		UptimePct:      uptime,
		P50:            p50,
		P95:            p95,
		P99:            p99,
		TotalChecks:    total,
		UpChecks:       up,
		DownChecks:     down,
		DegradedChecks: degraded,
	}, nil
}
