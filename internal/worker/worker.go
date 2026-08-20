package worker

import (
	"context"
	"github.com/dinailman/personalized-notification-engine/internal/models"
	"github.com/dinailman/personalized-notification-engine/internal/queue"
	"github.com/dinailman/personalized-notification-engine/internal/repositories"
	"github.com/dinailman/personalized-notification-engine/internal/sender"
	"log/slog"
	"sync"
	"time"
)

type Worker struct {
	Repo   *repositories.Repository
	Queue  *queue.Queue
	Sender sender.Sender
	Logger *slog.Logger
	Count  int
}

func (w *Worker) Run(ctx context.Context) {
	if w.Count < 1 {
		w.Count = 1
	}
	var wg sync.WaitGroup
	go w.recover(ctx)
	for i := 0; i < w.Count; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); w.loop(ctx) }()
	}
	<-ctx.Done()
	wg.Wait()
}

func (w *Worker) recover(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	w.requeuePending(ctx)
	for {
		select {
		case <-ticker.C:
			w.requeuePending(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (w *Worker) requeuePending(ctx context.Context) {
	ids, err := w.Repo.RecoverPending(ctx, time.Now().UTC())
	if err != nil {
		w.Logger.Error("recover pending notifications failed", "error", err)
		return
	}
	for _, id := range ids {
		if err := w.Queue.Enqueue(ctx, id); err != nil {
			w.Logger.Error("recover notification enqueue failed", "error", err, "notification_id", id)
		}
	}
}

func (w *Worker) loop(ctx context.Context) {
	for ctx.Err() == nil {
		id, err := w.Queue.Dequeue(ctx)
		if err != nil {
			w.Logger.Error("queue dequeue failed", "error", err)
			time.Sleep(time.Second)
			continue
		}
		if id == "" {
			continue
		}
		w.process(ctx, id)
	}
}

func (w *Worker) process(ctx context.Context, id string) {
	n, claimed, err := w.Repo.ClaimNotification(ctx, id)
	if err != nil {
		return
	}
	if !claimed || n.Status == models.NotificationSent {
		return
	}
	attempt := n.AttemptCount + 1
	provider, err := w.Sender.Send(ctx, n)
	if err == nil {
		if e := w.Repo.MarkSent(ctx, id, attempt, provider); e != nil {
			w.Logger.Error("mark notification sent failed", "error", e, "notification_id", id)
		}
		return
	}
	retry := attempt < 3
	next := time.Now().Add(time.Duration(1<<uint(attempt-1)) * time.Second)
	if e := w.Repo.MarkFailed(ctx, id, attempt, err.Error(), retry, next); e != nil {
		w.Logger.Error("mark notification failed", "error", e, "notification_id", id)
	}
	if retry {
		timer := time.NewTimer(time.Until(next))
		defer timer.Stop()
		select {
		case <-timer.C:
			_ = w.Queue.Enqueue(ctx, id)
		case <-ctx.Done():
		}
	}
}
