# Configuration

Pooly Sentinel uses YAML configuration with strict decoding and validation. Unknown fields are rejected where the YAML decoder can identify them.

The current supported configuration version is `1`.

## Secret Handling

Configuration should reference secrets through environment variable names only. Do not place webhook URLs, API keys, passwords, tokens, private keys, or authorized-key contents directly in YAML.

The example config uses `POOLY_DISCORD_WEBHOOK` as an environment variable name only; it is not a secret value.

## Safe Defaults

- API binds to `127.0.0.1:9587`.
- Logging defaults to text at info level.
- Production collectors are disabled in the Task 2 foundation.
- Local file receiver is enabled as the free-core receiver.
- Paid receivers are disabled and validation rejects enabled paid receivers.
- SQLite uses a bounded busy timeout and WAL by default where supported.
- Storage filenames must be plain filenames, not paths.
- Resource collectors are individually configurable and collect observations only; they do not evaluate alert thresholds.
- Resource `timeout` must be less than `interval`, mount paths must be absolute, and glob patterns are validated.
- Task 5 collectors for systemd, journald, SSH, and file state are disabled by default and can be run manually for diagnostics.
- systemd collection uses configured `critical_services` as factual targets only; no service restart, reload, enable, disable, or remediation is performed.
- journal streams use bounded JSON output, cursor state, field-length limits, and redacted summaries; raw journal dumps and full `MESSAGE` values are not emitted.
- SSH collection uses effective configuration and listening socket facts only; it does not edit config files or restart SSH.
- filewatch targets must be explicit absolute paths with type `file`, `directory`, or `any`; bounded file hashing uses no-follow descriptor reads and directory manifests expose truncation instead of silently replacing baselines.
- Rules are typed YAML entries evaluated against collector observations only. They may define warning, failure, and critical thresholds plus sustained and recovery durations.
- Rule evaluation owns WARN/FAIL/CRITICAL decisions. Collectors still emit facts only.
- Incident lifecycle persistence is local-only. Task 7 adds single-cycle notification delivery, but production scheduling, remediation, API serving, and systemd readiness are not implemented.
- Notification delivery is disabled and dry-run by default. Webhook destinations are referenced by environment variable name, not raw URL.

## Rules

Each rule has a stable `id`, `collector`, `metric` or `event_category`, optional `target`, threshold blocks, `recover_for`, and explicit missing/stale-data policies. Supported threshold operators are numeric comparison, equality/inequality, boolean true/false, state match, and event-category match. The configuration rejects duplicate IDs, unknown operators, unsafe targets, unsupported metric-name shapes, secret-bearing text, excessive summaries, and excessive rule counts.

## Notifications

Task 7 notification delivery uses `notify.enabled`, `notify.dry_run`, and `notify.receivers`. Receiver destinations are referenced through environment variable names such as `POOLY_WEBHOOK_URL`. Enabled webhook receivers require `url_env`; raw URLs are intentionally absent from the example configuration. Dry-run mode renders safe delivery results without contacting receivers or updating `last_alerted`.

See `docs/config.example.yaml`.
