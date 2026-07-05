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

- stores incident records for future lifecycle logic
- Task 3 persists records but does not implement rule evaluation or incident transitions

`notification_deliveries`

- stores delivery attempts for future notification history
- Task 3 does not send notifications

`rollup_metadata`

- minimal groundwork for future rollup tables
- Task 3 does not aggregate resources

## Timestamp Policy

Timestamps are stored as UTC RFC3339Nano text. Nullable timestamps are scanned explicitly.
