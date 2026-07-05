# Storage Schema

Schema ownership lives in versioned Go migration definitions under `internal/storage/migrations.go`.

## Tables

`schema_migrations`

- `version INTEGER PRIMARY KEY`
- `name TEXT NOT NULL`
- `applied_at TEXT NOT NULL`

`metadata`

- key/value records for small local state facts

`collector_state`

- keyed by `collector` and `target`
- stores status, JSON state, timestamps, and redacted error summaries

`incidents`

- stores local incident lifecycle records
- includes stable fingerprint and last-transition fields added for Task 6
- Task 6 does not send notifications

`notification_deliveries`

- stores Task 7 delivery attempts
- records receiver, status, attempt, attempted time, delivered time, and redacted error details

`rollup_metadata`

- minimal groundwork for future rollup tables
- Task 3 does not aggregate resources

`rule_evaluation_state`

- keyed by `rule_id` and `target`
- stores pending/recovery state, severity, condition timing, last evaluation time, last observed time, and safe summaries
- supports deterministic sustained-duration behavior across agent restarts

## Timestamp Policy

Timestamps are stored as UTC RFC3339Nano text. Nullable timestamps are scanned explicitly.
