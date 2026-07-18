#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  cat <<'EOF'
Usage: database-gateway-vps-acceptance.sh <prepare|verify>

prepare requires:
  HF_GATEWAY_MIGRATION_URL   migration-profile URL from HostForge
  HF_GATEWAY_TEST_SCHEMA     safe, unique schema name used by both phases

verify additionally requires:
  HF_GATEWAY_READ_ONLY_URL   read-only URL created after prepare completes
  HF_GATEWAY_READ_WRITE_URL  read-write URL created after prepare completes

Every URL must contain sslmode=verify-full. URLs remain in environment variables
and are never printed. Run both phases from an IPv4/IPv6 source allowed by each
connection's CIDR rules.
EOF
}

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

pass() {
  echo "PASS: $*"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

require_value() {
  local name="$1"
  [[ -n "${!name:-}" ]] || fail "$name is required"
}

require_verify_full_url() {
  local name="$1"
  local value="${!name}"
  case "${value}" in
    postgres://*|postgresql://*) ;;
    *) fail "$name must be a PostgreSQL URL" ;;
  esac
  case "${value}" in
    *sslmode=verify-full*) ;;
    *) fail "$name must contain sslmode=verify-full" ;;
  esac
}

psql_query() {
  local url="$1"
  local sql="$2"
  PGCONNECT_TIMEOUT=10 PGDATABASE="${url}" \
    psql -X --no-psqlrc --tuples-only --no-align --quiet \
      --set=ON_ERROR_STOP=1 --command "${sql}"
}

psql_run() {
  local url="$1"
  local sql="$2"
  PGCONNECT_TIMEOUT=10 PGDATABASE="${url}" \
    psql -X --no-psqlrc --quiet --set=ON_ERROR_STOP=1 \
      --command "${sql}" >/dev/null
}

psql_script() {
  local url="$1"
  shift
  PGCONNECT_TIMEOUT=10 PGDATABASE="${url}" \
    psql -X --no-psqlrc --quiet --set=ON_ERROR_STOP=1 "$@"
}

expect_denied() {
  local label="$1"
  local url="$2"
  local sql="$3"
  local error_file
  error_file="$(mktemp "${TMPDIR:-/tmp}/hostforge-gateway-denied.XXXXXX")"
  if PGCONNECT_TIMEOUT=10 PGDATABASE="${url}" \
    psql -X --no-psqlrc --quiet --set=ON_ERROR_STOP=1 \
      --command "${sql}" >/dev/null 2>"${error_file}"; then
    rm -f "${error_file}"
    fail "${label} unexpectedly succeeded"
  fi
  rm -f "${error_file}"
  pass "${label} is denied"
}

assert_tls() {
  local label="$1"
  local url="$2"
  local tls_active
  tls_active="$(psql_query "${url}" "SELECT ssl FROM pg_stat_ssl WHERE pid=pg_backend_pid();")"
  [[ "${tls_active}" == "t" ]] || fail "${label} did not negotiate TLS"
  pass "${label} connects with TLS and sslmode=verify-full"
}

assert_safe_role() {
  local label="$1"
  local url="$2"
  local safe
  safe="$(psql_query "${url}" "SELECT bool_and(NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) FROM pg_roles WHERE rolname IN (session_user,current_user);")"
  [[ "${safe}" == "t" ]] || fail "${label} has a forbidden PostgreSQL role attribute"
  pass "${label} has no administrative PostgreSQL role attributes"
}

discover_owner_role() {
  local url="$1"
  local roles
  roles="$(psql_query "${url}" "SELECT current_user WHERE current_user<>session_user AND pg_has_role(session_user,current_user,'member');")"
  [[ -n "${roles}" ]] || fail "migration credential did not activate its application-owner role at login"
  [[ "${roles}" != *$'\n'* ]] || fail "migration credential activated more than one owner role"
  [[ "${roles}" =~ ^[a-z_][a-z0-9_]{0,62}$ ]] || fail "active application-owner role has an unexpected identifier"
  printf '%s' "${roles}"
}

