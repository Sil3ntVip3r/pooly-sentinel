#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

export GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/pooly-sentinel-go-build-cache}"
mkdir -p "${GOCACHE}"

log() {
  printf '==> %s\n' "$*"
}

run() {
  log "$*"
  "$@"
}

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

require_govulncheck() {
  if ! command -v govulncheck >/dev/null 2>&1; then
    fail "govulncheck is required; install it with: go install golang.org/x/vuln/cmd/govulncheck@latest"
  fi
}

require_govulncheck

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

make_temp_config() {
  local source="$1"
  local dest="$2"
  local state_dir="$3"
  local log_dir="$4"
  awk -v state="${state_dir}" -v logdir="${log_dir}" '
    /^  state_dir:/ { print "  state_dir: " state; next }
    /^  log_dir:/ { print "  log_dir: " logdir; next }
    { print }
  ' "${source}" > "${dest}"
}

TEMP_STATE="${TMP_DIR}/state"
TEMP_LOG="${TMP_DIR}/log"
TEMP_CONFIG="${TMP_DIR}/config.yaml"
mkdir -p "${TEMP_STATE}" "${TEMP_LOG}"
make_temp_config "docs/config.example.yaml" "${TEMP_CONFIG}" "${TEMP_STATE}" "${TEMP_LOG}"

run go fmt ./...
run go mod tidy
run git diff --check
run go vet ./...
run govulncheck ./...
run go test ./...
run go test -race ./...
run go test -cover ./...

run env CGO_ENABLED=0 go build ./cmd/pooly-agent
cp ./pooly-agent "${TMP_DIR}/pooly-agent"
run env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${TMP_DIR}/pooly-agent-linux-amd64" ./cmd/pooly-agent
run env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o "${TMP_DIR}/pooly-agent-linux-arm64" ./cmd/pooly-agent

run "${TMP_DIR}/pooly-agent" version
run "${TMP_DIR}/pooly-agent" check config --config "${TEMP_CONFIG}"
run "${TMP_DIR}/pooly-agent" doctor --config "${TEMP_CONFIG}"
run "${TMP_DIR}/pooly-agent" api check --config "${TEMP_CONFIG}"
run "${TMP_DIR}/pooly-agent" rules validate --config "${TEMP_CONFIG}"
run "${TMP_DIR}/pooly-agent" notifications validate --config "${TEMP_CONFIG}"
run "${TMP_DIR}/pooly-agent" scheduler status --config "${TEMP_CONFIG}"
run scripts/scan-secrets.sh
run scripts/local-dry-run.sh --binary "${TMP_DIR}/pooly-agent"

log "release checks passed"
