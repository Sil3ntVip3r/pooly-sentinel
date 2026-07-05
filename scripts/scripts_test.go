package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallDryRunDoesNotWriteTargets(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	binary := fakePoolyAgent(t, tmp)
	cmd := exec.Command("bash",
		filepath.Join(root, "scripts", "install.sh"),
		"--dry-run",
		"--binary", binary,
		"--prefix", filepath.Join(tmp, "prefix"),
		"--etc-dir", filepath.Join(tmp, "etc"),
		"--state-dir", filepath.Join(tmp, "state"),
		"--log-dir", filepath.Join(tmp, "log"),
		"--systemd-dir", filepath.Join(tmp, "systemd"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install dry-run failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, forbidden := range []string{"discord.com/api/webhooks", "Authorization: Bearer", "PRIVATE KEY"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("install output leaked forbidden text %q:\n%s", forbidden, text)
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, "prefix", "bin", "pooly-agent")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created binary target err=%v", err)
	}
}

func TestUninstallDryRunDoesNotDeleteTargets(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "prefix", "bin")
	serviceDir := filepath.Join(tmp, "systemd")
	stateDir := filepath.Join(tmp, "state")
	logDir := filepath.Join(tmp, "log")
	for _, dir := range []string{binDir, serviceDir, stateDir, logDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	paths := []string{
		filepath.Join(binDir, "pooly-agent"),
		filepath.Join(serviceDir, "pooly-sentinel-agent.service"),
		filepath.Join(stateDir, "state.db"),
		filepath.Join(logDir, "events.jsonl"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("placeholder"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	cmd := exec.Command("bash",
		filepath.Join(root, "scripts", "uninstall.sh"),
		"--dry-run",
		"--remove-binary",
		"--remove-service",
		"--purge-state", "--confirm-purge-state",
		"--purge-logs", "--confirm-purge-logs",
		"--prefix", filepath.Join(tmp, "prefix"),
		"--systemd-dir", serviceDir,
		"--state-dir", stateDir,
		"--log-dir", logDir,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("uninstall dry-run failed: %v\n%s", err, out)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("dry-run removed %s: %v", path, err)
		}
	}
}

func TestReleaseScriptsExistAndAreExecutable(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{"install.sh", "uninstall.sh", "check-release.sh", "local-dry-run.sh", "scan-secrets.sh"} {
		info, err := os.Stat(filepath.Join(root, "scripts", name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("%s is not executable: %v", name, info.Mode())
		}
	}
}

func TestSystemdServiceHardening(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "systemd", "pooly-sentinel-agent.service"))
	if err != nil {
		t.Fatalf("read service: %v", err)
	}
	text := string(data)
	required := []string{
		"Type=notify",
		"NotifyAccess=main",
		"Restart=on-failure",
		"RestartSec=5s",
		"StartLimitIntervalSec=10min",
		"StartLimitBurst=5",
		"WatchdogSec=60s",
		"TimeoutStartSec=30s",
		"TimeoutStopSec=30s",
		"StateDirectory=pooly-sentinel",
		"LogsDirectory=pooly-sentinel",
		"RuntimeDirectory=pooly-sentinel",
		"ConfigurationDirectory=pooly-sentinel",
		"EnvironmentFile=-/etc/pooly-sentinel/pooly-sentinel.env",
		"ExecStartPre=/usr/local/bin/pooly-agent check config --config /etc/pooly-sentinel/config.yaml",
		"ExecStart=/usr/local/bin/pooly-agent run --config /etc/pooly-sentinel/config.yaml",
		"NoNewPrivileges=true",
		"PrivateTmp=true",
		"ProtectSystem=full",
		"ProtectHome=read-only",
		"ReadWritePaths=/var/lib/pooly-sentinel /var/log/pooly-sentinel /run/pooly-sentinel",
		"StandardOutput=journal",
		"StandardError=journal",
	}
	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Fatalf("service missing %q", want)
		}
	}
	for _, forbidden := range []string{"discord.com/api/webhooks", "Authorization: Bearer", "PRIVATE KEY"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("service contains forbidden text %q", forbidden)
		}
	}
}

func TestSecretScanHelper(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "scan-secrets.sh"))
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("secret scan failed: %v\n%s", err, out)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("cwd: %v", err)
	}
	return filepath.Dir(cwd)
}

func fakePoolyAgent(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "pooly-agent")
	data := []byte(`#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  version)
    printf '%s\n' "pooly-agent test"
    ;;
  check)
    if [[ "${2:-}" != "config" ]]; then
      printf '%s\n' "unexpected check command" >&2
      exit 1
    fi
    ;;
  *)
    printf '%s\n' "fake pooly-agent only supports version and check config" >&2
    exit 1
    ;;
esac
`)
	if err := os.WriteFile(path, data, 0o700); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return path
}
