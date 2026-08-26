package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dinailman/personalized-notification-engine/internal/models"
	"github.com/dinailman/personalized-notification-engine/internal/rules"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("resource not found")

type Repository struct{ DB *pgxpool.Pool }

func (r *Repository) Health(ctx context.Context) error { return r.DB.Ping(ctx) }

func (r *Repository) CreateUser(ctx context.Context, user models.User, preferences []models.Preference) (models.User, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return user, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO users(email,name,timezone,active) VALUES($1,$2,$3,$4) RETURNING id,created_at,updated_at`, user.Email, user.Name, user.Timezone, true).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return user, err
	}
	for _, p := range preferences {
		if !rules.ValidChannel(p.Channel) || !rules.ValidFrequency(p.Frequency) {
			return user, fmt.Errorf("invalid preference")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO notification_preferences(user_id,channel,frequency,enabled) VALUES($1,$2,$3,$4)`, user.ID, p.Channel, p.Frequency, p.Enabled); err != nil {
			return user, err
		}
	}
	return user, tx.Commit(ctx)
}

func (r *Repository) GetUser(ctx context.Context, id string) (models.User, error) {
	var u models.User
	err := r.DB.QueryRow(ctx, `SELECT id,email,name,timezone,active,created_at,updated_at FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Email, &u.Name, &u.Timezone, &u.Active, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (r *Repository) UpdateUser(ctx context.Context, id string, name, timezone string, active bool) (models.User, error) {
	var u models.User
	err := r.DB.QueryRow(ctx, `UPDATE users SET name=$2,timezone=$3,active=$4,updated_at=now() WHERE id=$1 RETURNING id,email,name,timezone,active,created_at,updated_at`, id, name, timezone, active).Scan(&u.ID, &u.Email, &u.Name, &u.Timezone, &u.Active, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (r *Repository) Preferences(ctx context.Context, userID string) ([]models.Preference, error) {
	rows, err := r.DB.Query(ctx, `SELECT user_id,channel,frequency,enabled FROM notification_preferences WHERE user_id=$1 ORDER BY channel`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Preference{}
	for rows.Next() {
		var p models.Preference
		if err := rows.Scan(&p.UserID, &p.Channel, &p.Frequency, &p.Enabled); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) ReplacePreferences(ctx context.Context, userID string, preferences []models.Preference) ([]models.Preference, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM notification_preferences WHERE user_id=$1`, userID); err != nil {
		return nil, err
	}
	for _, p := range preferences {
		if !rules.ValidChannel(p.Channel) || !rules.ValidFrequency(p.Frequency) {
			return nil, fmt.Errorf("invalid preference")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO notification_preferences(user_id,channel,frequency,enabled) VALUES($1,$2,$3,$4)`, userID, p.Channel, p.Frequency, p.Enabled); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.Preferences(ctx, userID)
}

func (r *Repository) CreateRule(ctx context.Context, rule models.Rule) (models.Rule, error) {
	if !rules.ValidChannel(rule.Channel) || !rules.ValidTrigger(rule.TriggerType) || (rule.TriggerType == models.TriggerEvent && rule.EventType == "") || (rule.TriggerType == models.TriggerScheduled && (!rules.ValidFrequency(rule.Frequency) || rule.ScheduledTime == "")) {
		return rule, fmt.Errorf("invalid notification rule")
	}
	err := r.DB.QueryRow(ctx, `INSERT INTO notification_rules(user_id,name,trigger_type,event_type,scheduled_time,frequency,channel,subject_template,body_template,enabled) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,'')::time,NULLIF($6,''),$7,$8,$9,$10) RETURNING id,created_at`, rule.UserID, rule.Name, rule.TriggerType, rule.EventType, rule.ScheduledTime, rule.Frequency, rule.Channel, rule.SubjectTemplate, rule.BodyTemplate, rule.Enabled).Scan(&rule.ID, &rule.CreatedAt)
	return rule, err
}

func (r *Repository) ListRules(ctx context.Context, userID string) ([]models.Rule, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,user_id,name,trigger_type,COALESCE(event_type,''),COALESCE(scheduled_time::text,''),COALESCE(frequency,''),channel,subject_template,body_template,enabled,created_at FROM notification_rules WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Rule{}
	for rows.Next() {
		var x models.Rule
		if err := rows.Scan(&x.ID, &x.UserID, &x.Name, &x.TriggerType, &x.EventType, &x.ScheduledTime, &x.Frequency, &x.Channel, &x.SubjectTemplate, &x.BodyTemplate, &x.Enabled, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateRule(ctx context.Context, id string, enabled bool, subject, body string) (models.Rule, error) {
	var x models.Rule
	err := r.DB.QueryRow(ctx, `UPDATE notification_rules SET enabled=$2,subject_template=$3,body_template=$4 WHERE id=$1 RETURNING id,user_id,name,trigger_type,COALESCE(event_type,''),COALESCE(scheduled_time::text,''),COALESCE(frequency,''),channel,subject_template,body_template,enabled,created_at`, id, enabled, subject, body).Scan(&x.ID, &x.UserID, &x.Name, &x.TriggerType, &x.EventType, &x.ScheduledTime, &x.Frequency, &x.Channel, &x.SubjectTemplate, &x.BodyTemplate, &x.Enabled, &x.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, ErrNotFound
	}
	return x, err
}

func (r *Repository) DeleteRule(ctx context.Context, id string) error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM notification_rules WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) CreateEvent(ctx context.Context, event models.Event) (models.Event, []string, error) {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return event, nil, err
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return event, nil, err
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `INSERT INTO events(user_id,event_type,external_id,payload,occurred_at) VALUES($1,$2,NULLIF($3,''),$4,$5) ON CONFLICT (external_id) DO NOTHING RETURNING id,created_at`, event.UserID, event.EventType, event.ExternalID, payload, event.OccurredAt)
	err = row.Scan(&event.ID, &event.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id,user_id,event_type,COALESCE(external_id,''),payload,occurred_at,created_at FROM events WHERE external_id=$1`, event.ExternalID).Scan(&event.ID, &event.UserID, &event.EventType, &event.ExternalID, &payload, &event.OccurredAt, &event.CreatedAt)
		if err != nil {
			return event, nil, err
		}
		return event, nil, tx.Commit(ctx)
	}
	if err != nil {
		return event, nil, err
	}
	rows, err := tx.Query(ctx, `SELECT r.id,r.channel,r.subject_template,r.body_template FROM notification_rules r JOIN notification_preferences p ON p.user_id=r.user_id AND p.channel=r.channel AND p.enabled=true WHERE r.user_id=$1 AND r.enabled=true AND r.trigger_type='event' AND r.event_type=$2`, event.UserID, event.EventType)
	if err != nil {
		return event, nil, err
	}
	type ruleRow struct{ id, channel, subject, body string }
	matched := []ruleRow{}
	for rows.Next() {
		var item ruleRow
		if err = rows.Scan(&item.id, &item.channel, &item.subject, &item.body); err != nil {
			rows.Close()
			return event, nil, err
		}
		matched = append(matched, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return event, nil, err
	}
	rows.Close()
	ids := []string{}
	for _, item := range matched {
		var id string
		err = tx.QueryRow(ctx, `INSERT INTO notifications(user_id,rule_id,event_id,channel,subject,body,status,scheduled_at) VALUES($1,$2,$3,$4,$5,$6,'pending',now()) RETURNING id`, event.UserID, item.id, event.ID, item.channel, rules.Render(item.subject, map[string]string{"event_type": event.EventType}), rules.Render(item.body, map[string]string{"event_type": event.EventType})).Scan(&id)
		if err != nil {
			return event, nil, err
		}
		ids = append(ids, id)
	}
	return event, ids, tx.Commit(ctx)
}

