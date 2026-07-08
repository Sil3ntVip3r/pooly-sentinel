# Pooly Sentinel

Pooly Sentinel is the planned Go-based replacement path for the current Bash-based Pooly Server Guard. This repository starts the new agent foundation without changing the existing guard.

## Current Status

Step 12 records the documentation-only alpha acceptance audit on top of the Step 11 security hardening and release-readiness baseline.

- Go module: `github.com/Sil3ntVip3r/pooly-sentinel`
- Primary binary path: `cmd/pooly-agent`
- Primary service template: `systemd/pooly-sentinel-agent.service`
- Configuration loading and validation are present
- Redaction, structured logging, safe command execution, lifecycle signals, and version metadata are present
- SQLite storage migrations, typed repositories, current-state JSON, JSONL events, evidence writing, and storage doctor checks are present
- Linux resource collectors emit typed observations for CPU, load, memory, PSI, filesystems, disk I/O, network, and uptime
- Linux systemd, journald, SSH, and file-state collectors emit typed factual observations and safe events
- Rule evaluation consumes typed observations and supports pending, sustained, critical, and recovery states
- Incident lifecycle persistence deduplicates, escalates, resolves, and reopens local incident records by stable fingerprint
- Single-cycle notification delivery can render safe payloads, send configured webhooks, persist attempts, and update `last_alerted` after success
- Read-only localhost API endpoints expose safe health, readiness, status, incident, delivery-history, and report-summary JSON
- Local report preview summarizes existing storage only
- `pooly-agent run` opens storage, optionally starts the localhost API and scheduler, sends truthful systemd readiness, and handles graceful shutdown
- Scheduler status and dry-run one-shot cycle commands are available
- Hardened install/uninstall helpers, release checks with `govulncheck`, secret scanning, local dry-run validation, no-follow state/log file handling, and watchdog shutdown joins are available
- Generic webhooks do not follow redirects; redirect responses are delivery failures and redirect targets are not included in summaries
- The Step 12 alpha acceptance audit passed for `14d5dbf fix: harden alpha security boundaries`; see `docs/alpha-acceptance-audit.md`
- The scheduler is disabled by default; report delivery, remediation, updating, public API exposure, new notification receivers, broad new collectors, and dashboards are not implemented

The current `pooly-agent` entrypoint supports safe one-shot manual collector runs, rule validation, fixture-based rule tests, local incident inspection, notification diagnostics, API config checks, report preview, scheduler status/run-once diagnostics, doctor checks, and run lifecycle wiring. `run` starts scheduled collection only when `agent.scheduler.enabled` is explicitly true.

## Safety Rules

- Do not commit secrets, tokens, webhook values, or private key material.
- Do not print, log, or expose webhook URLs.
- Do not send raw journal, audit, command, or SSH key material to external receivers.
- Do not enable paid receivers by default.
- Do not require Netdata, Prometheus, Grafana, SaaS monitoring, or paid notification services.
- Do not auto-repair SSH or restart/reload SSH during alpha work.

## Planned Architecture

The intended data flow is:

```text
collector
  -> observation
  -> rule engine
  -> incident engine
  -> notification manager
  -> receiver
  -> storage/history
```

Collectors will never send notifications directly or create incidents directly.

Rule evaluation owns WARN/FAIL/CRITICAL decisions. The incident engine owns local lifecycle state. The notification engine owns delivery decisions and delivery records.

## Repository Layout

```text
cmd/pooly-agent/          pooly-agent entrypoint
internal/agent/           lifecycle and scheduler coordination
internal/api/             localhost status, health, readiness, and metrics API
internal/collectors/      resource, systemd, journal, SSH, filewatch, and audit collectors
internal/command/         safe command runner
internal/config/          config loader and validator
internal/incidents/       incident lifecycle and fingerprints
internal/logging/         structured logging and redaction
internal/metrics/         metric registry and safe labels
internal/notify/          notification manager and receivers
internal/redaction/       secret and sensitive-output redaction
internal/retention/       retention cleanup policy
internal/rules/           rule engine
internal/storage/         SQLite, JSONL, evidence, rollups, and migrations
internal/systemdnotify/   systemd readiness and watchdog support
scripts/                  install, uninstall, and development helpers
systemd/                  service template
docs/                     design notes
```

## Development Checks

```bash
go fmt ./...
go mod tidy
go test ./...
go test -race ./...
go build ./cmd/pooly-agent
```

Alpha release checks:

```bash
scripts/check-release.sh
scripts/local-dry-run.sh --binary ./pooly-agent
scripts/scan-secrets.sh

# check-release requires a preinstalled govulncheck:
# go install golang.org/x/vuln/cmd/govulncheck@latest
```

Install preview:

```bash
scripts/install.sh --dry-run
scripts/uninstall.sh --dry-run
```
