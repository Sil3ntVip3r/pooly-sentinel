# Reporting

Step 8 adds local report preview generation from existing storage only. Step 9 includes safe scheduler status in report summaries when available.

Reports can be previewed with:

```bash
pooly-agent reports preview --config docs/config.example.yaml
pooly-agent reports preview --config docs/config.example.yaml --json
```

The API also exposes the same safe summary at:

```text
GET /reports/summary
```

## Summary Contents

The current report summary includes:

- generation timestamp
- storage availability
- schema version
- incident counts by status
- open incidents by severity
- recent resolved incident summaries when enabled
- notification delivery counts by status
- scheduler status
- known limitations

All incident and delivery fields are redacted. Reports do not include raw journal messages, command output, file contents, webhook URLs, tokens, passwords, private keys, source IPs, MAC addresses, usernames, or evidence contents.

## Deferred Work

Step 9 does not implement scheduled reports, report delivery, PDF generation, email/webhook report delivery, dashboards, external SaaS integrations, or retention cleanup.
