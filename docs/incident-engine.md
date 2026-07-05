# Incident Engine

Task 6 adds local incident lifecycle persistence driven by rule results.

The incident engine does not evaluate collector thresholds. It receives sustained rule results and updates local incident records.

## Ownership

- Collectors own factual observations.
- The rule engine owns WARN/FAIL/CRITICAL rule-result severity.
- The incident engine owns open, update, escalate, resolve, and reopen lifecycle state.
- Notification delivery remains a future task.

## Deduplication

Incidents are keyed by stable fingerprint. Repeated evaluations of the same active condition update the existing record, increment occurrence count, and refresh `last_seen`. Escalation updates the same incident instead of creating a duplicate.

After resolution, a later recurrence reopens the existing fingerprint record. This deterministic policy keeps incident history queryable without creating uncontrolled duplicate rows.
