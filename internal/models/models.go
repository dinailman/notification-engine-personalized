package models

import "time"

const (
	ChannelEmail        = "email"
	ChannelPush         = "push"
	ChannelInApp        = "in_app"
	FrequencyDaily      = "daily"
	FrequencyWeekly     = "weekly"
	TriggerScheduled    = "scheduled"
	TriggerEvent        = "event"
	NotificationPending = "pending"
	NotificationSent    = "sent"
	NotificationFailed  = "failed"
)

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	// Timezone names the IANA zone every local time for this user is read in.
	Timezone string `json:"timezone"`
	Active   bool   `json:"active"`
	// QuietHoursStart and QuietHoursEnd bound a "15:04" window, in Timezone, during
	// which the user is not disturbed. Both empty means the user has no quiet window;
	// an end at or before the start wraps past local midnight.
	QuietHoursStart string    `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd   string    `json:"quiet_hours_end,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Preference struct {
	UserID    string `json:"user_id"`
	Channel   string `json:"channel"`
	Frequency string `json:"frequency"`
	Enabled   bool   `json:"enabled"`
}

type Rule struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Name            string    `json:"name"`
	TriggerType     string    `json:"trigger_type"`
	EventType       string    `json:"event_type,omitempty"`
	ScheduledTime   string    `json:"scheduled_time,omitempty"`
	Frequency       string    `json:"frequency,omitempty"`
	Channel         string    `json:"channel"`
	SubjectTemplate string    `json:"subject_template"`
	BodyTemplate    string    `json:"body_template"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
}

type Event struct {
	ID         string         `json:"id"`
	UserID     string         `json:"user_id"`
	EventType  string         `json:"event_type"`
	ExternalID string         `json:"external_id,omitempty"`
	Payload    map[string]any `json:"payload"`
	OccurredAt time.Time      `json:"occurred_at"`
	CreatedAt  time.Time      `json:"created_at"`
}

type Notification struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	RuleID        string     `json:"rule_id"`
	EventID       *string    `json:"event_id,omitempty"`
	Channel       string     `json:"channel"`
	Subject       string     `json:"subject"`
	Body          string     `json:"body"`
	Status        string     `json:"status"`
	ScheduledAt   time.Time  `json:"scheduled_at"`
	AttemptCount  int        `json:"attempt_count"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
	LockedUntil   *time.Time `json:"-"`
	CreatedAt     time.Time  `json:"created_at"`
	SentAt        *time.Time `json:"sent_at,omitempty"`
}

type NotificationLog struct {
	ID               string    `json:"id"`
	NotificationID   string    `json:"notification_id"`
	AttemptNumber    int       `json:"attempt_number"`
	Status           string    `json:"status"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	ProviderResponse string    `json:"provider_response,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type Analytics struct {
	TotalSent int64           `json:"total_sent"`
	ByDay     []DayMetric     `json:"by_day"`
	ByUser    []UserMetric    `json:"by_user"`
	ByChannel []ChannelMetric `json:"by_channel"`
}

type DayMetric struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}
type UserMetric struct {
	UserID string `json:"user_id"`
	Count  int64  `json:"count"`
}
type ChannelMetric struct {
	Channel string `json:"channel"`
	Count   int64  `json:"count"`
}
