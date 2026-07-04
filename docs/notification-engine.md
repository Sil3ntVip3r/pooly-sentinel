# Notification Engine

The notification manager will receive incident candidates from the incident engine. Collectors do not notify directly.

## Responsibilities

- routing
- grouping
- dedupe
- silences
- inhibition
- timing
- rate limiting
- delivery
- retry
- history

## Cost Classes

- `free_core`: local files, JSONL events, local reports
- `free_self_hosted`: Gotify, ntfy, future Pooly Hub
- `free_external`: Discord webhooks
- `paid_external`: Pushover, Twilio SMS, AWS SNS SMS

Paid receivers must be disabled by default and must not be required for install, alpha operation, or normal monitoring.

## Receiver Placeholders

Task 1 includes receiver package placeholders only. No receiver sends notifications yet.
