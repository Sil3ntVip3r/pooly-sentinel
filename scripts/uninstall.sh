#!/usr/bin/env bash
set -euo pipefail

DRY_RUN=0
STOP_SERVICE=0
DISABLE_SERVICE=0
REMOVE_BINARY=0
REMOVE_SERVICE=0
PURGE_STATE=0
PURGE_LOGS=0
CONFIRM_STATE=0
CONFIRM_LOGS=0

PREFIX="/usr/local"
ETC_DIR="/etc/pooly-sentinel"
STATE_DIR="/var/lib/pooly-sentinel"
LOG_DIR="/var/log/pooly-sentinel"
SYSTEMD_DIR="/etc/systemd/system"
CONFIG_PATH=""

usage() {
  cat <<'USAGE'
Usage: scripts/uninstall.sh [options]

Default behavior is conservative: no files, configs, state, logs, or evidence are removed.

Options:
  --dry-run                         Print safe actions without changing the host.
  --stop-service                    Stop pooly-sentinel-agent.service.
  --disable-service                 Disable pooly-sentinel-agent.service.
  --remove-binary                   Remove installed pooly-agent binary.
  --remove-service                  Remove installed systemd service file.
  --purge-state --confirm-purge-state
                                    Delete state directory, including SQLite state. Never default.
  --purge-logs --confirm-purge-logs Delete log directory. Never default.
  --prefix <dir>                    Binary prefix (default: /usr/local).
  --config <path>                   Config path for reporting only.
  --etc-dir <dir>                   Config directory (default: /etc/pooly-sentinel).
  --state-dir <dir>                 State directory (default: /var/lib/pooly-sentinel).
  --log-dir <dir>                   Log directory (default: /var/log/pooly-sentinel).
  --systemd-dir <dir>               systemd unit directory (default: /etc/systemd/system).
  --help                            Show this help.
USAGE
}

redact() {
  sed -E \
    -e 's#https://[^[:space:]]*(discord(app)?\.com/api/webhooks|webhook)[^[:space:]]*#[REDACTED_URL]#gi' \
    -e 's#(Authorization:[[:space:]]*Bearer[[:space:]]*)[^[:space:]]+#\1[REDACTED]#gi' \
    -e 's#([?&](access_token|refresh_token|id_token|token|api[_-]?key|apikey|password|secret|signature|auth|authorization)=)[^&[:space:]]+#\1[REDACTED]#gi'
}

say() {
  printf '%s\n' "$*" | redact
}

fail() {
  say "ERROR: $*"
  exit 1
}

run() {
  if [[ "${DRY_RUN}" -eq 1 ]]; then
    say "DRY-RUN: $*"
    return 0
  fi
  "$@"
}

need_value() {
  local flag="$1"
  local value="${2:-}"
  [[ -n "${value}" ]] || fail "${flag} requires a value"
}

trim_trailing_slashes() {
  local value="$1"
  while [[ "${value}" != "/" && "${value}" == */ ]]; do
    value="${value%/}"
  done
  printf '%s' "${value}"
}

path_has_control_chars() {
  local value="$1"
  printf '%s' "${value}" | LC_ALL=C grep -q '[[:cntrl:]]'
}

is_dangerous_path() {
  local value
  value="$(trim_trailing_slashes "$1")"
  case "${value}" in
    /|/etc|/usr|/var|/bin|/sbin|/lib|/lib64)
      return 0
      ;;
  esac
  return 1
}

