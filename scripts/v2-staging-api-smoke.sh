#!/usr/bin/env bash
set -euo pipefail

: "${HF_SERVER_URL:?set HF_SERVER_URL to the HostForge management origin}"
: "${HF_TOKEN:?set HF_TOKEN to the management API token}"

if ! command -v curl >/dev/null 2>&1; then
  echo "error: curl is required" >&2
  exit 1
fi
if command -v jq >/dev/null 2>&1; then
  json_tool="jq"
elif command -v python3 >/dev/null 2>&1; then
  json_tool="python3"
else
  echo "error: jq or python3 is required for JSON validation" >&2
  exit 1
fi

server="${HF_SERVER_URL%/}"
cookie_jar="$(mktemp "${TMPDIR:-/tmp}/hostforge-v2-cookie.XXXXXX")"
response_body="$(mktemp "${TMPDIR:-/tmp}/hostforge-v2-response.XXXXXX")"

cleanup() {
  rm -f "${cookie_jar}" "${response_body}"
}
trap cleanup EXIT

json_validate() {
  local file="$1"
  if [[ "${json_tool}" == "jq" ]]; then
    jq -e . "${file}" >/dev/null
  else
    python3 -c 'import json,sys; json.load(open(sys.argv[1], encoding="utf-8"))' "${file}"
  fi
}

json_matches() {
  local file="$1"
  local field="$2"
  local expected="$3"
  if [[ "${json_tool}" == "jq" ]]; then
    jq -e --arg field "${field}" --arg expected "${expected}" '.[$field] == ($expected | fromjson)' "${file}" >/dev/null
  else
    python3 -c 'import json,sys; value=json.load(open(sys.argv[1], encoding="utf-8")).get(sys.argv[2]); expected=json.loads(sys.argv[3]); raise SystemExit(0 if value == expected else 1)' "${file}" "${field}" "${expected}"
  fi
}

json_first() {
  local file="$1"
  local collection="$2"
  local field="$3"
  if [[ "${json_tool}" == "jq" ]]; then
    jq -r --arg collection "${collection}" --arg field "${field}" '.[$collection][0][$field] // empty' "${file}"
  else
    python3 -c 'import json,sys; data=json.load(open(sys.argv[1], encoding="utf-8")); rows=data.get(sys.argv[2]) or []; print(rows[0].get(sys.argv[3], "") if rows else "")' "${file}" "${collection}" "${field}"
  fi
}

json_first_environment_by_kind() {
  local file="$1"
  local kind="$2"
  if [[ "${json_tool}" == "jq" ]]; then
    jq -r --arg kind "${kind}" '.environments[]? | select(.kind == $kind) | .id' "${file}" | head -n 1
  else
    python3 -c 'import json,sys; rows=json.load(open(sys.argv[1], encoding="utf-8")).get("environments") or []; print(next((row.get("id", "") for row in rows if row.get("kind") == sys.argv[2]), ""))' "${file}" "${kind}"
  fi
}

json_first_service_by_type() {
  local file="$1"
  local service_type="$2"
  if [[ "${json_tool}" == "jq" ]]; then
    jq -r --arg service_type "${service_type}" '.services[]? | select(.service_type == $service_type) | .id' "${file}" | head -n 1
  else
    python3 -c 'import json,sys; rows=json.load(open(sys.argv[1], encoding="utf-8")).get("services") or []; print(next((row.get("id", "") for row in rows if row.get("service_type") == sys.argv[2]), ""))' "${file}" "${service_type}"
  fi
}

json_database_catalog() {
  local file="$1"
  if [[ "${json_tool}" == "jq" ]]; then
    jq -e '([.engines[]?.id] | sort) == (["mariadb","mongodb","mysql","postgresql","redis","valkey"] | sort) and (.resource_presets | type == "array") and (.networking.public_access_available == false)' "${file}" >/dev/null
  else
    python3 -c 'import json,sys; data=json.load(open(sys.argv[1], encoding="utf-8")); engines={row.get("id") for row in data.get("engines") or []}; ok=engines=={"postgresql","mysql","mariadb","mongodb","redis","valkey"} and isinstance(data.get("resource_presets"),list) and (data.get("networking") or {}).get("public_access_available") is False; raise SystemExit(0 if ok else 1)' "${file}"
  fi
}

