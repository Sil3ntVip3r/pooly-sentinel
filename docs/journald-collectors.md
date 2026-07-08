# journald Collectors

The journald collectors cover:

- `journal.auth`
- `journal.services`
- `journal.kernel`

Data source:

- baseline: `journalctl --no-pager --output=json --lines=1` with stream filters
- established cursor replay: `journalctl --no-pager --output=json --after-cursor <cursor>` with stream filters
- no `--lines=N` is added to established cursor replay
- bounded command stdout/stderr reads and parser `MaxRecords` limits remain in force

Each stream stores its cursor separately in collector state. The first persisted run records a bounded baseline cursor. Later persisted runs read records after that cursor. Cursor corruption, invalidation, rotation, or vacuuming is handled narrowly: only cursor-shaped journalctl failures trigger a stale/reset baseline.

Cursors are saved only after successful complete processing. Timeout, cancellation, malformed JSON, scanner failures, output-limit failures, command truncation, response truncation, and parser `MaxRecords` truncation do not advance the cursor. Dry-run mode never persists cursor state.

Records are normalized into safe categories such as authentication failure, authentication success, invalid user, sudo failure, service start/stop/failure/restart, kernel OOM, storage, network, or generic journal event. These are event categories, not alert severity.

The collector does not emit raw journal dumps, raw `MESSAGE` fields, credentials, tokens, command lines, environment dumps, usernames, or source IPs as metric labels.
