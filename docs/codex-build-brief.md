# Codex Build Brief Summary

This repository was initialized from the Pooly Sentinel Phase 1 and Phase 1A build brief. This in-repo document is a safe summary for implementation work; it intentionally does not include secret values or webhook URLs.

## Product Direction

Pooly Sentinel is a self-hosted Go security and uptime monitoring agent for Pooly nodes. The current Bash-based guard remains available as the stable fallback while this agent is built.

The project must stay free-first, self-hosted, and security-focused. It must not become a Netdata clone, Prometheus clone, Grafana clone, SaaS monitoring stack, or paid notification dependency.

## Task 1 Scope

Create the repository skeleton and documentation:

- Go module `github.com/Sil3ntVip3r/pooly-sentinel`
- `cmd/pooly-agent/main.go`
- `internal` package layout
- `docs` directory
- `README.md`
- `systemd/pooly-sentinel-agent.service`
- install and uninstall script stubs

Task 1 does not implement production monitoring.

## Hard Rules

- Collectors never send notifications directly.
- Secrets, tokens, webhook URLs, SSH private key material, raw audit records, and raw journal dumps must not be logged or sent externally.
- Paid receivers are optional future integrations and disabled by default.
- No required third-party monitoring stack.
- No automatic security repair in alpha.
- No automatic SSH reload or restart in alpha.

## Future Implementation Order

1. Core Go foundation: config, redaction, logging, command runner, basic CLI.
2. Storage foundation: SQLite state, JSONL events, evidence, retention.
3. Resource collectors: CPU, memory, pressure, filesystem, disk I/O, network, uptime.
4. systemd, journald, SSH, and filewatch collectors.
5. Rule engine and incident engine.
6. Notification delivery.
7. Reporting, API, and systemd integration.
8. Tests and release checks.

## Current Implementation Status

Task 5 implemented Linux systemd, journald, SSH, and file-state collectors in addition to the Task 4 Linux resource collectors. These collectors gather facts only.

Task 6 implements deterministic rule evaluation and local incident lifecycle persistence. Rule thresholds are evaluated only from typed observations. The incident engine deduplicates, escalates, resolves, and reopens local incident records by stable fingerprint.

Task 7 implements single-cycle notification delivery for incident lifecycle events. Delivery reads existing incidents or incident transitions, renders safe payloads, sends through configured receivers, persists delivery attempts, and updates `last_alerted` only after success.

Step 8 implements read-only localhost API endpoints, local report preview from existing storage, systemd readiness/watchdog notification, and infrastructure-only `pooly-agent run` lifecycle wiring.

Step 9 implements the disabled-by-default production monitoring scheduler and agent run loop. The scheduler orchestrates existing collectors, rule evaluation, incident lifecycle updates, notification delivery for incident transitions, and safe scheduler status persistence.

Step 10 implements alpha hardening and release readiness: hardened install/uninstall helpers, alpha systemd service finalization, release-check and local dry-run scripts, secret-pattern scanning, doctor enhancements, and install/rollback/security documentation.

Step 11 implements security hardening and alpha acceptance fixes only: no-follow webhook redirects, stricter journald cursor behavior, SSH effective-config Match profile coverage, no-follow state/log file opens, command timeout process-group cleanup, API serve-error status reporting, govulncheck release checks, installer/uninstaller path validation, expanded redaction, atomic directory sync error returns, watchdog goroutine shutdown joins, and shared evidence path safety.

Report delivery, remediation, updating, public API serving, dashboards, broad new collectors, new notification receivers, remote fleet hub, real deployment, and Pooly Server Guard modifications remain intentionally unimplemented.
