# Notification Security

Notification payloads include only safe incident fields:

- incident ID
- fingerprint after redaction
- node ID
- type
- target
- condition
- severity
- status
- safe summary
- timestamps
- occurrence count
- safe local evidence path when present

Payloads do not include raw journal messages, command output, webhook URLs, tokens, passwords, private keys, authorization headers, environment dumps, or file contents.

Webhook URLs are read from environment variables and kept in memory only for the outbound request. They are not printed, logged, or stored in delivery history.

Webhook response bodies are read with a fixed upper bound and redacted before being stored as error summaries.

Dry-run mode does not contact receivers and does not update `last_alerted`.
