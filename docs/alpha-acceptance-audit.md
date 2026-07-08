# Alpha Acceptance Audit

Step 12 records the documentation-only alpha acceptance audit for Pooly Sentinel. This audit did not change runtime code, tests, scripts, deployment automation, or Pooly Server Guard.

## Audited Commit

- `14d5dbf` `fix: harden alpha security boundaries`
- Full commit: `14d5dbfc8e7e7a981c44f05c0e3d7d288acea8bb`

## Verdict

Pooly Sentinel is ready for alpha acceptance at the audited commit.

No alpha blockers were found, and no runtime remediation was required.

## Verification Passed

The alpha acceptance audit passed the full safe release verification set:

- `go fmt ./...`
- `go mod tidy`
- `git diff --check`
- `go vet ./...`
- `govulncheck ./...`
- `go test ./...`
- `go test -race ./...`
- `go test -cover ./...`
- `CGO_ENABLED=0 go build ./cmd/pooly-agent`
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/pooly-agent`
- `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/pooly-agent`
- `scripts/scan-secrets.sh`
- `scripts/local-dry-run.sh --binary ./pooly-agent`
- `scripts/check-release.sh`

## Vulnerability Scan

`govulncheck ./...` found no called vulnerabilities.

`govulncheck` also reported one vulnerability in required modules that the code does not appear to call. This was treated as a non-blocking release-risk note for alpha.

## Alpha Safety Defaults

- API serving is disabled by default.
- API serving is loopback-only when enabled unless non-loopback binding is explicitly allowed.
- The scheduler is disabled by default.
- Notifications are disabled and dry-run by default.
- Production collector execution requires explicit scheduler enablement and collector configuration. Systemd, journald, SSH, filewatch, and audit collectors are disabled by default.
- Remediation is not implemented.
- Updater behavior is not implemented.
- Dashboard behavior is not implemented.
- Public API exposure is not implemented.
- Report delivery is not implemented.

## Non-Blocking Concerns

- `govulncheck` must be installed and available on `PATH` before release checks run.
- Optional future polish: update the stale health placeholder comment.
- Optional future polish: add a clean-tree assertion after `go fmt ./...` and `go mod tidy` in `scripts/check-release.sh`.

## Alpha Release Risk

Release risk is low for alpha. Operational caution is still required for target-host configuration review, secret environment setup, and final verification in the release environment before tagging.

## Explicit Alpha Limitations

- No remediation.
- No updater.
- No dashboard.
- No public API.
- No report delivery.
- No remote fleet hub.
- No production deployment automation.
- Alpha release still requires final tagging and release steps later.
