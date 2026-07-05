# Task 5 Manual CLI

Manual collector commands run one cycle only:

```text
pooly-agent collectors list --config docs/config.example.yaml
pooly-agent collectors run systemd --config docs/config.example.yaml --dry-run
pooly-agent collectors run journal --config docs/config.example.yaml --json --dry-run
pooly-agent collectors run ssh --config docs/config.example.yaml --dry-run
pooly-agent collectors run filewatch --config docs/config.example.yaml --json --dry-run
```

Persistence is disabled unless `--persist` is supplied. Dry-run mode never updates journal cursors or file baselines.

On non-Linux platforms, these commands return unsupported observations instead of panicking or trying Linux-only commands.
