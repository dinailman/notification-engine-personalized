package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dinailman/notification-engine-personalized/internal/queue"
	"github.com/redis/go-redis/v9"
)

// rateQueue returns a queue backed by the Redis at TEST_REDIS_ADDR, skipping when it is
// unset, the way repo() skips without TEST_DATABASE_URL.
func rateQueue(t *testing.T) (*queue.Queue, *redis.Client) {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_REDIS_ADDR is unset under CI, so these tests would skip and report success")
		}
		t.Skip("set TEST_REDIS_ADDR to run rate limiter tests")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("ping redis at %s: %v", addr, err)
	}
	t.Cleanup(func() { client.Close() })
	return queue.New(addr, "", 0, "notifications:test"), client
}

// key gives each run its own counter so repeated runs never inherit each other's budget.
func key(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
}

// TestAllowLimitsOneClient is the guarantee the middleware depends on: the first `limit`
// calls pass and the next does not.
func TestAllowLimitsOneClient(t *testing.T) {
	q, _ := rateQueue(t)
	ctx := context.Background()
	client := key(t)

	const limit = 5
	for i := 1; i <= limit; i++ {
		allowed, err := q.Allow(ctx, client, limit)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("call %d of %d was refused, want allowed", i, limit)
		}
	}
	allowed, err := q.Allow(ctx, client, limit)
	if err != nil {
		t.Fatalf("call %d: %v", limit+1, err)
	}
	if allowed {
		t.Fatalf("call %d exceeded the limit of %d but was allowed", limit+1, limit)
	}
}

// TestAllowBudgetsPerKey covers the other half: exhausting one client must not spend
// another's budget. Two connections from one IP share a key; two IPs must not.
func TestAllowBudgetsPerKey(t *testing.T) {
	q, _ := rateQueue(t)
	ctx := context.Background()
	spent, fresh := key(t)+"-spent", key(t)+"-fresh"

	const limit = 2
	for i := 0; i <= limit; i++ {
		if _, err := q.Allow(ctx, spent, limit); err != nil {
			t.Fatalf("spend %d: %v", i, err)
		}
	}
	if allowed, err := q.Allow(ctx, spent, limit); err != nil || allowed {
		t.Fatalf("exhausted key allowed=%v err=%v, want allowed=false", allowed, err)
	}
	allowed, err := q.Allow(ctx, fresh, limit)
	if err != nil {
		t.Fatalf("fresh key: %v", err)
	}
	if !allowed {
		t.Fatal("a different key was refused, so one client's traffic is spending another's budget")
	}
}

// TestAllowAlwaysSetsATTL pins the bug where INCR landed and EXPIRE did not, leaving a key
// that never expired and rate-limited that client forever.
func TestAllowAlwaysSetsATTL(t *testing.T) {
	q, client := rateQueue(t)
	ctx := context.Background()
	name := key(t)

	if _, err := q.Allow(ctx, name, 10); err != nil {
		t.Fatalf("allow: %v", err)
	}
	bucket := fmt.Sprintf("notification-engine:rate:%s:%d", name, time.Now().Unix()/60)
	ttl, err := client.TTL(ctx, bucket).Result()
	if err != nil {
		t.Fatalf("ttl of %s: %v", bucket, err)
	}
	if ttl <= 0 {
		t.Fatalf("bucket %s has ttl %v; a key with no expiry blocks its client permanently", bucket, ttl)
	}
}
