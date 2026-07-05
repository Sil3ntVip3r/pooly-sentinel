package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validConfigYAML = `
version: "1"
node:
  id: "001"
  name: "Node001 Toronto"
  hostname: "pooly-ssdnodes-001-toronto"
  region: "toronto"
  role: "mining-node"
  environment: "production"
  ring: "alpha"
api:
  enabled: true
  bind: "127.0.0.1:9587"
logging:
  level: "info"
  format: "json"
receivers:
  - name: local_file
    type: file
    cost_class: free_core
    enabled: true
  - name: discord_primary
    type: discord
    cost_class: free_external
    enabled: false
    webhook_env: POOLY_DISCORD_WEBHOOK
storage:
  state_dir: /var/lib/pooly-sentinel
  log_dir: /var/log/pooly-sentinel
`

func TestLoadBytesValidConfig(t *testing.T) {
	cfg, err := LoadBytes(context.Background(), []byte(validConfigYAML))
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	if cfg.Version != CurrentConfigVersion {
		t.Fatalf("version = %q, want %q", cfg.Version, CurrentConfigVersion)
	}
	if cfg.Resources.Interval.Duration == 0 {
		t.Fatalf("default resource interval was not applied")
	}
	if cfg.API.Bind != DefaultAPIBind {
		t.Fatalf("api.bind = %q, want %q", cfg.API.Bind, DefaultAPIBind)
	}
}

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(validConfigYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadFile(context.Background(), path); err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
}

func TestExampleConfigLoads(t *testing.T) {
	_, err := LoadFile(context.Background(), filepath.Join("..", "..", "docs", "config.example.yaml"))
	if err != nil {
		t.Fatalf("example config should load: %v", err)
	}
}

func TestLoadBytesRejectsUnknownFields(t *testing.T) {
	input := validConfigYAML + "\nunknown_field: true\n"
	_, err := LoadBytes(context.Background(), []byte(input))
	if err == nil {
		t.Fatal("LoadBytes() error = nil, want unknown field error")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Fatalf("error = %q, want unknown field", err.Error())
	}
}

func TestLoadBytesRejectsInvalidConfigWithUsefulFields(t *testing.T) {
	input := strings.Replace(validConfigYAML, `bind: "127.0.0.1:9587"`, `bind: "0.0.0.0:9587"`, 1)
	_, err := LoadBytes(context.Background(), []byte(input))
	if err == nil {
		t.Fatal("LoadBytes() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "api.bind") {
		t.Fatalf("error = %q, want api.bind", err.Error())
	}
}

func TestLoadBytesRejectsSecretLiterals(t *testing.T) {
	input := strings.Replace(validConfigYAML, `webhook_env: POOLY_DISCORD_WEBHOOK`, `webhook_env: `+fakeDiscordWebhook(), 1)
	_, err := LoadBytes(context.Background(), []byte(input))
	if err == nil {
		t.Fatal("LoadBytes() error = nil, want secret validation error")
	}
	if strings.Contains(err.Error(), "redaction-test-token") || strings.Contains(err.Error(), webhookHostPath()) {
		t.Fatalf("validation error leaked secret: %q", err.Error())
	}
}

func TestLoadBytesRejectsPaidReceiversEnabled(t *testing.T) {
	input := strings.Replace(validConfigYAML, `  - name: local_file
    type: file
    cost_class: free_core
    enabled: true
  - name: discord_primary
    type: discord
    cost_class: free_external
    enabled: false
    webhook_env: POOLY_DISCORD_WEBHOOK`, `  - name: pushover_critical
    type: pushover
    cost_class: paid_external
    enabled: true`, 1)
	_, err := LoadBytes(context.Background(), []byte(input))
	if err == nil {
		t.Fatal("LoadBytes() error = nil, want paid receiver validation error")
	}
	if !strings.Contains(err.Error(), "paid receivers") {
		t.Fatalf("error = %q, want paid receiver detail", err.Error())
	}
}

func TestLoadBytesRejectsUnsupportedVersion(t *testing.T) {
	input := strings.Replace(validConfigYAML, `version: "1"`, `version: "999"`, 1)
	_, err := LoadBytes(context.Background(), []byte(input))
	if err == nil {
		t.Fatal("LoadBytes() error = nil, want version validation error")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("error = %q, want version detail", err.Error())
	}
}

func TestLoadBytesRejectsInvalidStorageFilename(t *testing.T) {
	input := strings.Replace(validConfigYAML, `storage:
  state_dir: /var/lib/pooly-sentinel
  log_dir: /var/log/pooly-sentinel`, `storage:
  state_dir: /var/lib/pooly-sentinel
  log_dir: /var/log/pooly-sentinel
  database_file: ../state.db`, 1)
	_, err := LoadBytes(context.Background(), []byte(input))
	if err == nil {
		t.Fatal("LoadBytes() error = nil, want storage filename validation error")
	}
	if !strings.Contains(err.Error(), "storage.database_file") {
		t.Fatalf("error = %q, want storage.database_file", err.Error())
	}
}

func TestLoadBytesRejectsInvalidStorageDuration(t *testing.T) {
	input := strings.Replace(validConfigYAML, `storage:
  state_dir: /var/lib/pooly-sentinel
  log_dir: /var/log/pooly-sentinel`, `storage:
  state_dir: /var/lib/pooly-sentinel
  log_dir: /var/log/pooly-sentinel
  sqlite:
    busy_timeout: 0s`, 1)
	_, err := LoadBytes(context.Background(), []byte(input))
	if err == nil {
		t.Fatal("LoadBytes() error = nil, want storage duration validation error")
	}
	if !strings.Contains(err.Error(), "storage.sqlite.busy_timeout") {
		t.Fatalf("error = %q, want storage.sqlite.busy_timeout", err.Error())
	}
}

func TestLoadBytesHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := LoadBytes(ctx, []byte(validConfigYAML))
	if err == nil {
		t.Fatal("LoadBytes() error = nil, want cancellation error")
	}
}

func fakeDiscordWebhook() string {
	return "https://" + webhookHostPath() + "/123/redaction-test-token"
}

func webhookHostPath() string {
	return "discord.com" + "/api/" + "webhooks"
}
