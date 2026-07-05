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
5. Rule, incident, notification, reporting, API, and systemd integration.
6. Tests and release checks.

## Current Implementation Status

Task 3 implements the storage foundation only. Production collectors, monitoring loops, notification delivery, journald parsing, SSH checks, file watching, resource collection, rule evaluation, incident lifecycle processing, and systemd readiness remain intentionally unimplemented.
