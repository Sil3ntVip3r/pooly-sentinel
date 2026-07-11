package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/agent"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/config"
)

func TestRulesValidateAndFixtureCLI(t *testing.T) {
	configPath := filepath.Join("..", "..", "docs", "config.example.yaml")
	if err := runCLI([]string{"rules", "validate", "--config", configPath}); err != nil {
		t.Fatalf("rules validate error = %v", err)
	}
	fixture := filepath.Join(t.TempDir(), "observations.json")
	data := []byte(`[{
		"collector":"memory",
		"target":"system",
		"timestamp":"2026-07-04T12:00:00Z",
		"success":true,
		"supported":true,
		"metrics":[{
			"name":"pooly_memory_available_ratio",
			"value":0.5,
			"kind":"gauge",
			"unit":"ratio",
			"timestamp":"2026-07-04T12:00:00Z"
		}]
	}]`)
	if err := os.WriteFile(fixture, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := runCLI([]string{"rules", "test", "--config", configPath, "--fixture", fixture, "--json"}); err != nil {
		t.Fatalf("rules test error = %v", err)
	}
}

func TestNotificationsValidateAndDryRunCLI(t *testing.T) {
	configPath := filepath.Join("..", "..", "docs", "config.example.yaml")
	if err := runCLI([]string{"notifications", "validate", "--config", configPath}); err != nil {
		t.Fatalf("notifications validate error = %v", err)
	}
	if err := runCLI([]string{"notifications", "test", "--config", configPath, "--receiver", "local-webhook", "--json", "--dry-run"}); err != nil {
		t.Fatalf("notifications test error = %v", err)
	}
}

func TestAPIAndReportsCLI(t *testing.T) {
	configPath := writeTempConfig(t)
	if err := runCLI([]string{"api", "check", "--config", configPath}); err != nil {
		t.Fatalf("api check error = %v", err)
	}
	if err := runCLI([]string{"reports", "preview", "--config", configPath, "--json"}); err != nil {
		t.Fatalf("reports preview error = %v", err)
	}
	if err := runCLI([]string{"doctor", "--config", configPath}); err != nil {
		t.Fatalf("doctor error = %v", err)
	}
}

func TestSchedulerCLIStatusAndDryRun(t *testing.T) {
	configPath := writeTempConfig(t)
	if err := runCLI([]string{"scheduler", "status", "--config", configPath}); err != nil {
		t.Fatalf("scheduler status error = %v", err)
	}
	if err := runCLI([]string{"scheduler", "run-once", "--config", configPath, "--json", "--dry-run"}); err != nil {
		t.Fatalf("scheduler run-once dry-run error = %v", err)
	}
	cfg, err := config.LoadFile(context.Background(), configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	assertHermeticSchedulerFilesystemMounts(t, cfg)
	dbPath := filepath.Join(cfg.Storage.StateDir, cfg.Storage.DatabaseFile)
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run touched configured database path err=%v", err)
	}
	store, err := openConfiguredStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open configured store: %v", err)
	}
	defer store.Close()
	if _, ok, err := agent.LoadPersistedSchedulerStatus(context.Background(), store); err != nil || ok {
		t.Fatalf("dry-run persisted scheduler status ok=%t err=%v", ok, err)
	}
}

func writeTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	logDir := filepath.Join(dir, "logs")
	for _, path := range []string{stateDir, logDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	configPath := filepath.Join(dir, "config.yaml")
	data := []byte(fmt.Sprintf(`version: "1"
node:
  id: "001"
  name: "Node001 Toronto"
  hostname: "pooly-ssdnodes-001-toronto"
  region: "toronto"
  role: "mining-node"
  environment: "production"
  ring: "alpha"
api:
  enabled: false
  listen: "127.0.0.1:9587"
reports:
  enabled: true
  max_incidents: 100
  include_resolved: true
logging:
  level: "info"
  format: "json"
resources:
  enabled: true
  interval: 30s
  timeout: 3s
  cpu:
    enabled: true
  memory:
    enabled: true
  pressure:
    enabled: true
    missing_is_ok: true
  filesystem:
    enabled: true
    mounts:
      - %s
      - %s
      - %s
      - %s
  diskio:
    enabled: true
    auto_discover: true
    exclude:
      - loop*
      - ram*
      - fd*
      - sr*
  network:
    enabled: true
    auto_discover: true
    include: []
    exclude:
      - lo
      - docker*
      - veth*
      - br-*
  uptime:
    enabled: true
receivers:
  - name: local_file
    type: file
    cost_class: free_core
    enabled: true
storage:
  state_dir: %s
  log_dir: %s
`, yamlString("/"), yamlString(dir), yamlString(stateDir), yamlString(logDir), yamlString(stateDir), yamlString(logDir)))
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

func assertHermeticSchedulerFilesystemMounts(t *testing.T, cfg config.Config) {
	t.Helper()
	if !cfg.Resources.Enabled || !cfg.Resources.Filesystem.Enabled {
		t.Fatal("scheduler test config disabled resource filesystem collection")
	}
	if len(cfg.Resources.Filesystem.Mounts) == 0 {
		t.Fatal("scheduler test config did not declare filesystem mounts")
	}
	forbidden := map[string]struct{}{
		filepath.Clean(config.DefaultStateDir): {},
		filepath.Clean(config.DefaultLogDir):   {},
	}
	for _, mount := range cfg.Resources.Filesystem.Mounts {
		clean := filepath.Clean(mount)
		if _, ok := forbidden[clean]; ok {
			t.Fatalf("scheduler test inherited production-only filesystem mount %q", mount)
		}
		if _, err := os.Stat(clean); err != nil {
			t.Fatalf("scheduler test filesystem mount %q is not hermetic/existing: %v", mount, err)
		}
	}
}

func yamlString(value string) string {
	return strconv.Quote(value)
}
