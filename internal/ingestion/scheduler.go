package ingestion

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler automatically triggers the Job at fixed intervals. It can be
// replaced with a library like robfig/cron (if cron syntax is desired);
// for now it is kept dependency-free using the stdlib time.Ticker.
type Scheduler struct {
	job      *Job
	interval time.Duration
}

func NewScheduler(job *Job, interval time.Duration) *Scheduler {
	return &Scheduler{job: job, interval: interval}
}

// Start launches a loop that runs in the background. When ctx is
// canceled, the loop terminates cleanly (graceful shutdown).
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("ingestion scheduler stopped")
				return
			case <-ticker.C:
				s.job.RunAll(ctx)
			}
		}
	}()
}
