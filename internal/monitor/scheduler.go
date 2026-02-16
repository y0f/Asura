package monitor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/asura-monitor/asura/internal/storage"
)

// Scheduler dispatches check jobs on a per-monitor interval basis.
type Scheduler struct {
	store    storage.Store
	jobs     chan<- Job
	logger   *slog.Logger
	mu       sync.RWMutex
	monitors []*storage.Monitor
	nextRun  map[int64]time.Time
	reload   chan struct{}
}

func NewScheduler(store storage.Store, jobs chan<- Job, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		store:   store,
		jobs:    jobs,
		logger:  logger,
		nextRun: make(map[int64]time.Time),
		reload:  make(chan struct{}, 1),
	}
}

// TriggerReload signals the scheduler to reload monitors.
func (s *Scheduler) TriggerReload() {
	select {
	case s.reload <- struct{}{}:
	default:
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	s.loadMonitors(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.reload:
			s.loadMonitors(ctx)
		case now := <-ticker.C:
			s.dispatch(now)
		}
	}
}

func (s *Scheduler) loadMonitors(ctx context.Context) {
	monitors, err := s.store.GetAllEnabledMonitors(ctx)
	if err != nil {
		s.logger.Error("scheduler: load monitors", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.monitors = monitors

	activeIDs := make(map[int64]bool, len(monitors))
	for _, m := range monitors {
		activeIDs[m.ID] = true
		if _, exists := s.nextRun[m.ID]; !exists {
			s.nextRun[m.ID] = time.Now()
		}
	}

	for id := range s.nextRun {
		if !activeIDs[id] {
			delete(s.nextRun, id)
		}
	}

	s.logger.Debug("scheduler: loaded monitors", "count", len(monitors))
}

func (s *Scheduler) dispatch(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, m := range s.monitors {
		if m.Type == "heartbeat" {
			continue
		}

		next, exists := s.nextRun[m.ID]
		if !exists {
			s.nextRun[m.ID] = now
			next = now
		}

		if now.Before(next) {
			continue
		}

		select {
		case s.jobs <- Job{Monitor: m}:
			s.nextRun[m.ID] = now.Add(time.Duration(m.Interval) * time.Second)
		default:
			s.logger.Warn("scheduler: job channel full, skipping", "monitor_id", m.ID)
		}
	}
}
