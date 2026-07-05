# Notification Configuration

Task 7 uses the `notify` configuration block.

```yaml
notify:
  enabled: false
  dry_run: true
  receivers:
    - id: local-webhook
      display_name: "Local webhook"
      enabled: false
      type: webhook
      url_env: POOLY_WEBHOOK_URL
      timeout: 5s
      events:
        - opened
        - escalated
        - resolved
      severities:
        - warning
        - failure
        - critical
```

Webhook destinations are referenced by environment variable name. Raw webhook URLs do not belong in YAML.

Supported receiver types:

- `webhook`
- `noop`

Supported event filters:

- `opened`
- `escalated`
- `resolved`
- `reopened`

Supported severity filters:

- `warning`
- `failure`
- `critical`
- `none`

HTTP webhook URLs must use HTTPS. HTTP is accepted only for explicitly allowed local testing through `allow_insecure_local: true`.

The legacy top-level `notification` and `receivers` blocks remain present for earlier task compatibility, but Task 7 delivery uses `notify`.
