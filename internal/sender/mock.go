package sender

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/dinailman/notification-engine-personalized/internal/models"
)

type Sender interface {
	Send(context.Context, models.Notification) (string, error)
}

type Mock struct{ Logger *slog.Logger }

func (m Mock) Send(_ context.Context, n models.Notification) (string, error) {
	if strings.Contains(strings.ToLower(n.Body), "{{fail}}") || strings.Contains(strings.ToLower(n.Body), "fail_delivery") {
		return "", errors.New("mock provider rejected delivery")
	}
	m.Logger.Info("notification sent", "notification_id", n.ID, "user_id", n.UserID, "channel", n.Channel, "subject", n.Subject)
	return "mock-provider-accepted", nil
}
