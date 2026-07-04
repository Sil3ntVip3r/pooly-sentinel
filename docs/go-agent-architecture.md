# Go Agent Architecture

The module path is `github.com/Sil3ntVip3r/pooly-sentinel`.

## Package Boundaries

- `internal/agent`: top-level lifecycle and scheduling
- `internal/api`: localhost-only API
- `internal/command`: safe command runner
- `internal/config`: config schema, defaults, loading, and validation
- `internal/collectors`: observations from Linux, systemd, journald, SSH, filewatch, and audit sources
- `internal/rules`: observation-to-incident candidate evaluation
- `internal/incidents`: incident identity and lifecycle
- `internal/notify`: routing, grouping, dedupe, silence, inhibition, rendering, and delivery
- `internal/storage`: SQLite state, JSONL events, evidence files, rollups, and migrations
- `internal/redaction`: common sensitive-data redaction
- `internal/logging`: redacting structured logging
- `internal/metrics`: metric naming and safe label validation
- `internal/retention`: cleanup policy
- `internal/systemdnotify`: readiness and watchdog support

## Context Policy

Future collectors, receivers, storage methods, command runners, and HTTP operations should take `context.Context` as their first argument. Contexts should not be stored in structs, nil contexts should not be accepted, and cancel functions should always be called.

## Dependency Policy

Use the Go standard library first. Keep runtime dependencies minimal and avoid release-build `replace` directives.
