# Configuration

Pooly Sentinel uses YAML configuration with strict decoding and validation. Unknown fields are rejected where the YAML decoder can identify them.

The current supported configuration version is `1`.

## Secret Handling

Configuration should reference secrets through environment variable names only. Do not place webhook URLs, API keys, passwords, tokens, private keys, or authorized-key contents directly in YAML.

The example config uses `POOLY_DISCORD_WEBHOOK` as an environment variable name only; it is not a secret value.

## Safe Defaults

- API binds to `127.0.0.1:9587`.
- Logging defaults to text at info level.
- Production collectors are disabled in the Task 2 foundation.
- Local file receiver is enabled as the free-core receiver.
- Paid receivers are disabled and validation rejects enabled paid receivers.
- SQLite uses a bounded busy timeout and WAL by default where supported.
- Storage filenames must be plain filenames, not paths.

See `docs/config.example.yaml`.
