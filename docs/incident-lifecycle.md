# Incident Lifecycle

Rule state transitions drive incident lifecycle changes:

- `OK` to `PENDING_WARN`: no incident yet
- `PENDING_WARN` to `WARN`: open or update one incident
- `WARN` to `PENDING_FAIL`: keep the existing warning incident
- `PENDING_FAIL` to `FAIL`: escalate the existing incident
- `FAIL` to `CRITICAL`: escalate the existing incident
- `WARN`, `FAIL`, or `CRITICAL` to `RECOVERING`: keep the incident open
- `RECOVERING` to `RECOVERED`: resolve the incident
- `RECOVERED` to `OK`: clear rule state after recovery persistence

`last_alerted` is reserved for the notification task and is not modified by delivery logic in Task 6.

Open incidents are never silently deleted.