// scheduledDueQuery finds scheduled rules whose local firing time falls in the half-open
// window (since, now], evaluated in each user's own timezone rather than in UTC. The
// LATERAL block builds the rule's firing instant on the local date at each end of the
// window -- the two differ only when the window crosses local midnight -- and keeps the
// one that lands inside it.
//
// Both DST transitions fall out of evaluating the window in local wall clock:
//
//   - Fall back (01:00-02:00 happens twice): local time moves backwards, so the window at
//     the transition tick is empty and the rule fires once, on the first pass through the
//     repeated hour. The unique index on (user_id, rule_id, occurrence_date) -- where
//     occurrence_date is now the user's local date -- suppresses the second pass. Chosen
//     because a duplicate notification is worse for a user than a slightly early one.
//   - Spring forward (02:00-03:00 does not exist): a rule at 02:30 would otherwise be
//     skipped for the day. The local window at the transition tick runs from 01:59:5x
//     straight to 03:00:0x, so 02:30 falls inside it and the rule fires at the jump.
const scheduledDueQuery = `
SELECT r.id, r.user_id, r.channel, r.subject_template, r.body_template, f.fire_ts::date::text
FROM notification_rules r
JOIN users u ON u.id = r.user_id AND u.active = true
JOIN notification_preferences p ON p.user_id = r.user_id AND p.channel = r.channel AND p.enabled = true
CROSS JOIN LATERAL (
    SELECT ts FROM (VALUES
        ((($1::timestamptz AT TIME ZONE u.timezone)::date + r.scheduled_time)),
        ((($2::timestamptz AT TIME ZONE u.timezone)::date + r.scheduled_time))
    ) AS candidate(ts)
    WHERE ts >  ($1::timestamptz AT TIME ZONE u.timezone)
      AND ts <= ($2::timestamptz AT TIME ZONE u.timezone)
    ORDER BY ts LIMIT 1
) AS f(fire_ts)
WHERE r.enabled = true
  AND r.trigger_type = 'scheduled'
  AND (r.frequency = 'daily' OR (r.frequency = 'weekly' AND EXTRACT(ISODOW FROM f.fire_ts::date) = 1))`

