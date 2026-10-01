package notifier

import (
	"context"
	"errors"
)

var (
	ErrInvalidWebhookURL = errors.New("invalid Teams webhook URL")
	ErrDeliveryFailed    = errors.New("notification delivery failed")
)

type Notification struct {
	Type     string
	Severity string
	Title    string
	Summary  string
	Details  map[string]string
}

type Notifier interface {
	Send(context.Context, Notification) error
}

type FakeNotifier struct {
	Notifications []Notification
	Err           error
}

func (f *FakeNotifier) Send(_ context.Context, notification Notification) error {
	if f.Err != nil {
		return f.Err
	}
	f.Notifications = append(f.Notifications, notification)
	return nil
}
