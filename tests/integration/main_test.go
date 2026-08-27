// Package integration exercises the repository against a real PostgreSQL instance.
//
// The tests are skipped unless TEST_DATABASE_URL is set. That database is reset before
// the suite runs -- the public schema is dropped and the migrations are re-applied -- so
// it must be a throwaway database, never a development one.
//
//	docker compose up -d postgres
//	docker compose exec -T postgres psql -U postgres -c 'CREATE DATABASE notifications_test'
//	TEST_DATABASE_URL='postgres://postgres:postgres@localhost:15435/notifications_test?sslmode=disable' \
//	  go test ./tests/integration -v
package integration

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/dinailman/personalized-notification-engine/internal/models"
	"github.com/dinailman/personalized-notification-engine/internal/repositories"
	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		os.Exit(m.Run())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		panic("connect to TEST_DATABASE_URL: " + err.Error())
	}
	if err := reset(ctx, p); err != nil {
		panic("reset test schema: " + err.Error())
	}
	pool = p
	code := m.Run()
	p.Close()
	os.Exit(code)
}

// reset drops the public schema and replays every migration, giving each run a database
// in a known state without depending on psql being installed.
func reset(ctx context.Context, p *pgxpool.Pool) error {
	if _, err := p.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		statements, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err := p.Exec(ctx, string(statements)); err != nil {
			return err
		}
	}
	return nil
}

func repo(t *testing.T) *repositories.Repository {
	t.Helper()
	if pool == nil {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	return &repositories.Repository{DB: pool}
}

// newUser creates an active user in the given timezone with every channel enabled and no
// quiet window.
func newUser(t *testing.T, r *repositories.Repository, timezone string) models.User {
	t.Helper()
	return newQuietUser(t, r, timezone, "", "")
}

// newQuietUser is newUser with a quiet window, given as "15:04" local times.
func newQuietUser(t *testing.T, r *repositories.Repository, timezone, quietStart, quietEnd string) models.User {
	t.Helper()
	email := t.Name() + "-" + timezone + "-" + time.Now().Format("150405.000000000") + "@example.com"
	user, err := r.CreateUser(context.Background(), models.User{
		Email:           email,
		Name:            "Integration User",
		Timezone:        timezone,
		Active:          true,
		QuietHoursStart: quietStart,
		QuietHoursEnd:   quietEnd,
	}, []models.Preference{
		{Channel: models.ChannelEmail, Frequency: models.FrequencyDaily, Enabled: true},
		{Channel: models.ChannelInApp, Frequency: models.FrequencyDaily, Enabled: true},
	})
	if err != nil {
		t.Fatalf("create user in %s: %v", timezone, err)
	}
	return user
}

// newScheduledRule creates a daily scheduled rule firing at the given local time.
func newScheduledRule(t *testing.T, r *repositories.Repository, userID, at string) models.Rule {
	t.Helper()
	rule, err := r.CreateRule(context.Background(), models.Rule{
		UserID:          userID,
		Name:            "Daily digest reminder",
		TriggerType:     models.TriggerScheduled,
		ScheduledTime:   at,
		Frequency:       models.FrequencyDaily,
		Channel:         models.ChannelEmail,
		SubjectTemplate: "Your daily digest is ready",
		BodyTemplate:    "Here's what changed on your account today.",
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("create scheduled rule at %s: %v", at, err)
	}
	return rule
}

// newEventRule creates an event-driven rule for the given event type.
func newEventRule(t *testing.T, r *repositories.Repository, userID, eventType string) models.Rule {
	t.Helper()
	rule, err := r.CreateRule(context.Background(), models.Rule{
		UserID:          userID,
		Name:            "Task summary",
		TriggerType:     models.TriggerEvent,
		EventType:       eventType,
		Channel:         models.ChannelEmail,
		SubjectTemplate: "Your summary is ready",
		BodyTemplate:    "Your {{event_type}} summary is ready.",
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("create event rule for %s: %v", eventType, err)
	}
	return rule
}

// notificationOwners maps the given notifications back to their user IDs.
func notificationOwners(t *testing.T, r *repositories.Repository, created []repositories.Created) []string {
	t.Helper()
	owners := []string{}
	for _, c := range created {
		n, err := r.GetNotification(context.Background(), c.ID)
		if err != nil {
			t.Fatalf("load notification %s: %v", c.ID, err)
		}
		owners = append(owners, n.UserID)
	}
	return owners
}

// scheduledAt reads back when a created notification is allowed to be delivered.
func scheduledAt(t *testing.T, r *repositories.Repository, c repositories.Created) time.Time {
	t.Helper()
	n, err := r.GetNotification(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("load notification %s: %v", c.ID, err)
	}
	return n.ScheduledAt.UTC()
}