prepare_fixture() {
  local migration_url="${HF_GATEWAY_MIGRATION_URL}"
  local schema="${HF_GATEWAY_TEST_SCHEMA}"
  local owner_role

  assert_tls "migration credential" "${migration_url}"
  assert_safe_role "migration credential" "${migration_url}"
  owner_role="$(discover_owner_role "${migration_url}")"

  psql_script "${migration_url}" \
    --set=owner_role="${owner_role}" --set=schema_name="${schema}" <<'SQL' >/dev/null
SELECT format('CREATE SCHEMA %I AUTHORIZATION %I', :'schema_name', :'owner_role') \gexec
SELECT format(
  'CREATE TABLE %I.current_rows (id bigserial PRIMARY KEY, note text NOT NULL)',
  :'schema_name'
) \gexec
SELECT format(
  'INSERT INTO %I.current_rows(note) VALUES (%L)',
  :'schema_name',
  'created before read-only and read-write grants'
) \gexec
SQL

  pass "migration credential created the current-object fixture as the application owner"
  echo "Fixture schema: ${schema}"
  echo "Create the read-only and read-write HostForge connections now, then run the verify phase with the same HF_GATEWAY_TEST_SCHEMA."
}

verify_profiles() {
  local migration_url="${HF_GATEWAY_MIGRATION_URL}"
  local read_only_url="${HF_GATEWAY_READ_ONLY_URL}"
  local read_write_url="${HF_GATEWAY_READ_WRITE_URL}"
  local schema="${HF_GATEWAY_TEST_SCHEMA}"
  local future_schema="${HF_GATEWAY_TEST_SCHEMA}_future" owner_role migration_role read_only_role read_write_role count inserted_id

  assert_tls "migration credential" "${migration_url}"
  assert_tls "read-only credential" "${read_only_url}"
  assert_tls "read-write credential" "${read_write_url}"
  assert_safe_role "migration credential" "${migration_url}"
  assert_safe_role "read-only credential" "${read_only_url}"
  assert_safe_role "read-write credential" "${read_write_url}"

  migration_role="$(psql_query "${migration_url}" "SELECT session_user;")"
  read_only_role="$(psql_query "${read_only_url}" "SELECT session_user;")"
  read_write_role="$(psql_query "${read_write_url}" "SELECT session_user;")"
  if [[ "${migration_role}" == "${read_only_role}" || "${migration_role}" == "${read_write_role}" || "${read_only_role}" == "${read_write_role}" ]]; then
    fail "each permission profile must use a distinct external credential role"
  fi
  pass "permission profiles use distinct generated roles"

  count="$(psql_query "${read_only_url}" "SELECT count(*) FROM ${schema}.current_rows;")"
  [[ "${count}" == "1" ]] || fail "read-only credential could not read the current-object fixture"
  pass "read-only credential can select current objects"

  inserted_id="$(psql_query "${read_write_url}" "INSERT INTO ${schema}.current_rows(note) VALUES ('read-write current object') RETURNING id;")"
  [[ "${inserted_id}" =~ ^[0-9]+$ ]] || fail "read-write insert did not return an id"
  psql_run "${read_write_url}" "UPDATE ${schema}.current_rows SET note='read-write current object updated' WHERE id=${inserted_id};"
  psql_run "${read_write_url}" "DELETE FROM ${schema}.current_rows WHERE id=${inserted_id};"
  pass "read-write credential can insert, update, delete, and use current-object sequences"

  expect_denied "read-only INSERT" "${read_only_url}" "INSERT INTO ${schema}.current_rows(note) VALUES ('forbidden');"
  expect_denied "read-only UPDATE" "${read_only_url}" "UPDATE ${schema}.current_rows SET note='forbidden' WHERE id=1;"
  expect_denied "read-only DELETE" "${read_only_url}" "DELETE FROM ${schema}.current_rows WHERE id=1;"
  expect_denied "read-only DDL" "${read_only_url}" "CREATE TABLE ${schema}.read_only_forbidden(id integer);"
  expect_denied "read-write TRUNCATE" "${read_write_url}" "TRUNCATE TABLE ${schema}.current_rows;"
  expect_denied "read-write DDL" "${read_write_url}" "CREATE TABLE ${schema}.read_write_forbidden(id integer);"

  psql_run "${migration_url}" "CREATE TABLE ${schema}.migration_ddl(id integer); ALTER TABLE ${schema}.migration_ddl ADD COLUMN note text; DROP TABLE ${schema}.migration_ddl;"
  pass "migration credential can perform owner-equivalent DDL"

  owner_role="$(discover_owner_role "${migration_url}")"
  psql_script "${migration_url}" \
    --set=owner_role="${owner_role}" --set=future_schema_name="${future_schema}" <<'SQL' >/dev/null
SELECT format(
  'CREATE SCHEMA %I AUTHORIZATION %I',
  :'future_schema_name',
  :'owner_role'
) \gexec
SELECT format(
  'CREATE TABLE %I.future_rows (id bigserial PRIMARY KEY, note text NOT NULL)',
  :'future_schema_name'
) \gexec
SELECT format(
  'INSERT INTO %I.future_rows(note) VALUES (%L)',
  :'future_schema_name',
  'created after read-only and read-write grants'
) \gexec
SQL

  count="$(psql_query "${read_only_url}" "SELECT count(*) FROM ${future_schema}.future_rows;")"
  [[ "${count}" == "1" ]] || fail "read-only credential did not receive future-object SELECT"
  inserted_id="$(psql_query "${read_write_url}" "INSERT INTO ${future_schema}.future_rows(note) VALUES ('read-write future object') RETURNING id;")"
  [[ "${inserted_id}" =~ ^[0-9]+$ ]] || fail "read-write credential did not receive future-object INSERT/sequence privileges"
  psql_run "${read_write_url}" "UPDATE ${future_schema}.future_rows SET note='read-write future object updated' WHERE id=${inserted_id};"
  psql_run "${read_write_url}" "DELETE FROM ${future_schema}.future_rows WHERE id=${inserted_id};"
  pass "read-only/read-write default privileges apply to future schemas, tables, and sequences"

  if [[ "${HF_GATEWAY_KEEP_FIXTURES:-false}" == "true" ]]; then
    echo "Keeping fixture schema ${schema} for lifecycle inspection."
  else
    psql_script "${migration_url}" \
      --set=owner_role="${owner_role}" --set=schema_name="${schema}" --set=future_schema_name="${future_schema}" <<'SQL' >/dev/null
SELECT format('SET ROLE %I', :'owner_role') \gexec
SELECT format('DROP SCHEMA %I CASCADE', :'future_schema_name') \gexec
SELECT format('DROP SCHEMA %I CASCADE', :'schema_name') \gexec
RESET ROLE;
SQL
    pass "acceptance fixture cleaned up"
  fi

  echo "==> PostgreSQL gateway TLS and permission acceptance: PASS"
}

mode="${1:-}"
case "${mode}" in
  prepare|verify) ;;
  -h|--help|help|"") usage; exit 0 ;;
  *) usage >&2; fail "unknown mode ${mode}" ;;
esac

require_command psql
require_value HF_GATEWAY_MIGRATION_URL
require_value HF_GATEWAY_TEST_SCHEMA
require_verify_full_url HF_GATEWAY_MIGRATION_URL

if [[ ! "${HF_GATEWAY_TEST_SCHEMA}" =~ ^[a-z][a-z0-9_]{0,47}$ ]]; then
  fail "HF_GATEWAY_TEST_SCHEMA must match ^[a-z][a-z0-9_]{0,47}$"
fi

if [[ "${mode}" == "prepare" ]]; then
  prepare_fixture
  exit 0
fi

require_value HF_GATEWAY_READ_ONLY_URL
require_value HF_GATEWAY_READ_WRITE_URL
require_verify_full_url HF_GATEWAY_READ_ONLY_URL
require_verify_full_url HF_GATEWAY_READ_WRITE_URL
verify_profiles
