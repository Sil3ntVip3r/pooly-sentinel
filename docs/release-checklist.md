# Release Checklist

Step 11 release checks require a preinstalled `govulncheck`. The script fails with an install instruction if it is missing and does not auto-install tools.

Install hint:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
```

Run:

```bash
scripts/check-release.sh
```

The script runs:

- `go fmt ./...`
- `go mod tidy`
- `git diff --check`
- `go vet ./...`
- `govulncheck ./...`
- `go test ./...`
- `go test -race ./...`
- `go test -cover ./...`
- CGO-free native build
- CGO-free Linux `amd64` and `arm64` builds
- safe CLI checks against a temporary config
- secret-pattern scan
- local end-to-end dry-run

The script does not require root, live systemd, external network access, production state paths, production log paths, or real notification receivers. It expects Go module dependencies and `govulncheck` to already be available locally.

Before tagging an alpha:

1. Confirm `git status --short` is clean.
2. Run `scripts/check-release.sh`.
3. Review `docs/config.example.yaml` for safe defaults.
4. Verify `scripts/scan-secrets.sh`.
5. Build the target binary with `CGO_ENABLED=0`.
6. Run `scripts/install.sh --dry-run`.
7. Review the installed config manually before enabling the service.

Deferred from alpha release readiness: remediation, updater, dashboard, public API, report delivery, new collectors, new notification receivers, and remote fleet hub.
