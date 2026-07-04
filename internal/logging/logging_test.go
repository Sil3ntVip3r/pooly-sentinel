package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestTextLoggerRedactsMessageAndAttributes(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(&buf, Options{Level: "debug", Format: "text"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Info("delivery failed for "+fakeDiscordWebhook(),
		Component("notify"),
		slog.String("token", "abc123"),
		slog.String("collector", "systemd"),
		slog.Duration(FieldDuration, 5*time.Second),
		slog.Any("err", errors.New("Authorization: "+"Bearer "+"bearer-secret")),
	)

	got := buf.String()
	for _, forbidden := range []string{"redaction-test-token", "abc123", "bearer-secret", webhookHostPath()} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log leaked %q in %q", forbidden, got)
		}
	}
	for _, want := range []string{"component=notify", "collector=systemd", "duration=5s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log missing safe field %q in %q", want, got)
		}
	}
}

func fakeDiscordWebhook() string {
	return "https://" + webhookHostPath() + "/123/redaction-test-token"
}

func webhookHostPath() string {
	return "discord.com" + "/api/" + "webhooks"
}

func TestJSONLoggerRedactsGroups(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(&buf, Options{Level: "info", Format: "json"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Info("config loaded",
		slog.Group("receiver",
			slog.String("name", "discord_primary"),
			slog.String("webhook_env", "POOLY_DISCORD_WEBHOOK"),
			slog.String("url", "https://example.test/?api_key=secret-key"),
		),
	)

	got := buf.String()
	for _, forbidden := range []string{"POOLY_DISCORD_WEBHOOK", "secret-key"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log leaked %q in %q", forbidden, got)
		}
	}
	if !strings.Contains(got, `"name":"discord_primary"`) {
		t.Fatalf("log missing safe grouped field: %q", got)
	}
}

func TestNewRejectsInvalidFormatAndLevel(t *testing.T) {
	if _, err := New(&bytes.Buffer{}, Options{Level: "trace", Format: "text"}); err == nil {
		t.Fatal("New() error = nil, want invalid level error")
	}
	if _, err := New(&bytes.Buffer{}, Options{Level: "info", Format: "xml"}); err == nil {
		t.Fatal("New() error = nil, want invalid format error")
	}
}
