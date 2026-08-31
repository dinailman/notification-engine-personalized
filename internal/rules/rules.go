package rules

import (
	"strings"

	"github.com/dinailman/notification-engine-personalized/internal/models"
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

func Render(template string, values map[string]string) string {
	for key, value := range values {
		template = strings.ReplaceAll(template, "{{"+key+"}}", value)
	}
	return template
}
