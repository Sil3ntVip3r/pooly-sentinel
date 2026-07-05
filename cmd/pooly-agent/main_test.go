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