func (r *Repository) CreateScheduled(ctx context.Context, since, now time.Time) ([]string, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, scheduledDueQuery, since.UTC(), now.UTC())
	if err != nil {
		return nil, err
	}
	type ruleRow struct{ id, userID, channel, subject, body, localDate string }
	matched := []ruleRow{}
	for rows.Next() {
		var item ruleRow
		if err = rows.Scan(&item.id, &item.userID, &item.channel, &item.subject, &item.body, &item.localDate); err != nil {
			rows.Close()
			return nil, err
		}
		matched = append(matched, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	ids := []string{}
	for _, item := range matched {
		var id string
		err = tx.QueryRow(ctx, `INSERT INTO notifications(user_id,rule_id,channel,subject,body,status,scheduled_at,occurrence_date) VALUES($1,$2,$3,$4,$5,'pending',$6,$7::date) ON CONFLICT DO NOTHING RETURNING id`, item.userID, item.id, item.channel, item.subject, item.body, now.UTC(), item.localDate).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Repository) GetNotification(ctx context.Context, id string) (models.Notification, error) {
	var n models.Notification
	err := r.DB.QueryRow(ctx, `SELECT id,user_id,rule_id,event_id,channel,subject,body,status,scheduled_at,attempt_count,next_attempt_at,locked_until,created_at,sent_at FROM notifications WHERE id=$1`, id).Scan(&n.ID, &n.UserID, &n.RuleID, &n.EventID, &n.Channel, &n.Subject, &n.Body, &n.Status, &n.ScheduledAt, &n.AttemptCount, &n.NextAttemptAt, &n.LockedUntil, &n.CreatedAt, &n.SentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, ErrNotFound
	}
	return n, err
}

func (r *Repository) ClaimNotification(ctx context.Context, id string) (models.Notification, bool, error) {
	var n models.Notification
	err := r.DB.QueryRow(ctx, `UPDATE notifications SET locked_until=now()+interval '60 seconds' WHERE id=$1 AND status='pending' AND (locked_until IS NULL OR locked_until < now()) RETURNING id,user_id,rule_id,event_id,channel,subject,body,status,scheduled_at,attempt_count,next_attempt_at,locked_until,created_at,sent_at`, id).Scan(&n.ID, &n.UserID, &n.RuleID, &n.EventID, &n.Channel, &n.Subject, &n.Body, &n.Status, &n.ScheduledAt, &n.AttemptCount, &n.NextAttemptAt, &n.LockedUntil, &n.CreatedAt, &n.SentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, false, nil
	}
	return n, err == nil, err
}

func (r *Repository) RecoverPending(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := r.DB.Query(ctx, `SELECT id FROM notifications WHERE status='pending' AND scheduled_at <= $1 AND (next_attempt_at IS NULL OR next_attempt_at <= $1) AND (locked_until IS NULL OR locked_until < $1) ORDER BY scheduled_at LIMIT 100`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) ListNotifications(ctx context.Context, userID string) ([]models.Notification, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,user_id,rule_id,event_id,channel,subject,body,status,scheduled_at,attempt_count,next_attempt_at,locked_until,created_at,sent_at FROM notifications WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.RuleID, &n.EventID, &n.Channel, &n.Subject, &n.Body, &n.Status, &n.ScheduledAt, &n.AttemptCount, &n.NextAttemptAt, &n.LockedUntil, &n.CreatedAt, &n.SentAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *Repository) Logs(ctx context.Context, id string) ([]models.NotificationLog, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,notification_id,attempt_number,status,COALESCE(error_message,''),COALESCE(provider_response,''),created_at FROM notification_logs WHERE notification_id=$1 ORDER BY attempt_number`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.NotificationLog{}
	for rows.Next() {
		var l models.NotificationLog
		if err := rows.Scan(&l.ID, &l.NotificationID, &l.AttemptNumber, &l.Status, &l.ErrorMessage, &l.ProviderResponse, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Repository) MarkSent(ctx context.Context, id string, attempt int, provider string) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE notifications SET status='sent',attempt_count=$2,sent_at=now(),next_attempt_at=NULL,locked_until=NULL WHERE id=$1`, id, attempt); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO notification_logs(notification_id,attempt_number,status,provider_response) VALUES($1,$2,'sent',$3)`, id, attempt, provider); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) MarkFailed(ctx context.Context, id string, attempt int, message string, retry bool, next time.Time) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status := models.NotificationFailed
	if retry {
		status = models.NotificationPending
	}
	if _, err = tx.Exec(ctx, `UPDATE notifications SET status=$2,attempt_count=$3,next_attempt_at=$4,locked_until=NULL WHERE id=$1`, id, status, attempt, next); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO notification_logs(notification_id,attempt_number,status,error_message) VALUES($1,$2,'failed',$3)`, id, attempt, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) Analytics(ctx context.Context, from, to time.Time, userID, channel string) (models.Analytics, error) {
	out := models.Analytics{}
	args := []any{from, to}
	where := ` WHERE status='sent' AND sent_at >= $1 AND sent_at < $2`
	if userID != "" {
		args = append(args, userID)
		where += fmt.Sprintf(" AND user_id=$%d", len(args))
	}
	if channel != "" {
		args = append(args, channel)
		where += fmt.Sprintf(" AND channel=$%d", len(args))
	}
	if err := r.DB.QueryRow(ctx, `SELECT count(*) FROM notifications`+where, args...).Scan(&out.TotalSent); err != nil {
		return out, err
	}
	rows, err := r.DB.Query(ctx, `SELECT sent_at::date,count(*) FROM notifications`+where+` GROUP BY sent_at::date ORDER BY sent_at::date`, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var x models.DayMetric
		if err := rows.Scan(&x.Date, &x.Count); err != nil {
			rows.Close()
			return out, err
		}
		out.ByDay = append(out.ByDay, x)
	}
	rows.Close()
	rows, err = r.DB.Query(ctx, `SELECT user_id,count(*) FROM notifications`+where+` GROUP BY user_id ORDER BY user_id`, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var x models.UserMetric
		if err := rows.Scan(&x.UserID, &x.Count); err != nil {
			rows.Close()
			return out, err
		}
		out.ByUser = append(out.ByUser, x)
	}
	rows.Close()
	rows, err = r.DB.Query(ctx, `SELECT channel,count(*) FROM notifications`+where+` GROUP BY channel ORDER BY channel`, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var x models.ChannelMetric
		if err := rows.Scan(&x.Channel, &x.Count); err != nil {
			return out, err
		}
		out.ByChannel = append(out.ByChannel, x)
	}
	return out, rows.Err()
}

func NewID() string { return uuid.NewString() }
