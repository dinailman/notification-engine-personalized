package main

import (
	"context"
	"github.com/dinailman/personalized-notification-engine/internal/config"
	"github.com/dinailman/personalized-notification-engine/internal/database"
	"github.com/dinailman/personalized-notification-engine/internal/queue"
	"github.com/dinailman/personalized-notification-engine/internal/repositories"
	"github.com/dinailman/personalized-notification-engine/internal/scheduler"
	"github.com/dinailman/personalized-notification-engine/internal/sender"
	"github.com/dinailman/personalized-notification-engine/internal/worker"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	q := queue.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer q.Close()
	w := &worker.Worker{Repo: &repositories.Repository{DB: db}, Queue: q, Sender: sender.Mock{Logger: logger}, Logger: logger, Count: cfg.WorkerCount}
	sched := &scheduler.Scheduler{Repo: &repositories.Repository{DB: db}, Queue: q, Every: cfg.PollInterval, Logger: logger}
	go sched.Run(ctx)
	w.Run(ctx)
}
