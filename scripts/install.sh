#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

DRY_RUN=0
FORCE=0
ENABLE_SERVICE=0
START_SERVICE=0

PREFIX="/usr/local"
ETC_DIR="/etc/pooly-sentinel"
STATE_DIR="/var/lib/pooly-sentinel"
LOG_DIR="/var/log/pooly-sentinel"
SYSTEMD_DIR="/etc/systemd/system"
BINARY_SOURCE="${REPO_ROOT}/pooly-agent"
CONFIG_PATH=""
ENV_PATH=""

usage() {
  cat <<'USAGE'
Usage: scripts/install.sh [options]

Options:
  --dry-run             Print safe actions without writing files or running systemctl.
  --force               Allow overwriting an existing config with the example config.
  --enable-service      Run systemctl enable after installing the service.
  --start-service       Run systemctl start after installing the service.
  --prefix <dir>        Install binary under <dir>/bin (default: /usr/local).
  --binary <path>       Built pooly-agent binary to install (default: ./pooly-agent).
  --config <path>       Installed config path (default: /etc/pooly-sentinel/config.yaml).
  --etc-dir <dir>       Configuration directory (default: /etc/pooly-sentinel).
  --state-dir <dir>     State directory (default: /var/lib/pooly-sentinel).
  --log-dir <dir>       Log directory (default: /var/log/pooly-sentinel).
  --systemd-dir <dir>   systemd unit directory (default: /etc/systemd/system).
  --help                Show this help.
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

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    --force) FORCE=1 ;;
    --enable-service) ENABLE_SERVICE=1 ;;
    --start-service) START_SERVICE=1 ;;
    --prefix) need_value "$1" "${2:-}"; PREFIX="$2"; shift ;;
    --binary) need_value "$1" "${2:-}"; BINARY_SOURCE="$2"; shift ;;
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

BIN_DIR="${PREFIX%/}/bin"
BIN_PATH="${BIN_DIR}/pooly-agent"
CONFIG_PATH="${CONFIG_PATH:-${ETC_DIR%/}/config.yaml}"
ENV_PATH="${ENV_PATH:-${ETC_DIR%/}/pooly-sentinel.env}"
SERVICE_SOURCE="${REPO_ROOT}/systemd/pooly-sentinel-agent.service"
SERVICE_PATH="${SYSTEMD_DIR%/}/pooly-sentinel-agent.service"
EXAMPLE_CONFIG="${REPO_ROOT}/docs/config.example.yaml"

[[ -f "${BINARY_SOURCE}" ]] || fail "built binary not found at ${BINARY_SOURCE}; run go build ./cmd/pooly-agent first"
[[ -x "${BINARY_SOURCE}" ]] || fail "binary is not executable: ${BINARY_SOURCE}"
[[ -f "${SERVICE_SOURCE}" ]] || fail "systemd service template missing: ${SERVICE_SOURCE}"
[[ -f "${EXAMPLE_CONFIG}" ]] || fail "example config missing: ${EXAMPLE_CONFIG}"

"${BINARY_SOURCE}" version >/dev/null || fail "binary could not run version"

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

INSTALL_CONFIG="${TMP_DIR}/config.yaml"
awk -v state="${STATE_DIR}" -v logdir="${LOG_DIR}" '
  /^  state_dir:/ { print "  state_dir: " state; next }
  /^  log_dir:/ { print "  log_dir: " logdir; next }
  { print }
' "${EXAMPLE_CONFIG}" > "${INSTALL_CONFIG}"

INSTALL_SERVICE="${TMP_DIR}/pooly-sentinel-agent.service"
awk -v bin="${BIN_PATH}" -v cfg="${CONFIG_PATH}" -v env="${ENV_PATH}" '
  {
    gsub("/usr/local/bin/pooly-agent", bin)
    gsub("/etc/pooly-sentinel/config.yaml", cfg)
    gsub("-/etc/pooly-sentinel/pooly-sentinel.env", "-" env)
    print
  }
' "${SERVICE_SOURCE}" > "${INSTALL_SERVICE}"

"${BINARY_SOURCE}" check config --config "${INSTALL_CONFIG}" >/dev/null || fail "example config failed validation"

say "Installing Pooly Sentinel alpha files"
say "Binary: ${BIN_PATH}"
say "Config: ${CONFIG_PATH}"
say "Service: ${SERVICE_PATH}"
say "State directory: ${STATE_DIR}"
say "Log directory: ${LOG_DIR}"

run install -d -m 0755 "${BIN_DIR}"
run install -d -m 0750 "${ETC_DIR}" "${STATE_DIR}" "${LOG_DIR}"
run install -d -m 0755 "${SYSTEMD_DIR}"
run install -m 0755 "${BINARY_SOURCE}" "${BIN_PATH}"

if [[ -e "${CONFIG_PATH}" && "${FORCE}" -ne 1 ]]; then
  say "Existing config preserved: ${CONFIG_PATH}"
else
  if [[ -e "${CONFIG_PATH}" && "${FORCE}" -eq 1 ]]; then
    say "Replacing existing config because --force was provided"
  fi
  run install -m 0640 "${INSTALL_CONFIG}" "${CONFIG_PATH}"
fi

if [[ -e "${ENV_PATH}" ]]; then
  run chmod 0600 "${ENV_PATH}"
  say "Existing environment file permissions checked: ${ENV_PATH}"
else
  if [[ "${DRY_RUN}" -eq 1 ]]; then
    say "DRY-RUN: create empty secret environment file ${ENV_PATH} with mode 0600"
  else
    umask 077
    : > "${ENV_PATH}"
    chmod 0600 "${ENV_PATH}"
  fi
fi

SERVICE_CHANGED=0
if [[ ! -e "${SERVICE_PATH}" ]] || ! cmp -s "${INSTALL_SERVICE}" "${SERVICE_PATH}"; then
  SERVICE_CHANGED=1
  run install -m 0644 "${INSTALL_SERVICE}" "${SERVICE_PATH}"
else
  say "Service file already up to date"
fi

if [[ "${DRY_RUN}" -eq 0 ]]; then
  "${BIN_PATH}" check config --config "${CONFIG_PATH}" >/dev/null || fail "installed config failed validation"
else
  say "DRY-RUN: validate installed config with ${BIN_PATH} check config --config ${CONFIG_PATH}"
fi

if [[ "${SERVICE_CHANGED}" -eq 1 ]]; then
  run systemctl daemon-reload
fi
if [[ "${ENABLE_SERVICE}" -eq 1 ]]; then
  run systemctl enable pooly-sentinel-agent.service
fi
if [[ "${START_SERVICE}" -eq 1 ]]; then
  run systemctl start pooly-sentinel-agent.service
fi

say "Install complete."
say "Next steps:"
say "  1. Review ${CONFIG_PATH}; scheduler remains disabled until explicitly enabled."
say "  2. Put secret environment values in ${ENV_PATH} without printing them."
say "  3. Validate with: ${BIN_PATH} check config --config ${CONFIG_PATH}"
say "  4. Start later with: systemctl start pooly-sentinel-agent.service"
