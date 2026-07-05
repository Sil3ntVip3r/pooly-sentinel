package main

import (
	"os"
	"path/filepath"
	"testing"
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
}

func writeTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	data := []byte(`version: "1"
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
receivers:
  - name: local_file
    type: file
    cost_class: free_core
    enabled: true
storage:
  state_dir: ` + dir + `
  log_dir: ` + filepath.Join(dir, "logs") + `
`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}
