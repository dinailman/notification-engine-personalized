package integration

import (
	"context"
	"testing"
	"time"
)

// tick is the scheduler's evaluation interval; windows below mirror one real tick.
const tick = 10 * time.Second

// TestScheduledRuleFiresInUserLocalTime is the core of timezone-aware scheduling: two
// users in different timezones hold the same 20:00 rule and must fire at different UTC
// instants, seven hours apart.
func TestScheduledRuleFiresInUserLocalTime(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	jakarta := newUser(t, r, "Asia/Jakarta")     // UTC+7 year round
	newYork := newUser(t, r, "America/New_York") // UTC-4 in August
	newScheduledRule(t, r, jakarta.ID, "20:00")
	newScheduledRule(t, r, newYork.ID, "20:00")

	// 20:00 in Jakarta on 2026-08-20 is 13:00 UTC.
	at := time.Date(2026, 8, 20, 13, 0, 0, 0, time.UTC)
	ids, err := r.CreateScheduled(ctx, at.Add(-tick), at)
	if err != nil {
		t.Fatalf("create scheduled at Jakarta 20:00: %v", err)
	}
	if owners := notificationOwners(t, r, ids); len(owners) != 1 || owners[0] != jakarta.ID {
		t.Fatalf("Jakarta 20:00 fired for %v, want exactly [%s]", owners, jakarta.ID)
	}

	// 20:00 in New York on the same day is 00:00 UTC the next day.
	at = time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	ids, err = r.CreateScheduled(ctx, at.Add(-tick), at)
	if err != nil {
		t.Fatalf("create scheduled at New York 20:00: %v", err)
	}
	if owners := notificationOwners(t, r, ids); len(owners) != 1 || owners[0] != newYork.ID {
		t.Fatalf("New York 20:00 fired for %v, want exactly [%s]", owners, newYork.ID)
	}
}

// TestScheduledRuleFiresOncePerLocalDay covers the occurrence_date guard: repeated ticks
// across the same local day must not produce a second notification.
func TestScheduledRuleFiresOncePerLocalDay(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	user := newUser(t, r, "Asia/Jakarta")
	newScheduledRule(t, r, user.ID, "20:00")

	at := time.Date(2026, 8, 20, 13, 0, 0, 0, time.UTC)
	first, err := r.CreateScheduled(ctx, at.Add(-tick), at)
	if err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first tick created %d notifications, want 1", len(first))
	}
	// A window covering the same local instant again -- as happens when a tick is
	// replayed -- must be suppressed by the local-date unique index.
	second, err := r.CreateScheduled(ctx, at.Add(-tick), at)
	if err != nil {
		t.Fatalf("replayed tick: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("replayed tick created %d notifications, want 0", len(second))
	}
}

// TestScheduledRuleCrossesLocalMidnight covers a window that spans local midnight, where
// the firing instant sits on the later of the two local dates.
func TestScheduledRuleCrossesLocalMidnight(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	user := newUser(t, r, "Asia/Jakarta")
	newScheduledRule(t, r, user.ID, "00:00")

	// 00:00 on 2026-08-21 in Jakarta is 17:00 UTC on 2026-08-20; the window straddles
	// local midnight, so the candidate on the window's start date must be ignored.
	at := time.Date(2026, 8, 20, 17, 0, 0, 0, time.UTC)
	ids, err := r.CreateScheduled(ctx, at.Add(-tick), at)
	if err != nil {
		t.Fatalf("midnight tick: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("midnight tick created %d notifications, want 1", len(ids))
	}
}

// TestSpringForwardFiresAtTheJump documents the nonexistent-hour choice: 02:30 does not
// occur on 2026-03-08 in New York, and the rule fires at the moment the clock jumps
// rather than being skipped for the day.
func TestSpringForwardFiresAtTheJump(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	user := newUser(t, r, "America/New_York")
	newScheduledRule(t, r, user.ID, "02:30")

	// 06:59:55 UTC is 01:59:55 EST; 07:00:05 UTC is 03:00:05 EDT. Local time never
	// passes through 02:30, but the window contains it.
	since := time.Date(2026, 3, 8, 6, 59, 55, 0, time.UTC)
	now := time.Date(2026, 3, 8, 7, 0, 5, 0, time.UTC)
	ids, err := r.CreateScheduled(ctx, since, now)
	if err != nil {
		t.Fatalf("spring forward tick: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("spring forward created %d notifications, want 1 at the jump", len(ids))
	}
}

// TestFallBackFiresOnlyOnce documents the ambiguous-hour choice: 01:30 happens twice on
// 2026-11-01 in New York, and only the first pass produces a notification.
func TestFallBackFiresOnlyOnce(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	user := newUser(t, r, "America/New_York")
	newScheduledRule(t, r, user.ID, "01:30")

	// First pass: 05:30 UTC is 01:30 EDT.
	at := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	first, err := r.CreateScheduled(ctx, at.Add(-tick), at)
	if err != nil {
		t.Fatalf("first pass through 01:30: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first pass created %d notifications, want 1", len(first))
	}

	// Second pass one hour later: 06:30 UTC is 01:30 EST, the same local wall clock on
	// the same local date, so the unique index must suppress it.
	at = time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)
	second, err := r.CreateScheduled(ctx, at.Add(-tick), at)
	if err != nil {
		t.Fatalf("second pass through 01:30: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second pass created %d notifications, want 0", len(second))
	}
}
