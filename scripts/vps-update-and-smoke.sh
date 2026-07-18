#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="${HF_REPO_DIR:-/opt/hostforge}"
ENV_FILE="${HF_ENV_FILE:-/etc/hostforge/hostforge.env}"
SERVICE="${HF_SERVICE_NAME:-hostforge-server}"
REF="${HF_UPDATE_REF:-main}"
HF_SERVER_URL="${HF_SERVER_URL:-}"
HF_LOCAL_SERVER_URL="${HF_LOCAL_SERVER_URL:-}"

read_env_value() {
  local key="$1"
  local value
  value="$(tr -d '\r' <"${ENV_FILE}" | awk -v key="${key}" '
    $0 ~ "^[[:space:]]*" key "=" {
      line=$0
      sub("^[[:space:]]*" key "=", "", line)
      print line
      exit
    }
  ')"
  if [[ "${value}" == \"*\" && "${value}" == *\" ]]; then
    value="${value:1:${#value}-2}"
  elif [[ "${value}" == \'*\' && "${value}" == *\' ]]; then
    value="${value:1:${#value}-2}"
  fi
  printf '%s' "${value}"
}

json_platform_domain() {
  local file="$1"
  if [[ "${json_tool}" == "jq" ]]; then
    jq -r '.onboarding.platform_domain // empty' "${file}"
  else
    python3 -c 'import json,sys; data=json.load(open(sys.argv[1], encoding="utf-8")); print((data.get("onboarding") or {}).get("platform_domain") or "")' "${file}"
  fi
}

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
if command -v jq >/dev/null 2>&1; then
  json_tool="jq"
elif command -v python3 >/dev/null 2>&1; then
  json_tool="python3"
else
  echo "error: jq or python3 is required to read the saved platform domain" >&2
  exit 1
fi
if [[ ! -d "${REPO_DIR}/.git" ]]; then
  echo "error: ${REPO_DIR} is not a Git checkout" >&2
  exit 1
fi
if [[ ! -r "${ENV_FILE}" ]]; then
  echo "error: cannot read ${ENV_FILE}" >&2
  exit 1
fi

api_token="$(read_env_value HOSTFORGE_API_TOKEN)"
if [[ -z "${api_token}" ]]; then
  echo "error: ${ENV_FILE} does not define HOSTFORGE_API_TOKEN" >&2
  exit 1
fi

if [[ -z "${HF_LOCAL_SERVER_URL}" ]]; then
  listen="$(read_env_value HOSTFORGE_LISTEN)"
  listen="${listen:-127.0.0.1:8080}"
  case "${listen}" in
    0.0.0.0:*) HF_LOCAL_SERVER_URL="http://127.0.0.1:${listen##*:}" ;;
    \[::\]:*) HF_LOCAL_SERVER_URL="http://[::1]:${listen##*:}" ;;
    :*) HF_LOCAL_SERVER_URL="http://127.0.0.1:${listen##*:}" ;;
    *) HF_LOCAL_SERVER_URL="http://${listen}" ;;
  esac
fi

onboarding_body="$(mktemp "${TMPDIR:-/tmp}/hostforge-onboarding.XXXXXX")"
cleanup() {
  rm -f "${onboarding_body}"
}
trap cleanup EXIT

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

echo "Waiting for the local HostForge API at ${HF_LOCAL_SERVER_URL%/}..."
local_ready=0
for _ in $(seq 1 30); do
  if curl --silent --show-error --fail --output "${onboarding_body}" --max-time 5 \
    -H "Accept: application/json" \
    -H "Authorization: Bearer ${api_token}" \
    "${HF_LOCAL_SERVER_URL%/}/api/onboarding"; then
    local_ready=1
    break
  fi
  sleep 2
done
if [[ "${local_ready}" -ne 1 ]]; then
  echo "error: local HostForge API did not become ready" >&2
  journalctl -u "${SERVICE}" -n 100 --no-pager >&2 || true
  echo "previous commit: ${previous_commit}" >&2
  exit 1
fi

if [[ -z "${HF_SERVER_URL}" ]]; then
  if ! platform_domain="$(json_platform_domain "${onboarding_body}")"; then
    echo "error: the local onboarding response was not valid JSON" >&2
    exit 1
  fi
  if [[ -z "${platform_domain}" ]]; then
    echo "error: onboarding has no saved platform domain; set HF_SERVER_URL for this update" >&2
    exit 1
  fi
  HF_SERVER_URL="https://${platform_domain}"
  echo "Using the saved onboarding domain: ${HF_SERVER_URL}"
fi
case "${HF_SERVER_URL}" in
  http://*|https://*) ;;
  *)
    echo "error: HF_SERVER_URL must be an http:// or https:// origin" >&2
    exit 1
    ;;
esac

echo "Waiting for ${HF_SERVER_URL%/}..."
public_ready=0
for _ in $(seq 1 30); do
  if curl --silent --show-error --fail --output /dev/null --max-time 5 "${HF_SERVER_URL%/}/"; then
    public_ready=1
    break
  fi
  sleep 2
done
if [[ "${public_ready}" -ne 1 ]]; then
  echo "error: public HostForge origin did not become ready" >&2
  journalctl -u "${SERVICE}" -n 100 --no-pager >&2 || true
  echo "previous commit: ${previous_commit}" >&2
  exit 1
fi

echo "Running HostForge v2 API acceptance smoke..."
HF_TOKEN="${api_token}" \
  HF_SERVER_URL="${HF_SERVER_URL}" \
  ./scripts/v2-staging-api-smoke.sh
echo "Running managed database and gateway isolation audit..."
./scripts/database-services-vps-audit.sh

unset api_token

systemctl --no-pager --full status "${SERVICE}"
echo "HostForge update and smoke complete: ${previous_commit} -> ${current_commit}"
