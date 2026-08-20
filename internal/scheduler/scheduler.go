package scheduler

import (
	"context"
	"github.com/dinailman/personalized-notification-engine/internal/queue"
	"github.com/dinailman/personalized-notification-engine/internal/repositories"
	"log/slog"
	"time"
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
	s.tick(ctx)
	for {
		select {
		case <-ticker.C:
			s.tick(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	ids, err := s.Repo.CreateScheduled(ctx, time.Now().UTC())
	if err != nil {
		s.Logger.Error("schedule notifications failed", "error", err)
		return
	}
	for _, id := range ids {
		if err := s.Queue.Enqueue(ctx, id); err != nil {
			s.Logger.Error("enqueue scheduled notification failed", "error", err, "notification_id", id)
		}
	}
	if len(ids) > 0 {
		s.Logger.Info("scheduled notifications", "count", len(ids))
	}
}
