package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dinailman/personalized-notification-engine/internal/models"
	"github.com/dinailman/personalized-notification-engine/internal/repositories"
)

// concurrentIngests is how many callers race to ingest the same event below.
const concurrentIngests = 50

// TestConcurrentIngestCreatesOneNotification is the measured form of the idempotency
// claim: fifty callers racing to ingest one external_id must leave exactly one event and
// exactly one notification behind, and exactly one of them may report having created it.
//
// The guard is the unique index on events(external_id) combined with ON CONFLICT DO
// NOTHING: a loser's insert blocks until the winner commits, then finds no returned row
// and resolves the existing event instead of matching rules a second time.
func TestConcurrentIngestCreatesOneNotification(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	user := newUser(t, r, "UTC")
	newEventRule(t, r, user.ID, "task_completed")
	external := "task-" + time.Now().Format("20060102150405.000000000")

	created := make([][]repositories.Created, concurrentIngests)
	events := make([]string, concurrentIngests)
	errs := make([]error, concurrentIngests)

	// Every goroutine blocks on the same channel so the ingests genuinely overlap rather
	// than queueing behind each other's start-up.
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < concurrentIngests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			event, ids, err := r.CreateEvent(ctx, models.Event{
				UserID:     user.ID,
				EventType:  "task_completed",
				ExternalID: external,
				Payload:    map[string]any{"items_completed": 3},
				OccurredAt: time.Now().UTC(),
			})
			created[i], events[i], errs[i] = ids, event.ID, err
		}(i)
	}
	close(start)
	wg.Wait()

	reported := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("ingest %d failed: %v", i, err)
		}
		reported += len(created[i])
		if events[i] != events[0] {
			t.Fatalf("ingest %d resolved to event %s, want the single event %s", i, events[i], events[0])
		}
	}
	if reported != 1 {
		t.Fatalf("%d ingests reported %d notifications created, want 1", concurrentIngests, reported)
	}

	notifications, err := r.ListNotifications(ctx, user.ID)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("user holds %d notifications, want 1", len(notifications))
	}

	var stored int
	if err := r.DB.QueryRow(ctx, `SELECT count(*) FROM events WHERE external_id=$1`, external).Scan(&stored); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if stored != 1 {
		t.Fatalf("%d event rows stored for external_id %q, want 1", stored, external)
	}
}
