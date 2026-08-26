package rules

import (
	"github.com/dinailman/personalized-notification-engine/internal/models"
	"testing"
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
