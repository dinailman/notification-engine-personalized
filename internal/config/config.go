package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr       string
	APIKey         string
	DatabaseURL    string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	QueueName      string
	WorkerCount    int
	PollInterval   time.Duration
	ShutdownPeriod time.Duration
	RateLimit      int
}

func Load() Config {
	return Config{
		HTTPAddr:       env("HTTP_ADDR", ":8083"),
		APIKey:         env("API_KEY", ""),
		DatabaseURL:    env("DATABASE_URL", "postgres://postgres:postgres@localhost:5435/notifications?sslmode=disable"),
		RedisAddr:      env("REDIS_ADDR", "localhost:16380"),
		RedisPassword:  env("REDIS_PASSWORD", ""),
		RedisDB:        envInt("REDIS_DB", 0),
		QueueName:      env("QUEUE_NAME", "notifications:pending"),
		WorkerCount:    envInt("WORKER_COUNT", 4),
		PollInterval:   time.Duration(envInt("SCHEDULER_INTERVAL_SECONDS", 10)) * time.Second,
		ShutdownPeriod: time.Duration(envInt("SHUTDOWN_SECONDS", 10)) * time.Second,
		RateLimit:      envInt("RATE_LIMIT_PER_MINUTE", 120),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	parsed, err := strconv.Atoi(value)
	if value == "" || err != nil {
		return fallback
	}
	return parsed
}
