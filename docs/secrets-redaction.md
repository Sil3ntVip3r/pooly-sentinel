# Secrets And Redaction

Pooly Sentinel must not store, print, log, or expose webhook URLs, tokens, passwords, private keys, authorization headers, raw environment secrets, raw journal messages, raw command output, file contents, or private evidence.

Configuration examples use environment variable names only:

```yaml
url_env: POOLY_WEBHOOK_URL
```

Put actual secret values in `/etc/pooly-sentinel/pooly-sentinel.env` or another operator-managed secret channel. The install helper creates the env file with `0600` permissions when absent, but does not write secret values.

## Redaction Coverage

Step 11 redacts Discord webhook URLs for `discord.com`, `discordapp.com`, `canary.discord.com`, `ptb.discord.com`, `canary.discordapp.com`, and `ptb.discordapp.com`. It also includes a conservative generic webhook-token URL pattern for webhook paths with token-shaped segments. Ordinary documentation links such as webhook setup pages are not redacted merely because they contain the word `webhook`.

Private-key blocks and authorized-key contents remain redacted. Already-redacted evidence paths are treated as unsafe metadata and are not exposed through API or notification payloads.

## Secret Scan Helper

Run:

```bash
scripts/scan-secrets.sh
```

The helper scans tracked source, docs, scripts, and service files for obvious forbidden patterns:

- Discord webhook URL literals, including canary/PTB domains
- `Authorization: Bearer` literals
- private-key block headers
- raw webhook URL assignments
- tracked `.env` files

It reports file names only, not matching lines, so a future accidental secret is not echoed back to the terminal.

This scan is a guardrail, not a replacement for careful review.
