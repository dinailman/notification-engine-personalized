package rules

import (
	"github.com/dinailman/personalized-notification-engine/internal/models"
	"testing"
	"time"
)

func TestEventMatches(t *testing.T) {
	rule := models.Rule{TriggerType: models.TriggerEvent, EventType: "document_uploaded", Enabled: true}
	if !EventMatches(rule, models.Event{EventType: "document_uploaded"}) {
		t.Fatal("matching event was rejected")
	}
	if EventMatches(rule, models.Event{EventType: "task_completed"}) {
		t.Fatal("different event matched")
	}
	rule.Enabled = false
	if EventMatches(rule, models.Event{EventType: "document_uploaded"}) {
		t.Fatal("disabled rule matched")
	}
}

func TestDueSchedule(t *testing.T) {
	now := time.Date(2026, 8, 19, 8, 0, 30, 0, time.UTC)
	rule := models.Rule{TriggerType: models.TriggerScheduled, ScheduledTime: "08:00", Frequency: models.FrequencyDaily, Enabled: true}
	if !Due(rule, now) {
		t.Fatal("due daily rule was not due")
	}
	if Due(rule, now.Add(time.Minute)) {
		t.Fatal("rule was due at the wrong minute")
	}
	rule.ScheduledTime = "bad"
	if Due(rule, now) {
		t.Fatal("invalid schedule was accepted")
	}
}

func TestRender(t *testing.T) {
	got := Render("Hello {{name}}: {{event_type}}", map[string]string{"name": "Mina", "event_type": "task_completed"})
	if got != "Hello Mina: task_completed" {
		t.Fatalf("rendered text = %q", got)
	}
}

func TestValidation(t *testing.T) {
	for _, channel := range []string{models.ChannelEmail, models.ChannelPush, models.ChannelInApp} {
		if !ValidChannel(channel) {
			t.Errorf("channel %q rejected", channel)
		}
	}
	if ValidChannel("sms") {
		t.Error("unsupported channel accepted")
	}
	if ValidFrequency("monthly") {
		t.Error("unsupported frequency accepted")
	}
}
