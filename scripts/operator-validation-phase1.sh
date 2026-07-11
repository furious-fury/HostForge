#!/usr/bin/env bash
# Automates Detailed backlog §1.1 (Docker runtime proof): preflight, deploy sample app,
# HTTP probe, restart policy, docker restart survival, stop+rm, port released.
#
# From repo root:
#   ./scripts/operator-validation-phase1.sh
#
# Env:
#   HOSTFORGE_BIN   Path to hostforge binary (default: build to a temp binary with go build)
#   REPO_URL        Sample repo (default: heroku node getting started)
#   HF_DATA         Data dir (default: mktemp under /tmp); removed on success unless KEEP_HF_DATA=1
#   KEEP_HF_DATA    If 1, do not rm -rf HF_DATA on exit
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_URL="${REPO_URL:-https://github.com/heroku/node-js-getting-started}"
KEEP_HF_DATA="${KEEP_HF_DATA:-0}"
TMP_BIN=""

cleanup() {
  local code=$?
  if [[ -n "${TMP_BIN}" && -f "${TMP_BIN}" ]]; then
    rm -f "${TMP_BIN}"
  fi
  if [[ "${KEEP_HF_DATA}" != "1" && -n "${HF_DATA:-}" && -d "${HF_DATA}" ]]; then
    rm -rf "${HF_DATA}"
  fi
  exit "${code}"
}
trap cleanup EXIT

cd "${REPO_ROOT}"

if [[ -n "${HOSTFORGE_BIN:-}" ]]; then
  HF=( "${HOSTFORGE_BIN}" )
else
  TMP_BIN="$(mktemp "${TMPDIR:-/tmp}/hostforge-phase1-bin.XXXXXX")"
  go build -o "${TMP_BIN}" ./cmd/cli
  HF=( "${TMP_BIN}" )
fi

HF_DATA="${HF_DATA:-$(mktemp -d "${TMPDIR:-/tmp}/hostforge-phase1-data.XXXXXX")}"
export HOSTFORGE_DATA_DIR="${HF_DATA}"

echo "==> preflight"
"${HF[@]}" validate preflight

echo "==> deploy (ephemeral host port)"
DEPLOY_LOG="$(mktemp "${TMPDIR:-/tmp}/hostforge-phase1-deploy.XXXXXX")"
if ! HOSTFORGE_DATA_DIR="${HF_DATA}" "${HF[@]}" deploy -host-port 0 "${REPO_URL}" 2>&1 | tee "${DEPLOY_LOG}"; then
  echo "error: deploy failed" >&2
  exit 1
fi

container_id="$(grep '^container_id=' "${DEPLOY_LOG}" | tail -1 | cut -d= -f2-)"
host_port="$(grep '^host_port=' "${DEPLOY_LOG}" | tail -1 | cut -d= -f2-)"
if [[ -z "${container_id}" || -z "${host_port}" ]]; then
  echo "error: could not parse container_id or host_port from deploy output" >&2
  exit 1
fi
rm -f "${DEPLOY_LOG}"
echo "    container_id=${container_id}"
echo "    host_port=${host_port}"

echo "==> HTTP GET /"
code="$(curl -sS -o /dev/null -w "%{http_code}" "http://127.0.0.1:${host_port}/")"
if [[ "${code}" != "200" ]]; then
  echo "error: expected HTTP 200, got ${code}" >&2
  exit 1
fi
echo "    http_code=${code}"

echo "==> restart policy"
policy="$(docker inspect --format '{{.HostConfig.RestartPolicy.Name}}' "${container_id}")"
if [[ "${policy}" != "unless-stopped" ]]; then
  echo "error: expected restart policy unless-stopped, got ${policy}" >&2
  exit 1
fi
echo "    restart_policy=${policy}"

echo "==> docker restart + probe"
docker restart "${container_id}" >/dev/null
sleep 2
code="$(curl -sS -o /dev/null -w "%{http_code}" "http://127.0.0.1:${host_port}/")"
if [[ "${code}" != "200" ]]; then
  echo "error: after restart expected HTTP 200, got ${code}" >&2
  exit 1
fi
echo "    http_code=${code}"

echo "==> docker stop + rm"
docker stop "${container_id}" >/dev/null
docker rm "${container_id}" >/dev/null
if curl -sS --connect-timeout 2 -o /dev/null "http://127.0.0.1:${host_port}/" 2>/dev/null; then
  echo "error: expected curl to fail after container removed" >&2
  exit 1
fi
echo "    port closed (curl failed as expected)"

echo "==> §1.1 automation: PASS"