json_array() {
  local file="$1"
  local field="$2"
  if [[ "${json_tool}" == "jq" ]]; then
    jq -e --arg field "${field}" '.[$field] | type == "array"' "${file}" >/dev/null
  else
    python3 -c 'import json,sys; value=json.load(open(sys.argv[1], encoding="utf-8")).get(sys.argv[2]); raise SystemExit(0 if isinstance(value, list) else 1)' "${file}" "${field}"
  fi
}

json_has_environment_kinds() {
  local file="$1"
  if [[ "${json_tool}" == "jq" ]]; then
    jq -e '([.environments[]?.kind] | index("production") != null) and ([.environments[]?.kind] | index("staging") != null)' "${file}" >/dev/null
  else
    python3 -c 'import json,sys; kinds={row.get("kind") for row in (json.load(open(sys.argv[1], encoding="utf-8")).get("environments") or [])}; raise SystemExit(0 if {"production","staging"} <= kinds else 1)' "${file}"
  fi
}

request() {
  local method="$1"
  local path="$2"
  local expected="${3:-200}"
  local code
  code="$(curl -sS -o "${response_body}" -w "%{http_code}" \
    -X "${method}" -b "${cookie_jar}" -c "${cookie_jar}" \
    -H "Accept: application/json" "${server}${path}")"
  if [[ "${code}" != "${expected}" ]]; then
    echo "error: ${method} ${path} returned ${code}, expected ${expected}" >&2
    cat "${response_body}" >&2
    exit 1
  fi
  if [[ "${code}" != "204" ]]; then
    json_validate "${response_body}"
    if [[ "${expected}" == 2* ]] && json_matches "${response_body}" "status" '"error"' >/dev/null 2>&1; then
      echo "error: ${method} ${path} returned an API error" >&2
      cat "${response_body}" >&2
      exit 1
    fi
  fi
  echo "PASS ${method} ${path} (${code})"
}

request_status() {
  local method="$1"
  local path="$2"
  local expected="$3"
  local code
  code="$(curl -sS -o "${response_body}" -w "%{http_code}" \
    -X "${method}" -b "${cookie_jar}" -c "${cookie_jar}" \
    -H "Accept: application/json" "${server}${path}")"
  if [[ "${code}" != "${expected}" ]]; then
    echo "error: ${method} ${path} returned ${code}, expected ${expected}" >&2
    cat "${response_body}" >&2
    exit 1
  fi
  echo "PASS ${method} ${path} (${code})"
}

verify_release_contract() {
  local code
  code="$(curl -sS -o "${response_body}" -w "%{http_code}" \
    -X POST -b "${cookie_jar}" -c "${cookie_jar}" \
    -H "Accept: application/json" "${server}/api/github/app/manifest/exchange")"
  if [[ "${code}" == "404" ]]; then
    echo "error: the server does not expose /api/github/app/manifest/exchange; deploy the current HostForge v2 release before acceptance" >&2
    cat "${response_body}" >&2
    exit 1
  fi
  if [[ "${code}" != "415" ]]; then
    echo "error: POST /api/github/app/manifest/exchange returned ${code}, expected 415 for a request without JSON content" >&2
    cat "${response_body}" >&2
    exit 1
  fi
  json_validate "${response_body}"
  echo "PASS POST /api/github/app/manifest/exchange (415)"
}

echo "==> establish API-token session"
login_code="$(curl -sS -o "${response_body}" -w "%{http_code}" \
  -X POST -c "${cookie_jar}" \
  -H "Accept: application/json" \
  -H "Authorization: Bearer ${HF_TOKEN}" \
  "${server}/auth/session")"
