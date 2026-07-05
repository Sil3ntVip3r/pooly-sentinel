#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

BINARY="${REPO_ROOT}/pooly-agent"
BINARY_EXPLICIT=0

usage() {
  cat <<'USAGE'
Usage: scripts/local-dry-run.sh [--binary <path>]

Runs a local end-to-end dry-run with temporary state/log directories.
No systemd, root privileges, external network, production paths, or real notifications are required.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary)
      [[ -n "${2:-}" ]] || { printf '%s\n' "ERROR: --binary requires a path"; exit 1; }
      BINARY="$2"
      BINARY_EXPLICIT=1
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'ERROR: unknown option %s\n' "$1"
      exit 1
      ;;
  esac
  shift
done

if [[ ! -x "${BINARY}" ]]; then
  if [[ "${BINARY_EXPLICIT}" -eq 1 ]]; then
    printf 'ERROR: binary is not executable: %s\n' "${BINARY}"
    exit 1
  fi
  printf '%s\n' "Built binary not found at ${BINARY}; running CGO-free local build"
  CGO_ENABLED=0 go build ./cmd/pooly-agent
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

CONFIG="${TMP_DIR}/config.yaml"
FIXTURE="${TMP_DIR}/observations.json"
STATE_DIR="${TMP_DIR}/state"
LOG_DIR="${TMP_DIR}/log"
mkdir -p "${STATE_DIR}" "${LOG_DIR}"

cat > "${CONFIG}" <<EOF_CONFIG
version: "1"

node:
  id: "dry-run-node"
  name: "Dry Run Node"
  hostname: "dry-run-host"
  region: "local"
  role: "alpha-test"
  environment: "test"
  ring: "alpha"

api:
  enabled: false
  listen: "127.0.0.1:9587"
  allow_non_loopback: false

reports:
  enabled: true
  max_incidents: 25
  include_resolved: true

agent:
  scheduler:
    enabled: false
    interval: 60s
    run_on_start: false
    cycle_timeout: 45s
    max_consecutive_failures: 5

logging:
  level: "info"
  format: "text"

rules:
  - id: dry-run-memory-low
    enabled: true
    collector: resources
    metric: pooly_memory_available_ratio
    target: system
    fail:
      operator: less_than
      value: 0.50
      for: 0s
    recover_for: 0s
    missing_data: stale
    stale_data: stale
    summary: "dry-run memory threshold for {{target}}"

notify:
  enabled: true
  dry_run: true
  receivers:
    - id: dry-run-noop
      display_name: "Dry run noop"
      enabled: true
      type: noop
      timeout: 5s
      events:
        - opened
        - escalated
        - resolved
      severities:
        - warning
        - failure
        - critical

receivers:
  - name: local_file
    type: file
    cost_class: free_core
    enabled: true

notification:
  paid_receivers_enabled_by_default: false

storage:
  state_dir: ${STATE_DIR}
  log_dir: ${LOG_DIR}
  database_file: state.db
  current_metrics_file: metrics-current.json
  sqlite:
    busy_timeout: 5s
    wal: true
EOF_CONFIG

cat > "${FIXTURE}" <<'EOF_FIXTURE'
[
  {
    "collector": "resources",
    "target": "system",
    "timestamp": "2026-07-05T12:00:00Z",
    "duration": 0,
    "success": true,
    "supported": true,
    "summary": "dry-run fixture",
    "metrics": [
      {
        "name": "pooly_memory_available_ratio",
        "value": 0.10,
        "kind": "gauge",
        "unit": "ratio",
        "timestamp": "2026-07-05T12:00:00Z"
      }
    ]
  }
]
EOF_FIXTURE

pass() {
  printf 'PASS %s\n' "$*"
}

run_step() {
  local name="$1"
  shift
  "$@" >/dev/null
  pass "${name}"
}

run_step "config validation" "${BINARY}" check config --config "${CONFIG}"
run_step "storage doctor" "${BINARY}" doctor --config "${CONFIG}"
run_step "api construction" "${BINARY}" api check --config "${CONFIG}"
run_step "rule validation" "${BINARY}" rules validate --config "${CONFIG}"
run_step "fixture rule/incident evaluation" "${BINARY}" rules test --config "${CONFIG}" --fixture "${FIXTURE}" --persist --json
run_step "incident listing" "${BINARY}" incidents list --config "${CONFIG}"
run_step "notification dry-run" "${BINARY}" notifications test --config "${CONFIG}" --receiver dry-run-noop --json --dry-run
run_step "report preview" "${BINARY}" reports preview --config "${CONFIG}" --json
run_step "scheduler status" "${BINARY}" scheduler status --config "${CONFIG}"
run_step "scheduler run-once dry-run" "${BINARY}" scheduler run-once --config "${CONFIG}" --json --dry-run

printf '%s\n' "PASS local end-to-end dry-run complete"
