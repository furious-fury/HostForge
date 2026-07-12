#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ROOT_DIR}/.hostforge.env"
TMP_ENV="${ROOT_DIR}/.hostforge.wsl.env"
LOCAL_ROOT="${ROOT_DIR}/.hostforge-local"
LOCAL_DATA_DIR="${LOCAL_ROOT}/data"

if ! command -v go >/dev/null 2>&1; then
  echo "error: Linux 'go' is not installed in this WSL environment." >&2
  echo "install Go in WSL first, then rerun this script." >&2
  exit 1
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "error: ${ENV_FILE} not found" >&2
  exit 1
fi

mkdir -p "${LOCAL_ROOT}" "${LOCAL_DATA_DIR}"
tr -d '\r' < "${ENV_FILE}" > "${TMP_ENV}"

set -a
source "${TMP_ENV}"
set +a

# Keep local credentials deterministic instead of inheriting the VPS token.
export HOSTFORGE_API_TOKEN="${HOSTFORGE_LOCAL_API_TOKEN:-admin}"
export HOSTFORGE_SESSION_SECRET="${HOSTFORGE_SESSION_SECRET:-local-dev-session-secret-12345}"
export HOSTFORGE_WEBHOOK_SECRET="${HOSTFORGE_WEBHOOK_SECRET:-local-dev-webhook-secret}"
export HOSTFORGE_ENV_ENCRYPTION_KEY="${HOSTFORGE_ENV_ENCRYPTION_KEY:-MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=}"
export HOSTFORGE_LISTEN="127.0.0.1:8080"
export HOSTFORGE_DATA_DIR="${LOCAL_DATA_DIR}"
export HOSTFORGE_RAILPACK_ENABLED="false"
export HOSTFORGE_BOOTSTRAP_ENABLED="false"
export HOSTFORGE_SYNC_CADDY="false"
export HOSTFORGE_CADDY_CERT_POLL_INTERVAL_SEC="0"

cd "${ROOT_DIR}"
echo "starting HostForge backend on ${HOSTFORGE_LISTEN}"
echo "using local data dir: ${HOSTFORGE_DATA_DIR}"
echo "login username: admin"
echo "login password: ${HOSTFORGE_API_TOKEN}"
go run ./cmd/server
