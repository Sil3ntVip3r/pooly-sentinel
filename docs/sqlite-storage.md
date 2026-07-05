# SQLite Storage

Pooly Sentinel uses SQLite for local agent state because it is self-hosted, durable, simple to deploy, and does not require a separate database service.

## Driver ADR

Decision: use `modernc.org/sqlite`.

Rationale:

- it works through Go `database/sql`
- it is CGo-free
- it avoids host compiler and SQLite development package requirements
- it is suitable for a single local agent writing small state records

Rejected alternative:

- `github.com/mattn/go-sqlite3`, because it requires CGo and is explicitly not allowed for this project.

Pinned direct dependency:

- `modernc.org/sqlite v1.34.5`

## Connection Policy

The storage opener centralizes SQLite initialization in `internal/storage/sqlite.go`.

Policy:

- `PRAGMA foreign_keys = ON`: enforce relationships such as notification delivery rows referencing incidents.
- `PRAGMA busy_timeout = 5s` by default: wait briefly for locks instead of failing immediately.
- `PRAGMA journal_mode = WAL` where supported: allow safer append-style journaling and good local-agent behavior.
- `PRAGMA synchronous = NORMAL`: balances durability and write cost for WAL-backed local state. SQLite still preserves consistency; the project can revisit `FULL` for stricter release profiles.
- `SetMaxOpenConns(1)` and `SetMaxIdleConns(1)`: conservative single-agent policy to avoid SQLite lock contention.
- `PingContext`: validates connectivity after opening and after PRAGMA setup.

Task 3 does not start background writers or production monitoring loops.
