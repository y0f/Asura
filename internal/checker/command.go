package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/asura-monitor/asura/internal/storage"
)

type CommandChecker struct {
	Allowlist []string
}

func (c *CommandChecker) Type() string { return "command" }

func (c *CommandChecker) Check(ctx context.Context, monitor *storage.Monitor) (*Result, error) {
	var settings storage.CommandSettings
	if len(monitor.Settings) > 0 {
		json.Unmarshal(monitor.Settings, &settings)
	}

	if settings.Command == "" {
		return &Result{Status: "down", Message: "no command specified"}, nil
	}

	// Security: validate command against allowlist
	if len(c.Allowlist) > 0 {
		allowed := false
		for _, prefix := range c.Allowlist {
			if settings.Command == prefix || strings.HasPrefix(settings.Command, prefix+"/") || strings.HasPrefix(settings.Command, prefix+"\\") {
				allowed = true
				break
			}
		}
		if !allowed {
			return &Result{
				Status:  "down",
				Message: fmt.Sprintf("command not in allowlist: %s", settings.Command),
			}, nil
		}
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, settings.Command, settings.Args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			StatusCode:   exitCode,
			Message:      fmt.Sprintf("command failed: %v", err),
			Body:         strings.TrimSpace(stderr.String()),
		}, nil
	}

	return &Result{
		Status:       "up",
		ResponseTime: elapsed,
		StatusCode:   0,
		Message:      "command succeeded",
		Body:         strings.TrimSpace(stdout.String()),
	}, nil
}
