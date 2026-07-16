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
export HOSTFORGE_RAILPACK_ENABLED="true"
export HOSTFORGE_RAILPACK_BIN="${HOSTFORGE_LOCAL_RAILPACK_BIN:-railpack}"
export HOSTFORGE_RAILPACK_VERSION="${HOSTFORGE_LOCAL_RAILPACK_VERSION:-v0.23.0}"
export HOSTFORGE_RAILPACK_FRONTEND_IMAGE="${HOSTFORGE_LOCAL_RAILPACK_FRONTEND_IMAGE:-ghcr.io/railwayapp/railpack-frontend@sha256:ba4c430961d9ee3215c64807727a4b11e2198daac31250e9db9eaf9cee4624d6}"
export HOSTFORGE_BUILDKIT_BIN="${HOSTFORGE_LOCAL_BUILDKIT_BIN:-buildctl}"
export HOSTFORGE_BUILDKIT_ADDRESS="${HOSTFORGE_LOCAL_BUILDKIT_ADDRESS:-unix:///run/buildkit/buildkitd.sock}"
export HOSTFORGE_RAILPACK_ARTIFACTS_DIR="${LOCAL_ROOT}/railpack"
export HOSTFORGE_RAILPACK_BUILD_CONCURRENCY="${HOSTFORGE_LOCAL_RAILPACK_BUILD_CONCURRENCY:-1}"
export HOSTFORGE_RAILPACK_MIN_FREE_DISK_BYTES="${HOSTFORGE_LOCAL_RAILPACK_MIN_FREE_DISK_BYTES:-1073741824}"
export HOSTFORGE_BOOTSTRAP_ENABLED="false"
export HOSTFORGE_SYNC_CADDY="false"
export HOSTFORGE_CADDY_CERT_POLL_INTERVAL_SEC="0"

for tool in "${HOSTFORGE_RAILPACK_BIN}" "${HOSTFORGE_BUILDKIT_BIN}" docker; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "error: required local deployment tool '${tool}' is not installed in WSL." >&2
    exit 1
  fi
done

cd "${ROOT_DIR}"
echo "starting HostForge backend on ${HOSTFORGE_LISTEN}"
echo "using local data dir: ${HOSTFORGE_DATA_DIR}"
echo "using Railpack ${HOSTFORGE_RAILPACK_VERSION} with BuildKit at ${HOSTFORGE_BUILDKIT_ADDRESS}"
echo "login username: admin"
echo "login password: ${HOSTFORGE_API_TOKEN}"
go run ./cmd/server
