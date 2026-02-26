package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/y0f/asura/internal/storage"
)

type DiscordSettings struct {
	WebhookURL string `json:"webhook_url"`
}

type DiscordSender struct{}

func (s *DiscordSender) Type() string { return "discord" }

func (s *DiscordSender) Send(ctx context.Context, channel *storage.NotificationChannel, payload *Payload) error {
	var settings DiscordSettings
	if err := json.Unmarshal(channel.Settings, &settings); err != nil {
		return fmt.Errorf("invalid discord settings: %w", err)
	}

	if settings.WebhookURL == "" {
		return fmt.Errorf("discord webhook_url is required")
	}

	text := FormatMessage(payload)

	// Discord webhook format with embed
	color := 0x2ECC71 // green
	switch payload.EventType {
	case "incident.created", "incident.reminder":
		color = 0xE74C3C // red
	case "incident.acknowledged":
		color = 0xF39C12 // yellow
	}

	body, _ := json.Marshal(map[string]any{
		"username": "Asura Monitor",
		"embeds": []map[string]any{
			{
				"title":       text,
				"color":       color,
				"description": string(marshalPayload(payload)),
				"timestamp":   time.Now().UTC().Format(time.RFC3339),
			},
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("discord request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord returned status %d", resp.StatusCode)
	}

	return nil
}
