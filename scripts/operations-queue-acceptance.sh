#!/usr/bin/env bash
set -Eeuo pipefail

# Acceptance checks for the generic operations queue (ADR-0002 §4, phase 1).
#
# Phase 1 moved every database operation onto a new queue table while keeping
# database_operations as a synchronised projection. Most of that is covered by
# unit tests, but four things can only be observed on a real host with a real
# database, and they are the ones that fail quietly rather than loudly:
#
#   - the migration's backfill, which behaves differently on a database that
#     has been in use than on a fresh one
#   - the two tables staying in step under real traffic
#   - operations that are stuck in a way the queue cannot see
#   - queuing replacing rejection when a database is already busy
#
# Sections A-C are read-only and safe to run against a live server at any
# time, including as a post-upgrade smoke check. Section D mutates and is
# opt-in.
#
# Usage:
#   sudo ./scripts/operations-queue-acceptance.sh
#   sudo HF_QUEUE_ACCEPTANCE_MUTATE=1 HF_DATABASE_INSTANCE_ID=... ./scripts/operations-queue-acceptance.sh
#
# Env overrides:
#   HF_ENV_FILE                   default: /etc/hostforge/hostforge.env
#   HF_SERVICE_NAME               default: hostforge-server
#   HF_QUEUE_ACCEPTANCE_MUTATE    set to 1 to run section D
#   HF_DATABASE_INSTANCE_ID       instance to exercise in section D

ENV_FILE="${HF_ENV_FILE:-/etc/hostforge/hostforge.env}"
SERVICE="${HF_SERVICE_NAME:-hostforge-server}"
MUTATE="${HF_QUEUE_ACCEPTANCE_MUTATE:-0}"
TARGET_INSTANCE="${HF_DATABASE_INSTANCE_ID:-}"

failures=0
fail() {
  echo "FAIL: $*" >&2
  failures=$((failures + 1))
}
pass() { echo "  ok: $*"; }

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: run this as root; it reads the control-plane database and the service env file" >&2
  exit 1
fi
if [[ ! -r "${ENV_FILE}" ]]; then
  echo "error: cannot read ${ENV_FILE}" >&2
  exit 1
fi
for tool in sqlite3 curl awk; do
  command -v "${tool}" >/dev/null 2>&1 || { echo "error: ${tool} is required" >&2; exit 1; }
done

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

DATA_DIR="$(read_env_value HOSTFORGE_DATA_DIR)"
DATA_DIR="${DATA_DIR:-/var/lib/hostforge}"
DB_PATH="${DATA_DIR}/hostforge.db"
if [[ ! -f "${DB_PATH}" ]]; then
  echo "error: ${DB_PATH} not found" >&2
  exit 1
fi

# Read-only, and WAL lets this run alongside the live server.
query() { sqlite3 -readonly "file:${DB_PATH}?mode=ro" "$1"; }

echo "HostForge operations queue acceptance"
echo "  database: ${DB_PATH}"
echo "  service:  ${SERVICE}"

# ---------------------------------------------------------------------------
echo
echo "A. Migration and backfill"
# ---------------------------------------------------------------------------

if [[ "$(query "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='operations'")" != "1" ]]; then
  fail "the operations table does not exist; migration 0028 has not applied"
  echo
  echo "${failures} check(s) failed." >&2
  exit 1
fi
pass "operations table exists"

queue_rows="$(query "SELECT COUNT(*) FROM operations")"
projection_rows="$(query "SELECT COUNT(*) FROM database_operations")"
if [[ "${queue_rows}" != "${projection_rows}" ]]; then
  fail "operations has ${queue_rows} rows, database_operations has ${projection_rows}; the backfill or an enqueue path missed rows"
else
  pass "row counts match (${queue_rows})"
fi

orphaned_projection="$(query "SELECT COUNT(*) FROM database_operations d WHERE NOT EXISTS (SELECT 1 FROM operations o WHERE o.id=d.id)")"
[[ "${orphaned_projection}" == "0" ]] \
  && pass "every database_operations row has a queue row" \
  || fail "${orphaned_projection} database_operations rows have no operations row; that work can never be claimed"

