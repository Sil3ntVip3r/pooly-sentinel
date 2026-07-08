package redaction

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		forbidden []string
		want      string
	}{
		{
			name:      "discord webhook",
			input:     "send to " + fakeDiscordWebhook("discord.com", "1234567890", "abc.DEF-gh_ij"),
			forbidden: []string{"1234567890", "abc.DEF-gh_ij", webhookHostPath("discord.com")},
		},
		{
			name:      "discordapp webhook",
			input:     "send to " + fakeDiscordWebhook("discordapp.com", "1234567890", "abcDEF"),
			forbidden: []string{webhookHostPath("discordapp.com"), "abcDEF"},
		},
		{
			name:      "discord canary webhook",
			input:     "send to " + fakeDiscordWebhook("canary.discord.com", "1234567890", "abcDEF"),
			forbidden: []string{webhookHostPath("canary.discord.com"), "abcDEF"},
		},
		{
			name:      "discord ptb webhook",
			input:     "send to " + fakeDiscordWebhook("ptb.discordapp.com", "1234567890", "abcDEF"),
			forbidden: []string{webhookHostPath("ptb.discordapp.com"), "abcDEF"},
		},
		{
			name:      "generic webhook token URL",
			input:     "send to https://hooks.example.test/webhook/channel01/abcdefghijklmnopqrstuvwxyz",
			forbidden: []string{"hooks.example.test", "abcdefghijklmnopqrstuvwxyz"},
		},
		{
			name:      "authorization bearer",
			input:     "Authorization: " + "Bearer " + "abc.def.ghi",
			forbidden: []string{"abc.def.ghi"},
			want:      "Authorization: [REDACTED]",
		},
		{
			name:      "password assignment",
			input:     "password=hunter2",
			forbidden: []string{"hunter2"},
			want:      "password=[REDACTED]",
		},
		{
			name:      "api key assignment",
			input:     "api_key: \"key-12345\"",
			forbidden: []string{"key-12345"},
			want:      "api_key: [REDACTED]",
		},
		{
			name:      "secret env value",
			input:     "POOLY_DISCORD_WEBHOOK=" + fakeDiscordWebhook("discord.com", "1", "redaction-test-token"),
			forbidden: []string{"redaction-test-token", webhookHostPath("discord.com")},
		},
		{
			name:      "sensitive query parameters",
			input:     "https://example.test/path?ok=yes&token=abc123&api_key=def456&name=node",
			forbidden: []string{"abc123", "def456"},
			want:      "https://example.test/path?ok=yes&token=[REDACTED]&api_key=[REDACTED]&name=node",
		},
		{
			name:      "private key block",
			input:     "-----BEGIN OPENSSH PRIVATE KEY-----\nabc123\n-----END OPENSSH PRIVATE KEY-----",
			forbidden: []string{"abc123", "OPENSSH PRIVATE KEY"},
			want:      "[REDACTED]",
		},
		{
			name:      "authorized key contents",
			input:     "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakePublicKeyMaterial comment",
			forbidden: []string{"AAAAC3NzaC1lZDI1NTE5AAAAIFakePublicKeyMaterial", "comment"},
			want:      "[REDACTED]",
		},
		{
			name:      "safe text stays safe",
			input:     "component=agent severity=warn duration=5s",
			forbidden: []string{},
			want:      "component=agent severity=warn duration=5s",
		},
		{
			name:      "harmless webhook documentation URL stays safe",
			input:     "see https://example.test/docs/webhook/setup for docs",
			forbidden: []string{},
			want:      "see https://example.test/docs/webhook/setup for docs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.input)
			if tt.want != "" && got != tt.want {
				t.Fatalf("Redact() = %q, want %q", got, tt.want)
			}
			if tt.want == "" && !strings.Contains(got, Replacement) {
				t.Fatalf("Redact() = %q, want replacement marker", got)
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(got, forbidden) {
					t.Fatalf("Redact() leaked %q in %q", forbidden, got)
				}
			}
		})
	}
}

func TestErrorRedactsFormattedErrors(t *testing.T) {
	secret := fakeDiscordWebhook("discord.com", "123", "redaction-test-token")
	err := Error(fmt.Errorf("delivery failed: %w", errors.New(secret)))
	got := fmt.Sprintf("%v", err)
	if strings.Contains(got, secret) || strings.Contains(got, "redaction-test-token") {
		t.Fatalf("formatted error leaked secret: %q", got)
	}
	if !strings.Contains(got, Replacement) {
		t.Fatalf("formatted error missing replacement marker: %q", got)
	}
}

func fakeDiscordWebhook(domain, id, token string) string {
	return "https://" + webhookHostPath(domain) + "/" + id + "/" + token
}

func webhookHostPath(domain string) string {
	return domain + "/api/" + "webhooks"
}

func TestIsSensitiveKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{key: "component", want: false},
		{key: "duration", want: false},
		{key: "webhook_env", want: true},
		{key: "api_key", want: true},
		{key: "Authorization", want: true},
		{key: "authorized_keys", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := IsSensitiveKey(tt.key); got != tt.want {
				t.Fatalf("IsSensitiveKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
