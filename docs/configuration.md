# Configuration

Pooly Sentinel uses YAML configuration with strict decoding and validation. Unknown fields are rejected where the YAML decoder can identify them.

The current supported configuration version is `1`.

## Secret Handling

Configuration should reference secrets through environment variable names only. Do not place webhook URLs, API keys, passwords, tokens, private keys, or authorized-key contents directly in YAML.

The example config uses `POOLY_DISCORD_WEBHOOK` as an environment variable name only; it is not a secret value.

## Safe Defaults

- API is disabled by default and listens on `127.0.0.1:9587` when enabled. Non-loopback listen addresses require explicit `api.allow_non_loopback`.
- API read, write, and shutdown timeouts are bounded.
- Local report preview is enabled by default with a bounded incident limit.
- The production monitoring scheduler is disabled by default and must be explicitly enabled under `agent.scheduler`.
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
- Incident lifecycle persistence is local-only. Task 7 adds single-cycle notification delivery. Step 8 adds read-only localhost API, local report preview, and systemd readiness/watchdog wiring. Step 9 adds the disabled-by-default scheduler and run loop; remediation, report delivery, public API, updater behavior, and dashboards are not implemented.
- Notification delivery is disabled and dry-run by default. Webhook destinations are referenced by environment variable name, not raw URL.
- Alpha install keeps scheduler disabled by default and installs configs as `0640`; secret environment files should be `0600`.

## Rules

Each rule has a stable `id`, `collector`, `metric` or `event_category`, optional `target`, threshold blocks, `recover_for`, and explicit missing/stale-data policies. Supported threshold operators are numeric comparison, equality/inequality, boolean true/false, state match, and event-category match. The configuration rejects duplicate IDs, unknown operators, unsafe targets, unsupported metric-name shapes, secret-bearing text, excessive summaries, and excessive rule counts.

## Notifications

Task 7 notification delivery uses `notify.enabled`, `notify.dry_run`, and `notify.receivers`. Receiver destinations are referenced through environment variable names such as `POOLY_WEBHOOK_URL`. Enabled webhook receivers require `url_env`; raw URLs are intentionally absent from the example configuration. Dry-run mode renders safe delivery results without contacting receivers or updating `last_alerted`.

## API, Reports, Scheduler, And systemd

Step 8 and Step 9 use:

- `api.enabled`
- `api.listen`
- `api.allow_non_loopback`
- `api.read_timeout`
- `api.write_timeout`
- `api.shutdown_timeout`
- `reports.enabled`
- `reports.max_incidents`
- `reports.include_resolved`
- `agent.scheduler.enabled`
- `agent.scheduler.interval`
- `agent.scheduler.run_on_start`
- `agent.scheduler.cycle_timeout`
- `agent.scheduler.max_consecutive_failures`
- `systemd.notify`
- `systemd.watchdog`
- `systemd.watchdog_interval`

The API is read-only and returns JSON. Reports are previews generated from existing storage only. The scheduler runs collection/evaluation/notification cycles only when explicitly enabled. systemd readiness is sent only after config, logging, storage, the enabled API, and the enabled scheduler are initialized.

See `docs/scheduler.md` for scheduler behavior and dry-run diagnostics.

For alpha install and rollback, see `docs/alpha-install.md`, `docs/alpha-uninstall.md`, and `docs/local-dry-run.md`.

See `docs/config.example.yaml`.
