package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dinailman/personalized-notification-engine/internal/models"
	"github.com/dinailman/personalized-notification-engine/internal/repositories"
)

// jakarta is UTC+7 year round, so a local wall clock built here never lands on a DST
// transition and the tests below stay deterministic whenever they run.
const jakarta = "Asia/Jakarta"

// TestQuietHoursRoundTrip covers storage: a window survives a write and read as the same
// "15:04" strings, and an update that carries none clears it.
func TestQuietHoursRoundTrip(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	user := newQuietUser(t, r, jakarta, "22:00", "07:00")
	stored, err := r.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if stored.QuietHoursStart != "22:00" || stored.QuietHoursEnd != "07:00" {
		t.Fatalf("quiet hours = %q-%q, want 22:00-07:00", stored.QuietHoursStart, stored.QuietHoursEnd)
	}

	cleared, err := r.UpdateUser(ctx, user.ID, models.User{Name: stored.Name, Timezone: jakarta, Active: true})
	if err != nil {
		t.Fatalf("clear quiet hours: %v", err)
	}
	if cleared.QuietHoursStart != "" || cleared.QuietHoursEnd != "" {
		t.Fatalf("quiet hours after clearing = %q-%q, want empty", cleared.QuietHoursStart, cleared.QuietHoursEnd)
	}
}

// TestQuietHoursRejectsHalfWindow covers the both-or-neither rule: half a window is
// refused before it reaches the database, where the CHECK constraint would reject it too.
func TestQuietHoursRejectsHalfWindow(t *testing.T) {
	r := repo(t)
	_, err := r.CreateUser(context.Background(), models.User{
		Email:           t.Name() + "@example.com",
		Name:            "Half Window",
		Timezone:        jakarta,
		Active:          true,
		QuietHoursStart: "22:00",
	}, nil)
	if !errors.Is(err, repositories.ErrInvalidQuietHours) {
		t.Fatalf("half a quiet window was accepted: err = %v", err)
	}
}

// TestEventNotificationDefersUntilQuietHoursEnd is the core of quiet hours on the event
// path: a notification raised inside the window is created, but held until the window
// closes rather than queued immediately.
func TestEventNotificationDefersUntilQuietHoursEnd(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	// CreateEvent reads the wall clock, so the window is built around the current local
	// time: it opened an hour ago and closes two hours from now.
	loc, err := time.LoadLocation(jakarta)
	if err != nil {
		t.Fatalf("load %s: %v", jakarta, err)
	}
	local := time.Now().In(loc)
	closes := local.Add(2 * time.Hour).Truncate(time.Minute)
	user := newQuietUser(t, r, jakarta, local.Add(-time.Hour).Format("15:04"), closes.Format("15:04"))
	newEventRule(t, r, user.ID, "task_completed")

	_, created, err := r.CreateEvent(ctx, models.Event{UserID: user.ID, EventType: "task_completed", OccurredAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("created %d notifications, want 1", len(created))
	}
	if !created[0].Deferred {
		t.Fatal("a notification raised inside the quiet window was not deferred")
	}
	if got := scheduledAt(t, r, created[0]); !got.Equal(closes.UTC()) {
		t.Fatalf("scheduled at %s, want the end of the quiet window %s", got, closes.UTC())
	}
}

// TestEventNotificationOutsideQuietHoursIsImmediate is the other half: a user with a
// window that is closed right now is not delayed at all.
func TestEventNotificationOutsideQuietHoursIsImmediate(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	loc, err := time.LoadLocation(jakarta)
	if err != nil {
		t.Fatalf("load %s: %v", jakarta, err)
	}
	local := time.Now().In(loc)
	// A window that opens in an hour and closes in two does not contain now.
	user := newQuietUser(t, r, jakarta, local.Add(time.Hour).Format("15:04"), local.Add(2*time.Hour).Format("15:04"))
	newEventRule(t, r, user.ID, "task_completed")

	before := time.Now().UTC()
	_, created, err := r.CreateEvent(ctx, models.Event{UserID: user.ID, EventType: "task_completed", OccurredAt: before})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("created %d notifications, want 1", len(created))
	}
	if created[0].Deferred {
		t.Fatal("a notification raised outside the quiet window was deferred")
	}
	if got := scheduledAt(t, r, created[0]); got.Before(before) || got.After(time.Now().UTC()) {
		t.Fatalf("scheduled at %s, want an instant inside the call", got)
	}
}

// TestScheduledRuleDefersUntilQuietHoursEnd covers a scheduled rule firing inside an
// overnight window: it still belongs to the local day it fired on, so the once-per-day
// guard holds, but delivery waits for the morning.
func TestScheduledRuleDefersUntilQuietHoursEnd(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	user := newQuietUser(t, r, jakarta, "22:00", "07:00")
	newScheduledRule(t, r, user.ID, "23:00")

	// 23:00 in Jakarta on 2026-08-20 is 16:00 UTC, inside the 22:00-07:00 window.
	at := time.Date(2026, 8, 20, 16, 0, 0, 0, time.UTC)
	created, err := r.CreateScheduled(ctx, at.Add(-tick), at)
	if err != nil {
		t.Fatalf("quiet hours tick: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("created %d notifications, want 1", len(created))
	}
	if !created[0].Deferred {
		t.Fatal("a rule firing inside the quiet window was not deferred")
	}
	// 07:00 on 2026-08-21 in Jakarta is 00:00 UTC that day.
	if want, got := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC), scheduledAt(t, r, created[0]); !got.Equal(want) {
		t.Fatalf("scheduled at %s, want %s", got, want)
	}

	// The deferral must not shift which local day the rule fired on, or the same rule
	// would fire again on a later tick within that day.
	second, err := r.CreateScheduled(ctx, at.Add(-tick), at)
	if err != nil {
		t.Fatalf("replayed quiet hours tick: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("replayed tick created %d notifications, want 0", len(second))
	}
}
