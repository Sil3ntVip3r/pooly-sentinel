# Notification Engine

Task 7 implements single-cycle notification delivery for incident lifecycle events. Collectors still do not notify directly.

Implemented responsibilities:

- route by enabled receiver, event filter, and severity filter
- render safe payloads from allowlisted incident fields
- deduplicate repeated successful deliveries for the same lifecycle event
- persist delivery attempts
- update incident `last_alerted` only after successful delivery
- support dry-run and diagnostic CLI commands

Deferred responsibilities:

- production scheduling
- grouping policies
- silence windows
- inhibition graphs
- report delivery
- provider-specific Discord, email, SMS, or paid receivers

Paid receivers remain disabled by default and are not required for install, alpha operation, or normal monitoring.
