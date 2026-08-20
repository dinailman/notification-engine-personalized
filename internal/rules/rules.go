package rules

import (
	"github.com/dinailman/personalized-notification-engine/internal/models"
	"strings"
	"time"
)

func ValidChannel(channel string) bool {
	return channel == models.ChannelEmail || channel == models.ChannelPush || channel == models.ChannelInApp
}

func ValidFrequency(frequency string) bool {
	return frequency == models.FrequencyDaily || frequency == models.FrequencyWeekly
}

func ValidTrigger(trigger string) bool {
	return trigger == models.TriggerScheduled || trigger == models.TriggerEvent
}

func EventMatches(rule models.Rule, event models.Event) bool {
	return rule.Enabled && rule.TriggerType == models.TriggerEvent && rule.EventType == event.EventType
}

func Due(rule models.Rule, now time.Time) bool {
	if !rule.Enabled || rule.TriggerType != models.TriggerScheduled || !ValidFrequency(rule.Frequency) {
		return false
	}
	parsed, err := time.Parse("15:04", rule.ScheduledTime)
	if err != nil {
		return false
	}
	return now.Hour() == parsed.Hour() && now.Minute() == parsed.Minute()
}

func Render(template string, values map[string]string) string {
	for key, value := range values {
		template = strings.ReplaceAll(template, "{{"+key+"}}", value)
	}
	return template
}
