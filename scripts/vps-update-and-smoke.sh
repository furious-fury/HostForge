#!/usr/bin/env bash
# Update a HostForge install to a published release, then smoke-test it.
#
#   sudo ./scripts/vps-update-and-smoke.sh                  # latest release
#   HOSTFORGE_VERSION=v0.9.0 sudo ./scripts/vps-update-and-smoke.sh
#   HF_FROM_SOURCE=1 sudo ./scripts/vps-update-and-smoke.sh # rebuild a checkout
#
# Rolling back is the same operation aimed at the older tag: this script
# prints the version it upgraded from on every failure path, so recovery is
# HOSTFORGE_VERSION=<that> re-run.
set -euo pipefail

REPO_DIR="${HF_REPO_DIR:-/opt/hostforge}"
ENV_FILE="${HF_ENV_FILE:-/etc/hostforge/hostforge.env}"

# A release update replaces REPO_DIR wholesale and deletes what it displaced,
# and REPO_DIR is operator-supplied. Refuse anything that is not clearly a
# dedicated HostForge directory. Kept in step with the copy in uninstall.sh,
# which stays self-contained on purpose.
assert_removable_dir() {
  local dir="$1" name="$2" trimmed
  case "${dir}" in
    /*) ;;
    *)
      echo "error: ${name} must be an absolute path, got '${dir}'" >&2
      exit 2
      ;;
  esac
  case "${dir}" in
    *//*|*/./*|*/../*|*/..)
      echo "error: ${name} must be a normalised path, got '${dir}'" >&2
      exit 2
      ;;
  esac
  trimmed="${dir%/}"
  case "${trimmed#/}" in
    */*) ;;
    *)
      echo "error: ${name} is too close to the filesystem root to replace: '${dir}'" >&2
      exit 2
      ;;
  esac
  case "${trimmed}" in
    /usr/bin|/usr/lib|/usr/local|/usr/sbin|/usr/share|/var/lib|/var/log|/etc/caddy|/home/*/|/root/*/)
      echo "error: ${name} names a shared system directory, refusing to replace: '${dir}'" >&2
      exit 2
      ;;
  esac
}
assert_removable_dir "${REPO_DIR}" HF_REPO_DIR
SERVICE="${HF_SERVICE_NAME:-hostforge-server}"
REF="${HF_UPDATE_REF:-main}"
GITHUB_REPO="furious-fury/HostForge"
FROM_SOURCE="${HF_FROM_SOURCE:-0}"
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

json_gateway_feature_enabled() {
  local file="$1"
  if [[ "${json_tool}" == "jq" ]]; then
    jq -er 'if .feature_enabled == true then "true" elif .feature_enabled == false then "false" else error("feature_enabled is not boolean") end' "${file}"
  else
    python3 -c 'import json,sys; value=json.load(open(sys.argv[1], encoding="utf-8")).get("feature_enabled"); sys.exit("feature_enabled is not boolean") if not isinstance(value, bool) else None; print(str(value).lower())' "${file}"
  fi
}

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: run this update helper as root" >&2
  exit 1
fi
for tool in systemctl curl awk tr tar; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "error: ${tool} is required" >&2
    exit 1
  fi
done
# Only a source update needs git; a release update fetches over HTTPS, and
# the host may legitimately not have git installed at all.
if [[ "${FROM_SOURCE}" == "1" ]] && ! command -v git >/dev/null 2>&1; then
  echo "error: git is required for HF_FROM_SOURCE=1" >&2
  exit 1
fi
if command -v jq >/dev/null 2>&1; then
  json_tool="jq"
elif command -v python3 >/dev/null 2>&1; then
  json_tool="python3"
else
  echo "error: jq or python3 is required to read the saved platform domain" >&2
  exit 1
fi
if [[ "${FROM_SOURCE}" == "1" && ! -d "${REPO_DIR}/.git" ]]; then
  echo "error: HF_FROM_SOURCE=1 requires ${REPO_DIR} to be a Git checkout" >&2
  exit 1
fi
if [[ ! -f "${REPO_DIR}/scripts/install.sh" ]]; then
  echo "error: ${REPO_DIR} does not contain scripts/install.sh" >&2
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
gateway_body="$(mktemp "${TMPDIR:-/tmp}/hostforge-database-gateway.XXXXXX")"
cleanup() {
  rm -f "${onboarding_body}" "${gateway_body}"
}
trap cleanup EXIT

# The version already running is the rollback anchor. It comes from the
# server rather than the filesystem because that is the version actually
# being served; a half-finished earlier update could leave the tree and the
# running binary disagreeing, and the binary is the one users are hitting.
previous_version="$(curl --silent --fail --max-time 5 "${HF_LOCAL_SERVER_URL%/}/api/version" 2>/dev/null |
  sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
previous_version="${previous_version:-unknown}"

resolve_release_tag() {
  if [[ -n "${HOSTFORGE_VERSION:-}" ]]; then
    printf '%s' "${HOSTFORGE_VERSION}"
    return
  fi
  local tag
  tag="$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" |
    sed -n 's/^ *"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
  if [[ -z "${tag}" ]]; then
    echo "error: could not resolve the latest HostForge release from the GitHub API." >&2
    echo "Pin one with HOSTFORGE_VERSION=vX.Y.Z, or update a checkout with HF_FROM_SOURCE=1." >&2
    exit 1
  fi
  printf '%s' "${tag}"
}

if [[ "${FROM_SOURCE}" == "1" ]]; then
  cd "${REPO_DIR}"
  current_branch="$(git branch --show-current)"
  if [[ "${current_branch}" != "${REF}" ]]; then
    echo "error: ${REPO_DIR} is on ${current_branch:-a detached HEAD}, expected ${REF}" >&2
    exit 1
  fi
  if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "error: tracked VPS changes must be reviewed before updating" >&2
    git status --short >&2
    exit 1
  fi
  echo "Updating ${REPO_DIR} from origin/${REF}..."
  git pull --ff-only origin "${REF}"
  target_version="$(git rev-parse --short HEAD)"
  echo "Building and installing ${target_version}..."
  ./scripts/install.sh --with-systemd
else
  target_version="$(resolve_release_tag)"
  echo "Updating ${REPO_DIR} to ${target_version}..."
  # Staged beside REPO_DIR, not in /tmp: the swap below has to be a rename on
  # one filesystem. Across devices mv degrades to copy-then-delete, which can
  # fail halfway and leave no install tree at all.
  parent_dir="$(dirname "${REPO_DIR}")"
  install -d -m 0755 "${parent_dir}"
  tmp_dir="$(mktemp -d "${parent_dir}/.hostforge-update.XXXXXX")"
  old_dir="${REPO_DIR}.replaced.$$"
  if ! curl -fsSL "https://github.com/${GITHUB_REPO}/archive/refs/tags/${target_version}.tar.gz" |
    tar -xz -C "${tmp_dir}"; then
    rm -rf "${tmp_dir}"
    echo "error: could not download the HostForge ${target_version} source archive." >&2
    echo "previous version: ${previous_version}" >&2
    exit 1
  fi
  extracted="$(find "${tmp_dir}" -mindepth 1 -maxdepth 1 -type d | head -n1)"
  if [[ -z "${extracted}" || ! -f "${extracted}/scripts/install.sh" ]]; then
    rm -rf "${tmp_dir}"
    echo "error: the HostForge ${target_version} archive did not contain scripts/install.sh." >&2
    echo "previous version: ${previous_version}" >&2
    exit 1
  fi
  # Move the old tree aside rather than deleting it outright, so an
  # interrupted swap leaves something to recover from. This script is usually
  # running out of that very directory; renaming is safe where deleting the
  # file bash is still reading is needlessly close to the edge.
  if [[ -d "${REPO_DIR}" ]]; then
    mv "${REPO_DIR}" "${old_dir}"
  fi
  if ! mv "${extracted}" "${REPO_DIR}"; then
    [[ -d "${old_dir}" ]] && mv "${old_dir}" "${REPO_DIR}"
    rm -rf "${tmp_dir}"
    echo "error: could not install the ${target_version} tree at ${REPO_DIR}." >&2
    echo "previous version: ${previous_version}" >&2
    exit 1
  fi
  rm -rf "${tmp_dir}" "${old_dir}"
  cd "${REPO_DIR}"
  echo "Installing ${target_version}..."
  HOSTFORGE_VERSION="${target_version}" ./scripts/install.sh --with-systemd --download-release
fi

echo "Restarting ${SERVICE}..."
systemctl restart "${SERVICE}"
if ! systemctl is-active --quiet "${SERVICE}"; then
  echo "error: ${SERVICE} did not become active" >&2
  systemctl --no-pager --full status "${SERVICE}" >&2 || true
  echo "previous version: ${previous_version}" >&2
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
  echo "previous version: ${previous_version}" >&2
  exit 1
fi

configured_gateway_flag="$(read_env_value HOSTFORGE_DATABASE_GATEWAYS_ENABLED)"
configured_gateway_flag="${configured_gateway_flag//[[:space:]]/}"
configured_gateway_flag="${configured_gateway_flag,,}"
case "${configured_gateway_flag:-false}" in
  1|t|true) expected_gateway_enabled="true" ;;
  0|f|false) expected_gateway_enabled="false" ;;
  *)
    echo "error: HOSTFORGE_DATABASE_GATEWAYS_ENABLED must be a boolean" >&2
    exit 1
    ;;
esac
echo "Verifying the running database gateway feature state is ${expected_gateway_enabled}..."
if ! curl --silent --show-error --fail --output "${gateway_body}" --max-time 5 \
  -H "Accept: application/json" \
  -H "Authorization: Bearer ${api_token}" \
  "${HF_LOCAL_SERVER_URL%/}/api/database-gateways/postgresql"; then
  echo "error: local PostgreSQL gateway status endpoint is unavailable" >&2
  exit 1
fi
if ! observed_gateway_enabled="$(json_gateway_feature_enabled "${gateway_body}")"; then
  echo "error: local PostgreSQL gateway status response is invalid" >&2
  exit 1
fi
if [[ "${observed_gateway_enabled}" != "${expected_gateway_enabled}" ]]; then
  echo "error: running database gateway feature state is ${observed_gateway_enabled}, expected ${expected_gateway_enabled}" >&2
  exit 1
fi
echo "Database gateway feature state matches the configured rollout state."

if [[ -z "${HF_SERVER_URL}" ]]; then
  if ! platform_domain="$(json_platform_domain "${onboarding_body}")"; then
    echo "error: the local onboarding response was not valid JSON" >&2
    exit 1
  fi
  if [[ -z "${platform_domain}" ]]; then
    # The release install above already succeeded; only the public smoke test
    # needs an onboarded box, and this one is not onboarded yet. That is not a
    # failed update -- report the version change and stop cleanly. Set
    # HF_SERVER_URL to force the public smoke test against a known origin.
    echo "Onboarding has no saved platform domain, so the public smoke test is skipped."
    echo "The release install succeeded: ${previous_version} -> ${target_version}."
    echo "Set HF_SERVER_URL=https://<domain> to run the public smoke test."
    exit 0
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
  echo "previous version: ${previous_version}" >&2
  exit 1
fi

echo "Running HostForge v2 API acceptance smoke..."
HF_TOKEN="${api_token}" \
  HF_SERVER_URL="${HF_SERVER_URL}" \
  ./scripts/v2-staging-api-smoke.sh
echo "Running managed database and gateway isolation audit..."
bash ./scripts/database-services-vps-audit.sh

unset api_token

systemctl --no-pager --full status "${SERVICE}"
echo "HostForge update and smoke complete: ${previous_version} -> ${target_version}"
