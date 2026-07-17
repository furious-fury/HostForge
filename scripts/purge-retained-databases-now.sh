#!/usr/bin/env bash
set -Eeuo pipefail

# Testing-only escape hatch. This deliberately expires retained database
# records, then asks the normal authenticated API to perform its ownership-
# checked volume and record purge. Do not use this when recovery is desired.

ENV_FILE="${HF_ENV_FILE:-/etc/hostforge/hostforge.env}"
TARGET_SERVICE_ID="${HF_DATABASE_SERVICE_ID:-}"
CONFIRM="${HF_CONFIRM_PURGE_RETAINED_DATABASES:-}"

read_env_value() {
  local key="$1"
  tr -d '\r' <"${ENV_FILE}" | awk -v key="${key}" '
    $0 ~ "^[[:space:]]*" key "=" {
      line=$0
      sub("^[[:space:]]*" key "=", "", line)
      gsub(/^['\''\"]|['\''\"]$/, "", line)
      print line
      exit
    }
  '
}

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: run this testing purge as root" >&2
  exit 1
fi
if [[ "${CONFIRM}" != "PURGE" ]]; then
  echo "error: this permanently deletes retained database volumes" >&2
  echo "rerun with HF_CONFIRM_PURGE_RETAINED_DATABASES=PURGE" >&2
  exit 1
fi
for tool in curl python3 awk; do
  command -v "${tool}" >/dev/null 2>&1 || { echo "error: ${tool} is required" >&2; exit 1; }
done
if [[ ! -r "${ENV_FILE}" ]]; then
  echo "error: cannot read ${ENV_FILE}" >&2
  exit 1
fi

data_dir="$(read_env_value HOSTFORGE_DATA_DIR)"
data_dir="${data_dir:-/var/lib/hostforge}"
database_path="${data_dir}/hostforge.db"
api_token="$(read_env_value HOSTFORGE_API_TOKEN)"
listen="$(read_env_value HOSTFORGE_LISTEN)"
listen="${listen:-127.0.0.1:8080}"
case "${listen}" in
  0.0.0.0:*) server_url="http://127.0.0.1:${listen##*:}" ;;
  \[::\]:*) server_url="http://[::1]:${listen##*:}" ;;
  :*) server_url="http://127.0.0.1:${listen##*:}" ;;
  *) server_url="http://${listen}" ;;
esac
if [[ ! -f "${database_path}" || -z "${api_token}" ]]; then
  echo "error: HostForge database or API token is unavailable" >&2
  exit 1
fi

records_file="$(mktemp "${TMPDIR:-/tmp}/hostforge-retained-databases.XXXXXX")"
response_file="$(mktemp "${TMPDIR:-/tmp}/hostforge-purge-response.XXXXXX")"
cleanup() { rm -f "${records_file}" "${response_file}"; }
trap cleanup EXIT

python3 - "${database_path}" "${TARGET_SERVICE_ID}" >"${records_file}" <<'PY'
import json, sqlite3, sys

database_path, target = sys.argv[1:]
db = sqlite3.connect(database_path, timeout=30)
query = """
SELECT services.id, services.name, COUNT(database_instances.id)
FROM services
JOIN database_instances ON database_instances.service_id = services.id
WHERE services.service_type = 'database'
GROUP BY services.id, services.name
HAVING COUNT(database_instances.id) > 0
   AND SUM(CASE WHEN database_instances.deleted_at <> '' THEN 1 ELSE 0 END) = COUNT(database_instances.id)
"""
rows = db.execute(query).fetchall()
for service_id, name, instances in rows:
    if not target or target == service_id:
        print(json.dumps({"id": service_id, "name": name, "instances": instances}))
PY

if [[ ! -s "${records_file}" ]]; then
  echo "No retained database services matched."
  exit 0
fi

echo "Permanently purging these retained database services:"
while IFS= read -r record; do
  python3 -c 'import json,sys; row=json.loads(sys.argv[1]); print(f"- {row['\''name'\'']} ({row['\''id'\'']}, {row['\''instances'\'']} instance(s))")' "${record}"
done <"${records_file}"

failures=0
while IFS= read -r record; do
  service_id="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["id"])' "${record}")"
  service_name="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["name"])' "${record}")"
  python3 - "${database_path}" "${service_id}" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1], timeout=30)
db.execute("UPDATE database_instances SET purge_after='1970-01-01T00:00:00Z', updated_at=datetime('now') WHERE service_id=? AND deleted_at<>''", (sys.argv[2],))
db.commit()
PY
  payload="$(python3 -c 'import json,sys; print(json.dumps({"confirmation": sys.argv[1]}))' "${service_name}")"
  status="$(curl --silent --show-error --output "${response_file}" --write-out '%{http_code}' \
    -H "Authorization: Bearer ${api_token}" -H 'Content-Type: application/json' \
    -X DELETE --data "${payload}" "${server_url}/api/database-services/${service_id}/purge" || true)"
  if [[ "${status}" == "200" ]]; then
    echo "Purged ${service_name} (${service_id})."
  else
    echo "Failed to purge ${service_name} (${service_id}); HTTP ${status}: $(cat "${response_file}")" >&2
    failures=$((failures + 1))
  fi
done <"${records_file}"

if (( failures > 0 )); then
  echo "${failures} retained database purge(s) failed. They remain due for the background purge worker." >&2
  exit 1
fi
echo "All selected retained database services were permanently purged."
