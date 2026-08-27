package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dinailman/personalized-notification-engine/internal/config"
	"github.com/dinailman/personalized-notification-engine/internal/database"
	"github.com/dinailman/personalized-notification-engine/internal/handlers"
	"github.com/dinailman/personalized-notification-engine/internal/queue"
	"github.com/dinailman/personalized-notification-engine/internal/repositories"
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
	if err := q.Ping(ctx); err != nil {
		logger.Error("redis connection failed", "error", err)
		os.Exit(1)
	}
	s := &handlers.Server{Repo: &repositories.Repository{DB: db}, Queue: q, Logger: logger, RateLimit: cfg.RateLimit, APIKey: cfg.APIKey}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users", s.CreateUser)
	mux.HandleFunc("GET /users/{id}", s.GetUser)
	mux.HandleFunc("PATCH /users/{id}", s.UpdateUser)
	mux.HandleFunc("GET /users/{id}/preferences", s.Preferences)
	mux.HandleFunc("PUT /users/{id}/preferences", s.Preferences)
	mux.HandleFunc("POST /users/{id}/rules", s.CreateRule)
	mux.HandleFunc("GET /users/{id}/rules", s.ListRules)
	mux.HandleFunc("PATCH /rules/{id}", s.UpdateRule)
	mux.HandleFunc("DELETE /rules/{id}", s.DeleteRule)
	mux.HandleFunc("POST /events", s.CreateEvent)
	mux.HandleFunc("GET /users/{id}/events", s.ListEvents)
	mux.HandleFunc("GET /users/{id}/notifications", s.ListNotifications)
	mux.HandleFunc("GET /notifications/{id}", s.GetNotification)
	mux.HandleFunc("GET /notifications/{id}/logs", s.NotificationLogs)
	mux.HandleFunc("GET /analytics/notifications", s.Analytics)
	mux.HandleFunc("GET /metrics", s.Metrics)
	mux.HandleFunc("GET /openapi.json", s.OpenAPI)
	mux.HandleFunc("GET /healthz", s.Health)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: s.RateLimited(s.RequireAPIKey(mux)), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api stopped", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), cfg.ShutdownPeriod)
	defer cancel()
	_ = server.Shutdown(shutdown)
}
