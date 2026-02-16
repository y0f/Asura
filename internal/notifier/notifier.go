package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/asura-monitor/asura/internal/storage"
)

// Sender sends a notification via a specific channel type.
type Sender interface {
	Type() string
	Send(ctx context.Context, channel *storage.NotificationChannel, payload *Payload) error
}

// Payload contains the notification data.
type Payload struct {
	EventType   string             `json:"event_type"`
	Incident    *storage.Incident  `json:"incident,omitempty"`
	Monitor     *storage.Monitor   `json:"monitor,omitempty"`
	Change      *storage.ContentChange `json:"change,omitempty"`
}

// Dispatcher manages notification channels and dispatching.
type Dispatcher struct {
	store   storage.Store
	senders map[string]Sender
	logger  *slog.Logger
}

// NewDispatcher creates a notification dispatcher.
func NewDispatcher(store storage.Store, logger *slog.Logger) *Dispatcher {
	d := &Dispatcher{
		store:   store,
		senders: make(map[string]Sender),
		logger:  logger,
	}
	// Register all built-in senders
	d.RegisterSender(&WebhookSender{})
	d.RegisterSender(&EmailSender{})
	d.RegisterSender(&TelegramSender{})
	d.RegisterSender(&DiscordSender{})
	d.RegisterSender(&SlackSender{})
	return d
}

// RegisterSender adds a sender for a channel type.
func (d *Dispatcher) RegisterSender(s Sender) {
	d.senders[s.Type()] = s
}

// Notify sends notifications for an event to all matching channels.
func (d *Dispatcher) Notify(eventType string, data interface{}) {
	ctx := context.Background()

	channels, err := d.store.ListNotificationChannels(ctx)
	if err != nil {
		d.logger.Error("list notification channels", "error", err)
		return
	}

	payload := &Payload{EventType: eventType}
	switch v := data.(type) {
	case *storage.Incident:
		payload.Incident = v
	case *storage.ContentChange:
		payload.Change = v
	}

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if !matchesEvent(ch.Events, eventType) {
			continue
		}

		sender, ok := d.senders[ch.Type]
		if !ok {
			d.logger.Warn("no sender for channel type", "type", ch.Type)
			continue
		}

		go func(ch *storage.NotificationChannel) {
			if err := sender.Send(ctx, ch, payload); err != nil {
				d.logger.Error("notification send failed",
					"channel_id", ch.ID,
					"channel_type", ch.Type,
					"error", err,
				)
			} else {
				d.logger.Info("notification sent",
					"channel_id", ch.ID,
					"channel_type", ch.Type,
					"event", eventType,
				)
			}
		}(ch)
	}
}

// NotifyWithPayload sends notifications for the given payload.
func (d *Dispatcher) NotifyWithPayload(payload *Payload) {
	ctx := context.Background()

	channels, err := d.store.ListNotificationChannels(ctx)
	if err != nil {
		d.logger.Error("list notification channels", "error", err)
		return
	}

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if !matchesEvent(ch.Events, payload.EventType) {
			continue
		}

		sender, ok := d.senders[ch.Type]
		if !ok {
			continue
		}

		go func(ch *storage.NotificationChannel) {
			if err := sender.Send(ctx, ch, payload); err != nil {
				d.logger.Error("notification send failed", "channel_id", ch.ID, "error", err)
			}
		}(ch)
	}
}

// SendTest sends a test notification through a specific channel.
func (d *Dispatcher) SendTest(ch *storage.NotificationChannel, inc *storage.Incident) error {
	sender, ok := d.senders[ch.Type]
	if !ok {
		return fmt.Errorf("no sender for type: %s", ch.Type)
	}

	payload := &Payload{
		EventType: "test",
		Incident:  inc,
	}
	return sender.Send(context.Background(), ch, payload)
}

func matchesEvent(events []string, eventType string) bool {
	if len(events) == 0 {
		return true // subscribe to all if no filter
	}
	for _, e := range events {
		if e == eventType {
			return true
		}
	}
	return false
}

// FormatMessage creates a human-readable notification message.
func FormatMessage(p *Payload) string {
	switch p.EventType {
	case "incident.created":
		if p.Incident != nil {
			return fmt.Sprintf("[ALERT] Incident #%d opened for %s: %s",
				p.Incident.ID, p.Incident.MonitorName, p.Incident.Cause)
		}
	case "incident.acknowledged":
		if p.Incident != nil {
			return fmt.Sprintf("[ACK] Incident #%d for %s acknowledged by %s",
				p.Incident.ID, p.Incident.MonitorName, p.Incident.AcknowledgedBy)
		}
	case "incident.resolved":
		if p.Incident != nil {
			return fmt.Sprintf("[RESOLVED] Incident #%d for %s resolved by %s",
				p.Incident.ID, p.Incident.MonitorName, p.Incident.ResolvedBy)
		}
	case "content.changed":
		if p.Change != nil {
			return fmt.Sprintf("[CHANGE] Content changed for monitor #%d", p.Change.MonitorID)
		}
	case "test":
		return "[TEST] This is a test notification from Asura"
	}
	return fmt.Sprintf("[%s] Notification event", p.EventType)
}

func marshalPayload(p *Payload) []byte {
	b, _ := json.Marshal(p)
	return b
}
