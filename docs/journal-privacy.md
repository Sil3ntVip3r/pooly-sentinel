# Journal Privacy

Journal collection is privacy-preserving by default.

Controls:

- machine-readable JSON input only
- maximum records per run
- maximum bytes per command output
- maximum field length
- redaction before summaries or events leave the parser
- allowlisted fields only

Retained safe fields:

- timestamp
- cursor for local state only
- priority
- transport
- systemd unit
- command or executable identifier when safe
- normalized category
- safe summary

Not emitted:

- raw journal dumps
- full `MESSAGE` values
- passwords, tokens, API keys, webhook URLs, private keys, authorized-key contents
- raw command lines or environment dumps
- unbounded user-controlled fields
