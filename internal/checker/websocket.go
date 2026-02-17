package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/y0f/Asura/internal/storage"
	"github.com/coder/websocket"
)

type WebSocketChecker struct{}

func (c *WebSocketChecker) Type() string { return "websocket" }

func (c *WebSocketChecker) Check(ctx context.Context, monitor *storage.Monitor) (*Result, error) {
	var settings storage.WebSocketSettings
	if len(monitor.Settings) > 0 {
		json.Unmarshal(monitor.Settings, &settings)
	}

	timeout := time.Duration(monitor.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := &websocket.DialOptions{}
	if len(settings.Headers) > 0 {
		header := http.Header{}
		for k, v := range settings.Headers {
			header.Set(k, v)
		}
		opts.HTTPHeader = header
	}

	start := time.Now()
	conn, _, err := websocket.Dial(ctx, monitor.Target, opts)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("WebSocket dial failed: %v", err),
		}, nil
	}
	defer conn.CloseNow()

	if settings.SendMessage != "" {
		err := conn.Write(ctx, websocket.MessageText, []byte(settings.SendMessage))
		if err != nil {
			return &Result{
				Status:       "down",
				ResponseTime: elapsed,
				Message:      fmt.Sprintf("WebSocket write failed: %v", err),
			}, nil
		}
	}

	if settings.ExpectReply != "" {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			return &Result{
				Status:       "down",
				ResponseTime: elapsed,
				Message:      fmt.Sprintf("WebSocket read failed: %v", err),
			}, nil
		}
		if !strings.Contains(string(msg), settings.ExpectReply) {
			return &Result{
				Status:       "down",
				ResponseTime: elapsed,
				Message:      "expected reply not found in WebSocket message",
			}, nil
		}
	}

	conn.Close(websocket.StatusNormalClosure, "check complete")

	return &Result{
		Status:       "up",
		ResponseTime: time.Since(start).Milliseconds(),
		Message:      "WebSocket connection successful",
	}, nil
}
