# Local Dry Run

Use the local dry-run script before installing or enabling the alpha service:

```bash
CGO_ENABLED=0 go build ./cmd/pooly-agent
scripts/local-dry-run.sh --binary ./pooly-agent
```

The script creates temporary state and log directories, writes a temporary safe config, and runs:

- config validation
- storage doctor
- API construction check
- rule validation
- fixture-based rule and incident evaluation
- incident listing from temporary SQLite state
- noop notification dry-run
- report preview
- scheduler status
- scheduler run-once dry-run

It does not touch `/var/lib/pooly-sentinel`, `/var/log/pooly-sentinel`, systemd, network receivers, real webhook URLs, or host remediation.

The fixture is synthetic so macOS development machines and Linux nodes can run the same validation without depending on live `/proc`, journald, SSH, or systemd state.

Known limitation: this validates the local pipeline and storage behavior, not production host collector correctness on a real Pooly node.