orphaned_queue="$(query "SELECT COUNT(*) FROM operations o WHERE NOT EXISTS (SELECT 1 FROM database_operations d WHERE d.id=o.id)")"
[[ "${orphaned_queue}" == "0" ]] \
  && pass "every queue row has a projection row" \
  || fail "${orphaned_queue} operations rows have no database_operations row; the API cannot see them"

# The backfill's real hazard: terminal 'delete' audit rows have no instance,
# so a lock key derived from the instance alone would be empty for exactly
# those rows — and only on a database where a service has been deleted.
bad_lock_keys="$(query "SELECT COUNT(*) FROM operations WHERE lock_key='' OR lock_key IS NULL OR (lock_key NOT LIKE 'dbi:%' AND lock_key NOT LIKE 'dbsvc:%')")"
[[ "${bad_lock_keys}" == "0" ]] \
  && pass "every operation has a well-formed lock key" \
  || fail "${bad_lock_keys} operations have an empty or malformed lock_key"

delete_rows="$(query "SELECT COUNT(*) FROM database_operations WHERE operation_type='delete'")"
if [[ "${delete_rows}" == "0" ]]; then
  echo "  note: no 'delete' audit rows present, so the backfill's hardest case is untested here."
  echo "        Delete a database service and re-run to exercise it."
else
  mismatched_delete="$(query "SELECT COUNT(*) FROM operations o JOIN database_operations d USING(id) WHERE d.operation_type='delete' AND o.lock_key NOT LIKE 'dbsvc:%'")"
  [[ "${mismatched_delete}" == "0" ]] \
    && pass "${delete_rows} delete audit row(s) carry service-scoped lock keys" \
    || fail "${mismatched_delete} delete audit rows have an instance-scoped lock key"
fi

snapshots="$(find "${DATA_DIR}" -maxdepth 1 -name 'hostforge.db.pre-migration-*.snapshot' 2>/dev/null | wc -l)"
[[ "${snapshots}" -gt 0 ]] \
  && pass "${snapshots} pre-migration snapshot(s) present" \
  || fail "no pre-migration snapshot found; an upgrade that applied migrations should have written one"

# ---------------------------------------------------------------------------
echo
echo "B. Projection consistency"
# ---------------------------------------------------------------------------

diverged="$(query "
  SELECT COUNT(*) FROM operations o JOIN database_operations d USING(id)
  WHERE o.status <> d.status
     OR o.progress_percent <> d.progress_percent
     OR o.attempt <> d.attempt_count")"
[[ "${diverged}" == "0" ]] \
  && pass "the two tables agree on status, progress, and attempts" \
  || fail "${diverged} operations disagree between the queue and its projection; the UI and the queue see different states"

# ---------------------------------------------------------------------------
echo
echo "C. Stuck work"
# ---------------------------------------------------------------------------

# Queued, but at its attempt limit: the claim skips these, so without the
# sweeper they would sit invisible forever while the UI polls them.
invisible="$(query "SELECT COUNT(*) FROM operations WHERE status='queued' AND attempt>=max_attempts")"
[[ "${invisible}" == "0" ]] \
  && pass "no queued operation is past its attempt limit" \
  || fail "${invisible} queued operations are past their attempt limit; the claim cannot pick them up and they will poll forever"

# Running with a lease that expired: recovery runs at startup, so a live
# server should never accumulate these.
expired="$(query "SELECT COUNT(*) FROM operations WHERE status='running' AND lease_expires_at<>'' AND lease_expires_at <= strftime('%Y-%m-%dT%H:%M:%SZ','now')")"
[[ "${expired}" == "0" ]] \
  && pass "no running operation holds an expired lease" \
  || fail "${expired} running operations hold an expired lease; a worker died without recovery picking it up"

if systemctl is-active --quiet "${SERVICE}"; then
  pass "${SERVICE} is active"
else
  fail "${SERVICE} is not active"
fi

# ---------------------------------------------------------------------------
echo
echo "D. Queuing replaces rejection"
# ---------------------------------------------------------------------------

