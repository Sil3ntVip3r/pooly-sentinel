#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

if ! command -v git >/dev/null 2>&1; then
  printf '%s\n' "ERROR: git is required for tracked-file scanning"
  exit 1
fi

failures=0

scan_pattern() {
  local name="$1"
  local pattern="$2"
  shift 2
  local files
  files="$(git grep -IlE "${pattern}" -- "$@" 2>/dev/null || true)"
  if [[ -n "${files}" ]]; then
    printf 'FAIL forbidden pattern detected: %s\n' "${name}"
    printf '%s\n' "${files}" | sed 's/^/  /'
    failures=$((failures + 1))
  else
    printf 'PASS %s\n' "${name}"
  fi
}

tracked_env="$(git ls-files '*.env' '.env' '**/.env' 2>/dev/null || true)"
if [[ -n "${tracked_env}" ]]; then
  printf '%s\n' "FAIL tracked raw environment file detected"
  printf '%s\n' "${tracked_env}" | sed 's/^/  /'
  failures=$((failures + 1))
else
  printf '%s\n' "PASS no tracked raw .env files"
fi

scan_pattern "Discord webhook URL literals" 'https://((canary|ptb)\.)?discord(app)?\.com/api/webhooks/[A-Za-z0-9_./-]+' \
  '*.go' '*.md' '*.yaml' '*.yml' '*.sh' 'systemd/*'

scan_pattern "Authorization Bearer literals" 'Authorization:[[:space:]]*Bearer[[:space:]]+[A-Za-z0-9._~+/-]+' \
  '*.go' '*.md' '*.yaml' '*.yml' '*.sh' 'systemd/*'

scan_pattern "private key blocks" '-----BEGIN [A-Z ]*PRIVATE KEY-----' \
  '*.go' '*.md' '*.yaml' '*.yml' '*.sh' 'systemd/*'

scan_pattern "raw webhook URL assignments" '(webhook|url)[A-Za-z0-9_.-]*:[[:space:]]*https?://[^[:space:]]+' \
  '*.md' '*.yaml' '*.yml' '*.sh'

if [[ "${failures}" -ne 0 ]]; then
  printf 'Secret scan failed with %d finding group(s).\n' "${failures}"
  exit 1
fi

printf '%s\n' "PASS secret scan complete"
