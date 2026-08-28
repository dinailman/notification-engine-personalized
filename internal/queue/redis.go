package queue

import (
	"context"
	"fmt"
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

// Allow reports whether key may make one more request in the current minute. key identifies
// the client, so it must not carry anything that varies per connection -- see clientIP in the
// handlers package.
//
// The minute is baked into the Redis key rather than tracked by a TTL on a fixed key. That
// makes setting the expiry idempotent: the counter and its expiry are pipelined into one
// MULTI/EXEC, so there is no window where an INCR has landed but its TTL has not and the key
// outlives its window forever. Re-setting the expiry on every request is safe only because
// the bucket rotates -- on a fixed key it would hold the window open for as long as a client
// kept knocking.
func (q *Queue) Allow(ctx context.Context, key string, limit int) (bool, error) {
	bucket := fmt.Sprintf("notification-engine:rate:%s:%d", key, time.Now().Unix()/60)
	pipe := q.client.TxPipeline()
	count := pipe.Incr(ctx, bucket)
	pipe.Expire(ctx, bucket, 2*time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return count.Val() <= int64(limit), nil
}
