package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"github.com/asura-monitor/asura/internal/storage"
)

type EmailSettings struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	From     string   `json:"from"`
	To       []string `json:"to"`
}

type EmailSender struct{}

func (s *EmailSender) Type() string { return "email" }

func (s *EmailSender) Send(ctx context.Context, channel *storage.NotificationChannel, payload *Payload) error {
	var settings EmailSettings
	if err := json.Unmarshal(channel.Settings, &settings); err != nil {
		return fmt.Errorf("invalid email settings: %w", err)
	}

	if settings.Host == "" || len(settings.To) == 0 {
		return fmt.Errorf("email host and recipients are required")
	}

	port := settings.Port
	if port == 0 {
		port = 587
	}

	subject := FormatMessage(payload)
	bodyText := subject + "\n\n" + string(marshalPayload(payload))

	msg := strings.Builder{}
	msg.WriteString(fmt.Sprintf("From: %s\r\n", settings.From))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(settings.To, ",")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(bodyText)

	addr := fmt.Sprintf("%s:%d", settings.Host, port)
	host, _, _ := net.SplitHostPort(addr)

	var auth smtp.Auth
	if settings.Username != "" {
		auth = smtp.PlainAuth("", settings.Username, settings.Password, host)
	}

	return smtp.SendMail(addr, auth, settings.From, settings.To, []byte(msg.String()))
}
