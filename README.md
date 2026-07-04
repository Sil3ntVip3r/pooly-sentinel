# Pooly Sentinel

Pooly Sentinel is the planned Go-based replacement path for the current Bash-based Pooly Server Guard. This repository starts the new agent foundation without changing the existing guard.

## Current Status

Task 2 core foundation is implemented. Production monitoring remains intentionally unimplemented.

- Go module: `github.com/Sil3ntVip3r/pooly-sentinel`
- Primary binary path: `cmd/pooly-agent`
- Primary service template: `systemd/pooly-sentinel-agent.service`
- Configuration loading and validation are present
- Redaction, structured logging, safe command execution, lifecycle signals, and version metadata are present
- Install and uninstall scripts remain stubs only
- Collectors, monitoring, storage, notification delivery, journald parsing, SSH checks, file watching, and resource collection are not implemented yet

The current `pooly-agent` entrypoint supports safe placeholder commands. `run` loads configuration and logging, then waits without starting production monitoring.

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

Collectors will never send notifications directly.

## Repository Layout

```text
cmd/pooly-agent/          pooly-agent entrypoint
internal/agent/           lifecycle and scheduler coordination
internal/api/             localhost status, health, readiness, and metrics API
internal/collectors/      resource, systemd, journal, SSH, filewatch, and audit collectors
internal/command/         future safe command runner
internal/config/          future config loader and validator
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

Release work will add broader checks, including race tests, coverage, vulnerability scanning, parser fixtures, and redaction tests.
