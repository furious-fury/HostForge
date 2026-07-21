#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/env-file.sh"

work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
env_file="${work_dir}/hostforge.env"

printf 'HOSTFORGE_API_TOKEN=test\n' >"${env_file}"
hostforge_ensure_env_default "${env_file}" HOSTFORGE_CADDY_STORAGE_ROOT /var/lib/caddy/.local/share/caddy
if [[ "$(grep -c '^HOSTFORGE_CADDY_STORAGE_ROOT=' "${env_file}")" != "1" ]]; then
  echo "FAIL: missing Caddy storage default" >&2
  exit 1
fi
if [[ "$(hostforge_read_env_value "${env_file}" HOSTFORGE_CADDY_STORAGE_ROOT)" != "/var/lib/caddy/.local/share/caddy" ]]; then
  echo "FAIL: appended Caddy storage default could not be read" >&2
  exit 1
fi

printf 'HOSTFORGE_CADDY_STORAGE_ROOT=/srv/custom-caddy\n' >"${env_file}"
hostforge_ensure_env_default "${env_file}" HOSTFORGE_CADDY_STORAGE_ROOT /var/lib/caddy/.local/share/caddy
if ! grep -q '^HOSTFORGE_CADDY_STORAGE_ROOT=/srv/custom-caddy$' "${env_file}"; then
  echo "FAIL: custom Caddy storage root was overwritten" >&2
  exit 1
fi
if [[ "$(grep -c '^HOSTFORGE_CADDY_STORAGE_ROOT=' "${env_file}")" != "1" ]]; then
  echo "FAIL: duplicate Caddy storage assignment" >&2
  exit 1
fi
if [[ "$(hostforge_read_env_value "${env_file}" HOSTFORGE_CADDY_STORAGE_ROOT)" != "/srv/custom-caddy" ]]; then
  echo "FAIL: custom Caddy storage root could not be read" >&2
  exit 1
fi

echo "PASS: installer environment defaults are idempotent"
