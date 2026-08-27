# Personalized Notification Engine

Production-style Go notification service that personalizes user engagement messages for SaaS and marketplace products.

## Problem

Engagement products need timely, relevant reminders without spamming users. This service stores user preferences, evaluates scheduled and event-driven rules, queues notifications for delivery, records every attempt, and exposes delivery analytics.

## Architecture

```text
Client -> REST API -> PostgreSQL (source of truth)
                 -> Redis queue and rate limiter

Worker -> Redis queue -> mock sender -> notification status + attempt log
       -> scheduler -> scheduled notifications -> Redis queue
```

Redis carries notification IDs rather than full payloads. PostgreSQL owns users, preferences, rules, events, notification state, and audit logs. This keeps retries inspectable and makes duplicate event ingestion safe.

## Features

- User management with timezone and active state
- Email, push, and in-app preferences
- Daily and weekly frequencies
- Scheduled reminder rules evaluated in each user's timezone, including DST transitions
- Per-user quiet hours that hold delivery until the window closes
- Event-driven rules such as `task_completed` or `document_uploaded`
- Idempotent event ingestion using `external_id`
- Redis-backed notification queue
- Separate worker process with mock sender
- Three-attempt retry handling with exponential backoff
- Notification and attempt logs
- Redis-backed API rate limiting
- Daily, per-user, and per-channel analytics
- PostgreSQL migrations and seed data
- Health, metrics, and OpenAPI endpoints

## Run Locally

```bash
docker compose up -d --build
```

Services:

- API: `http://localhost:8083`
- PostgreSQL: `localhost:15435`
- Redis: `localhost:16380`
- OpenAPI JSON: `http://localhost:8083/openapi.json`
- Health: `http://localhost:8083/healthz`
- Metrics: `http://localhost:8083/metrics`

The Compose API uses the local development key `dev-api-key`. Send it as `X-API-Key` on application requests. Set `API_KEY` to a long secret outside local development.

Reset the demo database:

```bash
docker compose down -v
docker compose up -d --build
```

## Example Flow

Create a user:

```bash
curl -X POST http://localhost:8083/users \
  -H 'X-API-Key: dev-api-key' \
  -H 'Content-Type: application/json' \
  -d '{
    "email":"sam@example.com",
    "name":"Sam Lee",
    "timezone":"Asia/Jakarta",
    "quiet_hours_start":"22:00",
    "quiet_hours_end":"07:00",
    "preferences":[
      {"channel":"email","frequency":"daily","enabled":true},
      {"channel":"in_app","frequency":"weekly","enabled":true}
    ]
  }'
```

Quiet hours are optional `HH:MM` local times, sent as both halves or neither, and wrap past midnight.

Create an event rule:

```bash
curl -X POST http://localhost:8083/users/{user_id}/rules \
  -H 'X-API-Key: dev-api-key' \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"Task summary",
    "trigger_type":"event",
    "event_type":"task_completed",
    "channel":"in_app",
    "subject_template":"Your summary is ready",
    "body_template":"Your {{event_type}} summary is ready.",
    "enabled":true
  }'
```

Ingest activity:

```bash
curl -X POST http://localhost:8083/events \
  -H 'X-API-Key: dev-api-key' \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id":"{user_id}",
    "event_type":"task_completed",
    "external_id":"task-2026-0001",
    "payload":{"items_completed":3}
  }'
```

The API stores the event, matches enabled rules and preferences, creates pending notifications, and pushes their IDs to Redis. The worker consumes the IDs and logs mock delivery.

Ingesting one `external_id` from 50 concurrent callers leaves one event row and one notification: `TestConcurrentIngestCreatesOneNotification` in `tests/integration`.

A notification raised inside the user's quiet window is held until the window closes and returned under `deferred_notification_ids`.

Create a daily digest reminder:

```bash
curl -X POST http://localhost:8083/users/{user_id}/rules \
  -H 'X-API-Key: dev-api-key' \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"Daily digest reminder",
    "trigger_type":"scheduled",
    "scheduled_time":"20:00",
    "frequency":"daily",
    "channel":"email",
    "subject_template":"Your daily digest is ready",
    "body_template":"Here's what changed on your account today.",
    "enabled":true
  }'
```

The scheduler evaluates due rules every ten seconds. Each tick claims the window since the previous tick in each user's own timezone, so this rule fires at 20:00 local time rather than 20:00 UTC. Notifications are unique per rule per user-local day.

## API

```text
POST   /users
GET    /users/{id}
PATCH  /users/{id}
GET    /users/{id}/preferences
PUT    /users/{id}/preferences
POST   /users/{id}/rules
GET    /users/{id}/rules
PATCH  /rules/{id}
DELETE /rules/{id}
POST   /events
GET    /users/{id}/events
GET    /users/{id}/notifications
GET    /notifications/{id}
GET    /notifications/{id}/logs
GET    /analytics/notifications
GET    /healthz
GET    /metrics
GET    /openapi.json
```

Analytics example:

```bash
curl 'http://localhost:8083/analytics/notifications?from=2026-08-01&to=2026-09-01&channel=in_app' \
  -H 'X-API-Key: dev-api-key'
```

The response includes `total_sent`, `by_day`, `by_user`, and `by_channel`.

## Testing

Run local checks:

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/api
go build ./cmd/worker
docker compose config
```

Run the PostgreSQL-backed repository tests. They are skipped unless `TEST_DATABASE_URL` is set, and they drop and re-apply the schema, so point them at a throwaway database:

```bash
docker compose up -d postgres
docker compose exec -T postgres psql -U postgres -c 'CREATE DATABASE notifications_test'
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:15435/notifications_test?sslmode=disable' \
  go test -race ./tests/integration -v
```

Run the Docker-backed lifecycle test:

```bash
docker compose up -d --build
E2E_BASE_URL=http://localhost:8083 go test ./tests/e2e -v
docker compose down -v
```

The mock sender logs successful delivery. Put `fail_delivery` in a notification body to exercise retry behavior; after three attempts the notification becomes `failed`.

Workers lease notifications for sixty seconds before delivery. A recovery loop periodically requeues pending notifications whose lease expired, allowing work to resume after a worker crash without treating Redis as the source of truth.

## Production Considerations

- Use an outbox table to make database writes and queue publication atomic.
- Replace the mock sender with provider adapters for email, push, and in-app delivery.
- Use Kafka or a durable managed queue for higher throughput.
- Schedule in each user timezone and add quiet hours.
- Add authentication, tenant isolation, provider webhooks, OpenTelemetry, and a dead-letter workflow.
- Replace the single development API key with tenant-scoped credentials and role-based authorization.
- Move secrets and local database credentials to a secret manager.