if [[ "${login_code}" != "200" ]]; then
  echo "error: login returned ${login_code}" >&2
  cat "${response_body}" >&2
  exit 1
fi
json_validate "${response_body}"
json_matches "${response_body}" "authenticated" "true"
echo "PASS POST /auth/session (200)"

request GET "/auth/session"
request GET "/api/onboarding"
request GET "/api/settings"
request GET "/api/system/status"
request GET "/api/system/host/snapshot"
request GET "/api/system/host/history?points=12"
json_array "${response_body}" "samples"
request GET "/api/github/app"
request GET "/api/github/installations"
json_array "${response_body}" "installations"
verify_release_contract
request POST "/api/github/app/exchange" "404"
request_status GET "/api/projects" "404"
request GET "/api/observability/summary"
request GET "/api/observability/requests?limit=10"
json_array "${response_body}" "requests"
request GET "/api/observability/deploy-steps?limit=10"
json_array "${response_body}" "deploy_steps"
request GET "/api/events?limit=10"
json_array "${response_body}" "events"
request GET "/api/deployments?limit=10"
json_array "${response_body}" "deployments"
request GET "/api/applications"
json_array "${response_body}" "applications"
request GET "/api/database-engines"
json_database_catalog "${response_body}"
request GET "/api/backup-destinations"
json_array "${response_body}" "destinations"
request GET "/api/applications"

application_id="${HF_APPLICATION_ID:-$(json_first "${response_body}" "applications" "id")}"
if [[ -n "${application_id}" ]]; then
  request GET "/api/applications/${application_id}"
  json_array "${response_body}" "environments"
  json_array "${response_body}" "services"
  json_has_environment_kinds "${response_body}"
  service_id="${HF_SERVICE_ID:-$(json_first_service_by_type "${response_body}" "application")}"
  database_service_id="$(json_first_service_by_type "${response_body}" "database")"
  environment_id="${HF_ENVIRONMENT_ID:-$(json_first_environment_by_kind "${response_body}" "production")}"

  if [[ -n "${service_id}" ]]; then
    request GET "/api/services/${service_id}"
    json_array "${response_body}" "bindings"
    json_array "${response_body}" "environment_states"
    if [[ -n "${environment_id}" ]]; then
      request GET "/api/services/${service_id}/environments/${environment_id}/metrics?points=12"
      json_array "${response_body}" "samples"
      request GET "/api/applications/${application_id}/environments/${environment_id}/domains?service_id=${service_id}"
      json_array "${response_body}" "domains"
      request GET "/api/applications/${application_id}/environments/${environment_id}/variables?service_id=${service_id}"
      json_array "${response_body}" "variables"
    fi
  else
    echo "SKIP application service detail checks (no application service exists)"
  fi
  if [[ -n "${database_service_id}" ]]; then
    request GET "/api/database-services/${database_service_id}"
    json_array "${response_body}" "instances"
    json_array "${response_body}" "operations"
  else
    echo "SKIP database service detail checks (no database service exists)"
  fi
else
  echo "SKIP application/service detail checks (no application exists)"
fi

request GET "/api/deployments?limit=10"
json_array "${response_body}" "deployments"
deployment_id="${HF_DEPLOYMENT_ID:-$(json_first "${response_body}" "deployments" "id")}"
if [[ -n "${deployment_id}" ]]; then
  request GET "/api/deployments/${deployment_id}"
  request GET "/api/deployments/${deployment_id}/steps"
  json_array "${response_body}" "steps"
  request GET "/api/deployments/${deployment_id}/logs?eof_meta=1&tail_lines=20"
else
  echo "SKIP deployment detail checks (no deployment exists)"
fi

echo "==> clear session and verify protection"
request DELETE "/auth/session"
request GET "/api/applications" "401"

echo "==> HostForge v2 staging API smoke: PASS"
