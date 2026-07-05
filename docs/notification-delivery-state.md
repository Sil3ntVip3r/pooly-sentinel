# Notification Delivery State

Task 7 reuses the existing `notification_deliveries` table.

Each real delivery attempt records:

- delivery ID
- incident ID
- receiver ID
- cost class
- status
- attempt number
- attempted time
- delivered time when successful
- error class
- redacted error summary

Duplicate suppression is deterministic. The delivery key is derived from incident ID, receiver ID, event, severity, status, and transition time. Failed attempts do not suppress retry. A prior successful attempt suppresses repeated delivery for the same lifecycle event across process restarts.

Successful delivery inserts the delivery record and updates incident `last_alerted` in one storage transaction. Failed delivery inserts only a failed delivery record. Dry-run delivery does not write delivery state.

No schema migration is required for Task 7.
