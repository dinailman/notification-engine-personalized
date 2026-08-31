package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/dinailman/notification-engine-personalized/internal/queue"
	"github.com/dinailman/notification-engine-personalized/internal/repositories"
)

type Scheduler struct {
	Repo   *repositories.Repository
	Queue  *queue.Queue
	Every  time.Duration
	Logger *slog.Logger
}

func (s *Scheduler) Run(ctx context.Context) {
	interval := s.Every
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// Each tick claims the window since the previous one, so a rule whose local firing
	// time lands between two ticks is still picked up. The first window covers the
	// interval leading up to startup rather than starting empty.
	since := time.Now().UTC().Add(-interval)
	since = s.tick(ctx, since)
	for {
		select {
		case <-ticker.C:
			since = s.tick(ctx, since)
		case <-ctx.Done():
			return
		}
	}
}

// tick evaluates the window (since, now] and returns the point the next window starts
// from. A failed tick returns since unchanged so the window is retried rather than lost.
func (s *Scheduler) tick(ctx context.Context, since time.Time) time.Time {
	now := time.Now().UTC()
	created, err := s.Repo.CreateScheduled(ctx, since, now)
	if err != nil {
		s.Logger.Error("schedule notifications failed", "error", err)
		return since
	}
	deferred := 0
	for _, c := range created {
		// A notification held for the user's quiet window is not queued now; the
		// worker's recovery loop picks it up once the window closes.
		if c.Deferred {
			deferred++
			continue
		}
		if err := s.Queue.Enqueue(ctx, c.ID); err != nil {
			s.Logger.Error("enqueue scheduled notification failed", "error", err, "notification_id", c.ID)
		}
	}
	if len(created) > 0 {
		s.Logger.Info("scheduled notifications", "count", len(created), "deferred_for_quiet_hours", deferred)
	}
	return now
}
