package monitor

import (
	"context"
	"log/slog"
	"time"

	"github.com/asura-monitor/asura/internal/checker"
	"github.com/asura-monitor/asura/internal/storage"
)

// Job represents a check to be executed.
type Job struct {
	Monitor *storage.Monitor
}

// WorkerResult holds the outcome of a check job.
type WorkerResult struct {
	Monitor *storage.Monitor
	Result  *checker.Result
	Err     error
}

// Pool manages a fixed set of worker goroutines.
type Pool struct {
	workers  int
	registry *checker.Registry
	jobs     <-chan Job
	results  chan<- WorkerResult
	logger   *slog.Logger
}

// NewPool creates a worker pool.
func NewPool(workers int, registry *checker.Registry, jobs <-chan Job, results chan<- WorkerResult, logger *slog.Logger) *Pool {
	return &Pool{
		workers:  workers,
		registry: registry,
		jobs:     jobs,
		results:  results,
		logger:   logger,
	}
}

// Run starts all workers and blocks until ctx is cancelled.
func (p *Pool) Run(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		go p.worker(ctx, i)
	}
	<-ctx.Done()
}

func (p *Pool) worker(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			p.executeJob(ctx, job)
		}
	}
}

func (p *Pool) executeJob(ctx context.Context, job Job) {
	c, err := p.registry.Get(job.Monitor.Type)
	if err != nil {
		p.results <- WorkerResult{
			Monitor: job.Monitor,
			Err:     err,
		}
		return
	}

	timeout := time.Duration(job.Monitor.Timeout) * time.Second
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := c.Check(checkCtx, job.Monitor)
	p.results <- WorkerResult{
		Monitor: job.Monitor,
		Result:  result,
		Err:     err,
	}
}
