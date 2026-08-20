package queue

import (
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

type Queue struct {
	client *redis.Client
	name   string
}

func New(addr, password string, db int, name string) *Queue {
	return &Queue{client: redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db}), name: name}
}

func (q *Queue) Close() error                   { return q.client.Close() }
func (q *Queue) Ping(ctx context.Context) error { return q.client.Ping(ctx).Err() }
func (q *Queue) Enqueue(ctx context.Context, id string) error {
	return q.client.RPush(ctx, q.name, id).Err()
}
func (q *Queue) Depth(ctx context.Context) (int64, error) { return q.client.LLen(ctx, q.name).Result() }
func (q *Queue) Dequeue(ctx context.Context) (string, error) {
	values, err := q.client.BLPop(ctx, time.Second, q.name).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(values) < 2 {
		return "", nil
	}
	return values[1], nil
}

func (q *Queue) Allow(ctx context.Context, key string, limit int) (bool, error) {
	key = "notification-engine:rate:" + key
	count, err := q.client.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		_ = q.client.Expire(ctx, key, time.Minute).Err()
	}
	return count <= int64(limit), nil
}