if [[ "${MUTATE}" != "1" ]]; then
  echo "  skipped (read-only run). To exercise it:"
  echo "    sudo HF_QUEUE_ACCEPTANCE_MUTATE=1 HF_DATABASE_INSTANCE_ID=<id> $0"
  echo "  Pick a healthy instance id from:"
  echo "    sqlite3 -readonly '${DB_PATH}' \"SELECT id,network_alias,status FROM database_instances WHERE deleted_at='';\""
elif [[ -z "${TARGET_INSTANCE}" ]]; then
  fail "HF_QUEUE_ACCEPTANCE_MUTATE=1 requires HF_DATABASE_INSTANCE_ID"
else
  api_token="$(read_env_value HOSTFORGE_API_TOKEN)"
  listen="$(read_env_value HOSTFORGE_LISTEN)"
  listen="${listen:-127.0.0.1:8080}"
  case "${listen}" in
    0.0.0.0:*|:*) server_url="http://127.0.0.1:${listen##*:}" ;;
    \[::\]:*)     server_url="http://[::1]:${listen##*:}" ;;
    *)            server_url="http://${listen}" ;;
  esac
  if [[ -z "${api_token}" ]]; then
    fail "HOSTFORGE_API_TOKEN is unavailable; cannot exercise the API"
  else
    response_file="$(mktemp)"
    trap 'rm -f "${response_file}"' EXIT

    post_action() {
      curl --silent --show-error --output "${response_file}" --write-out '%{http_code}' \
        -H "Authorization: Bearer ${api_token}" -H 'Content-Type: application/json' \
        -X POST "${server_url}/api/database-instances/${TARGET_INSTANCE}/$1" || true
    }

    echo "  queueing two restarts back to back on ${TARGET_INSTANCE}"
    first_status="$(post_action restart)"
    second_status="$(post_action restart)"

    # Before phase 1 the second call returned an error because an operation
    # was already in progress. It should now be accepted and queued.
    if [[ "${first_status}" == "200" || "${first_status}" == "202" ]]; then
      pass "first action accepted (HTTP ${first_status})"
    else
      fail "first action returned HTTP ${first_status}: $(cat "${response_file}")"
    fi
    if [[ "${second_status}" == "200" || "${second_status}" == "202" ]]; then
      pass "second action accepted while the first was outstanding (HTTP ${second_status})"
    else
      fail "second action returned HTTP ${second_status}; it should queue, not be rejected: $(cat "${response_file}")"
    fi

    # And only one of them may actually be running.
    sleep 2
    running="$(query "SELECT COUNT(*) FROM operations WHERE lock_key='dbi:${TARGET_INSTANCE}' AND status='running'")"
    if [[ "${running}" -le 1 ]]; then
      pass "at most one operation is running for this instance (${running})"
    else
      fail "${running} operations are running for the same instance; lock_key is not serialising them"
    fi
  fi
fi

# ---------------------------------------------------------------------------
echo
echo "E. Checks that need a person"
# ---------------------------------------------------------------------------
cat <<'MANUAL'
  These cannot be asserted from a shell. Walk them once after an upgrade:

  1. Open a database service detail page while an operation runs.
     The page must keep refreshing every 2s and stop when the operation
     reaches a terminal status. A silent stop looks exactly like a hang,
     which is why no automated check covers it.

  2. With a backup running, click Stop. It must be accepted and then run
     after the backup, and the progress banner must show the *running*
     backup rather than the newly queued stop sitting at 0%.

  3. Trigger a replace-current restore. Its safety backup must finish
     before the restore starts, and the log should show at most one
     "waiting" deferral line every 5 seconds — not a tight loop.

  4. Restart the service mid-operation. The operation must be requeued
     and resume immediately on the next start, not sit idle for the
     two-minute lease to expire:
       journalctl -u SERVICE -n 100 | grep -i "recovered interrupted operations"
MANUAL

echo
if (( failures > 0 )); then
  echo "${failures} check(s) failed." >&2
  exit 1
fi
echo "All automated checks passed."
