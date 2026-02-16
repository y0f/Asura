package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/asura-monitor/asura/internal/storage"
)

// Sender sends a notification via a specific channel type.
type Sender interface {
	Type() string
	Send(ctx context.Context, channel *storage.NotificationChannel, payload *Payload) error
}

// Payload contains the notification data.
type Payload struct {
	EventType string                `json:"event_type"`
	Incident  *storage.Incident     `json:"incident,omitempty"`
	Monitor   *storage.Monitor      `json:"monitor,omitempty"`
	Change    *storage.ContentChange `json:"change,omitempty"`
}

type Dispatcher struct {
	store   storage.Store
	senders map[string]Sender
	logger  *slog.Logger
	sem     chan struct{}
}

const maxConcurrentSends = 10

func NewDispatcher(store storage.Store, logger *slog.Logger) *Dispatcher {
	d := &Dispatcher{
		store:   store,
		senders: make(map[string]Sender),
		logger:  logger,
		sem:     make(chan struct{}, maxConcurrentSends),
	}
	d.RegisterSender(&WebhookSender{})
	d.RegisterSender(&EmailSender{})
	d.RegisterSender(&TelegramSender{})
	d.RegisterSender(&DiscordSender{})
	d.RegisterSender(&SlackSender{})
	return d
}

func (d *Dispatcher) RegisterSender(s Sender) {
	d.senders[s.Type()] = s
}

func (d *Dispatcher) NotifyWithPayload(payload *Payload) {
	channels, err := d.store.ListNotificationChannels(context.Background())
	if err != nil {
		d.logger.Error("list notification channels", "error", err)
		return
	}

	for _, ch := range channels {
		if !ch.Enabled || !matchesEvent(ch.Events, payload.EventType) {
			continue
		}

		sender, ok := d.senders[ch.Type]
		if !ok {
			d.logger.Warn("no sender for channel type", "type", ch.Type)
			continue
		}

		go d.sendWithRetry(sender, ch, payload)
	}
}

func (d *Dispatcher) SendTest(ch *storage.NotificationChannel, inc *storage.Incident) error {
	sender, ok := d.senders[ch.Type]
	if !ok {
		return fmt.Errorf("no sender for type: %s", ch.Type)
	}
	return sender.Send(context.Background(), ch, &Payload{
		EventType: "test",
		Incident:  inc,
	})
}

func (d *Dispatcher) sendWithRetry(sender Sender, ch *storage.NotificationChannel, payload *Payload) {
	d.sem <- struct{}{}
	defer func() { <-d.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(5 * time.Second)
		}
		if err := sender.Send(ctx, ch, payload); err != nil {
			lastErr = err
			d.logger.Warn("notification send attempt failed",
				"channel_id", ch.ID,
				"channel_type", ch.Type,
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}
		d.logger.Info("notification sent",
			"channel_id", ch.ID,
			"channel_type", ch.Type,
			"event", payload.EventType,
		)
		return
	}
	d.logger.Error("notification send failed after retry",
		"channel_id", ch.ID,
		"channel_type", ch.Type,
		"error", lastErr,
	)
}

func matchesEvent(events []string, eventType string) bool {
	if len(events) == 0 {
		return true
	}
	for _, e := range events {
		if e == eventType {
			return true
		}
	}
	return false
}

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