validate_absolute_path() {
  local label="$1"
  local value="$2"
  [[ -n "${value}" ]] || fail "${label} must not be empty"
  if path_has_control_chars "${value}"; then
    fail "${label} must not contain control characters"
  fi
  case "${value}" in
    /*) ;;
    *) fail "${label} must be an absolute path" ;;
  esac
  if is_dangerous_path "${value}"; then
    fail "${label} points at a dangerous broad system path: ${value}"
  fi
}

path_depth() {
  local value
  value="$(trim_trailing_slashes "$1")"
  value="${value#/}"
  local count=0
  local part
  IFS='/' read -r -a parts <<< "${value}"
  for part in "${parts[@]}"; do
    [[ -n "${part}" ]] && count=$((count + 1))
  done
  printf '%s' "${count}"
}

validate_purge_path() {
  local label="$1"
  local value="$2"
  validate_absolute_path "${label}" "${value}"
  local trimmed
  trimmed="$(trim_trailing_slashes "${value}")"
  case "${trimmed}" in
    /var/log|/var/lib|/etc/pooly-sentinel|/etc/systemd|/usr/local)
      fail "${label} is too broad to purge safely: ${value}"
      ;;
  esac
  if [[ "$(path_depth "${value}")" -lt 3 ]]; then
    fail "${label} is too shallow to purge safely: ${value}"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    --stop-service) STOP_SERVICE=1 ;;
    --disable-service) DISABLE_SERVICE=1 ;;
    --remove-binary) REMOVE_BINARY=1 ;;
    --remove-service) REMOVE_SERVICE=1 ;;
    --purge-state) PURGE_STATE=1 ;;
    --purge-logs) PURGE_LOGS=1 ;;
    --confirm-purge-state) CONFIRM_STATE=1 ;;
    --confirm-purge-logs) CONFIRM_LOGS=1 ;;
    --prefix) need_value "$1" "${2:-}"; PREFIX="$2"; shift ;;
    --config) need_value "$1" "${2:-}"; CONFIG_PATH="$2"; shift ;;
    --etc-dir) need_value "$1" "${2:-}"; ETC_DIR="$2"; shift ;;
    --state-dir) need_value "$1" "${2:-}"; STATE_DIR="$2"; shift ;;
    --log-dir) need_value "$1" "${2:-}"; LOG_DIR="$2"; shift ;;
    --systemd-dir) need_value "$1" "${2:-}"; SYSTEMD_DIR="$2"; shift ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown option $1" ;;
  esac
  shift
done

BIN_PATH="${PREFIX%/}/bin/pooly-agent"
SERVICE_PATH="${SYSTEMD_DIR%/}/pooly-sentinel-agent.service"
CONFIG_PATH="${CONFIG_PATH:-${ETC_DIR%/}/config.yaml}"
ENV_PATH="${ETC_DIR%/}/pooly-sentinel.env"

validate_absolute_path "--prefix" "${PREFIX}"
validate_absolute_path "--etc-dir" "${ETC_DIR}"
validate_absolute_path "--state-dir" "${STATE_DIR}"
validate_absolute_path "--log-dir" "${LOG_DIR}"
validate_absolute_path "--systemd-dir" "${SYSTEMD_DIR}"
validate_absolute_path "--config" "${CONFIG_PATH}"
validate_absolute_path "environment path" "${ENV_PATH}"
validate_absolute_path "binary target" "${BIN_PATH}"
validate_absolute_path "service target" "${SERVICE_PATH}"

if [[ "${PURGE_STATE}" -eq 1 ]]; then
  validate_purge_path "--state-dir" "${STATE_DIR}"
fi
if [[ "${PURGE_LOGS}" -eq 1 ]]; then
  validate_purge_path "--log-dir" "${LOG_DIR}"
fi

if [[ "${PURGE_STATE}" -eq 1 && "${CONFIRM_STATE}" -ne 1 ]]; then
  fail "--purge-state requires --confirm-purge-state"
fi
if [[ "${PURGE_LOGS}" -eq 1 && "${CONFIRM_LOGS}" -ne 1 ]]; then
  fail "--purge-logs requires --confirm-purge-logs"
fi

say "Pooly Sentinel uninstall helper"
say "Config and secret environment files are preserved by default:"
say "  ${CONFIG_PATH}"
say "  ${ENV_PATH}"
say "State and evidence are preserved by default:"
say "  ${STATE_DIR}"
say "Logs are preserved by default:"
say "  ${LOG_DIR}"

if [[ "${STOP_SERVICE}" -eq 1 ]]; then
  run systemctl stop pooly-sentinel-agent.service
fi
if [[ "${DISABLE_SERVICE}" -eq 1 ]]; then
  run systemctl disable pooly-sentinel-agent.service
fi
if [[ "${REMOVE_SERVICE}" -eq 1 ]]; then
  run rm -f "${SERVICE_PATH}"
  run systemctl daemon-reload
fi
if [[ "${REMOVE_BINARY}" -eq 1 ]]; then
  run rm -f "${BIN_PATH}"
fi
if [[ "${PURGE_STATE}" -eq 1 ]]; then
  say "WARNING: purging state directory, including SQLite state and any local evidence under it: ${STATE_DIR}"
  run rm -rf "${STATE_DIR}"
fi
if [[ "${PURGE_LOGS}" -eq 1 ]]; then
  say "WARNING: purging log directory: ${LOG_DIR}"
  run rm -rf "${LOG_DIR}"
fi

if [[ "${STOP_SERVICE}${DISABLE_SERVICE}${REMOVE_BINARY}${REMOVE_SERVICE}${PURGE_STATE}${PURGE_LOGS}" == "000000" ]]; then
  say "No destructive action selected. Re-run with explicit flags to remove installed components."
fi

say "Uninstall helper complete."
