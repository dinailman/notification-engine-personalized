package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dinailman/notification-engine-personalized/internal/config"
	"github.com/dinailman/notification-engine-personalized/internal/database"
	"github.com/dinailman/notification-engine-personalized/internal/queue"
	"github.com/dinailman/notification-engine-personalized/internal/repositories"
	"github.com/dinailman/notification-engine-personalized/internal/scheduler"
	"github.com/dinailman/notification-engine-personalized/internal/sender"
	"github.com/dinailman/notification-engine-personalized/internal/worker"
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
