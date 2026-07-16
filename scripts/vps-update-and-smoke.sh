#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="${HF_REPO_DIR:-/opt/hostforge}"
ENV_FILE="${HF_ENV_FILE:-/etc/hostforge/hostforge.env}"
SERVICE="${HF_SERVICE_NAME:-hostforge-server}"
REF="${HF_UPDATE_REF:-main}"
: "${HF_SERVER_URL:?set HF_SERVER_URL to the public HostForge management origin}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: run this update helper as root" >&2
  exit 1
fi
for tool in git systemctl curl awk tr; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "error: ${tool} is required" >&2
    exit 1
  fi
done
if [[ ! -d "${REPO_DIR}/.git" ]]; then
  echo "error: ${REPO_DIR} is not a Git checkout" >&2
  exit 1
fi
if [[ ! -r "${ENV_FILE}" ]]; then
  echo "error: cannot read ${ENV_FILE}" >&2
  exit 1
fi

cd "${REPO_DIR}"
current_branch="$(git branch --show-current)"
if [[ "${current_branch}" != "${REF}" ]]; then
  echo "error: ${REPO_DIR} is on ${current_branch:-a detached HEAD}, expected ${REF}" >&2
  exit 1
fi
previous_commit="$(git rev-parse HEAD)"

# Older releases tracked this generated file. Discard only that known artifact.
if git ls-files --error-unmatch web/tsconfig.app.tsbuildinfo >/dev/null 2>&1 &&
  ! git diff --quiet -- web/tsconfig.app.tsbuildinfo; then
  echo "Restoring generated web/tsconfig.app.tsbuildinfo before update..."
  git restore -- web/tsconfig.app.tsbuildinfo
fi

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "error: tracked VPS changes must be reviewed before updating" >&2
  git status --short >&2
  exit 1
fi

echo "Updating ${REPO_DIR} from origin/${REF}..."
git pull --ff-only origin "${REF}"
current_commit="$(git rev-parse HEAD)"

echo "Building and installing commit ${current_commit}..."
./scripts/install.sh --with-systemd

echo "Restarting ${SERVICE}..."
systemctl restart "${SERVICE}"
if ! systemctl is-active --quiet "${SERVICE}"; then
  echo "error: ${SERVICE} did not become active" >&2
  systemctl --no-pager --full status "${SERVICE}" >&2 || true
  echo "previous commit: ${previous_commit}" >&2
  exit 1
fi

echo "Waiting for ${HF_SERVER_URL%/}..."
ready=0
for _ in $(seq 1 30); do
  if curl --silent --show-error --fail --output /dev/null --max-time 5 "${HF_SERVER_URL%/}/"; then
    ready=1
    break
  fi
  sleep 2
done
if [[ "${ready}" -ne 1 ]]; then
  echo "error: public HostForge origin did not become ready" >&2
  journalctl -u "${SERVICE}" -n 100 --no-pager >&2 || true
  echo "previous commit: ${previous_commit}" >&2
  exit 1
fi

api_token="$(tr -d '\r' <"${ENV_FILE}" | awk '/^[[:space:]]*HOSTFORGE_API_TOKEN=/{line=$0; sub(/^[[:space:]]*HOSTFORGE_API_TOKEN=/, "", line); print line; exit}')"
if [[ "${api_token}" == \"*\" && "${api_token}" == *\" ]]; then
  api_token="${api_token:1:${#api_token}-2}"
elif [[ "${api_token}" == \'*\' && "${api_token}" == *\' ]]; then
  api_token="${api_token:1:${#api_token}-2}"
fi
if [[ -z "${api_token}" ]]; then
  echo "error: ${ENV_FILE} does not define HOSTFORGE_API_TOKEN" >&2
  exit 1
fi

echo "Running HostForge v2 API acceptance smoke..."
HF_TOKEN="${api_token}" \
  HF_SERVER_URL="${HF_SERVER_URL}" \
  ./scripts/v2-staging-api-smoke.sh
unset api_token

systemctl --no-pager --full status "${SERVICE}"
echo "HostForge update and smoke complete: ${previous_commit} -> ${current_commit}"
