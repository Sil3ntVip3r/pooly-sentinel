# Storage Migrations

Migrations are deterministic Go definitions embedded in the binary.

Rules:

- versions are monotonically increasing integers
- migrations run in order
- each migration is recorded in `schema_migrations`
- already-applied migrations are skipped
- migration statements run transactionally where SQLite permits it
- startup fails if the database has a schema version newer than the binary supports
- no migration downloads files
- destructive migrations must never occur silently

Initial migration sequence:

1. `metadata`
2. `collector_state`
3. `incidents`
4. `notification_deliveries`
5. `rollup_metadata`
6. `rule_and_incident_state`

Migration 6 adds incident fingerprints, incident last-transition time, and `rule_evaluation_state`. It is additive and does not delete collector state, incident history, or notification-delivery history.

Task 7 uses the existing `notification_deliveries` schema and incident `last_alerted` field. No migration is required.

Task 3 includes tests for fresh migration, reopening an existing database, ordering, rollback on failure, future schema detection, and repeated initialization. Task 6 adds repository coverage for rule evaluation state and fingerprint lookup.
