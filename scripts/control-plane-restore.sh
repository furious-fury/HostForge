#!/usr/bin/env bash
set -Eeuo pipefail

# Restores hostforge.db from a control-plane snapshot: either the
# pre-migration snapshot internal/database.OpenSQLite writes next to
# hostforge.db, or the scheduled, retained snapshots
# internal/services.StartControlPlaneSnapshotLoop writes under
# HOSTFORGE_CONTROL_PLANE_SNAPSHOT_DIR (ADR-0002 §17).
#
# Usage:
#   control-plane-restore.sh /path/to/snapshot.sqlite
#
# For a snapshot uploaded to a backup_destinations remote, download it
# first with that provider's own tool, using the credentials already
# configured on that destination, then run this script against the local
# file. This script deliberately does not re-implement backup_destinations
# credential decryption in bash — that logic already exists in Go, in
# internal/backups and internal/crypto/envcrypt.
#
# IMPORTANT: a snapshot sealed with a different HOSTFORGE_ENV_ENCRYPTION_KEY
# than this host's current key will fail the encryption canary check at
# boot (ADR-0002 §20.4). That is an expected outcome of restoring the wrong
# key, not a bug in this script — restore the matching key (backed up
# separately, per §17.4) and restart. This script detects that case and
# exits 3, distinct from a mechanical failure (exit 1).
#
# Env overrides:
#   HF_ENV_FILE                          default: /etc/hostforge/hostforge.env
#   HF_SERVICE_NAME                      default: hostforge-server
#   HF_DB_OWNER / HF_DB_GROUP            default: hostforge / hostforge
#   HF_CONFIRM_CONTROL_PLANE_RESTORE     must be "RESTORE" to proceed

SNAPSHOT="${1:?usage: control-plane-restore.sh /path/to/snapshot.sqlite}"
ENV_FILE="${HF_ENV_FILE:-/etc/hostforge/hostforge.env}"
SERVICE="${HF_SERVICE_NAME:-hostforge-server}"
DB_OWNER="${HF_DB_OWNER:-hostforge}"
DB_GROUP="${HF_DB_GROUP:-hostforge}"
CONFIRM="${HF_CONFIRM_CONTROL_PLANE_RESTORE:-}"
CANARY_MESSAGE="does not match the key that encrypted this database's secrets"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: run this restore as root" >&2
  exit 1
fi
if [[ "${CONFIRM}" != "RESTORE" ]]; then
  echo "error: this replaces the live control-plane database" >&2
  echo "rerun with HF_CONFIRM_CONTROL_PLANE_RESTORE=RESTORE" >&2
  exit 1
fi
if [[ ! -s "${SNAPSHOT}" ]]; then
  echo "error: snapshot file ${SNAPSHOT} is missing or empty" >&2
  exit 1
fi
if [[ ! -r "${ENV_FILE}" ]]; then
  echo "error: cannot read ${ENV_FILE}" >&2
  exit 1
fi

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

if command -v sqlite3 >/dev/null 2>&1; then
  integrity="$(sqlite3 "${SNAPSHOT}" 'PRAGMA integrity_check;' 2>&1 || true)"
  if [[ "${integrity}" != "ok" ]]; then
    echo "error: snapshot failed integrity_check: ${integrity}" >&2
    exit 1
  fi
else
  echo "warning: sqlite3 not found; skipping integrity_check on the snapshot" >&2
fi

RESTORE_BACKUP="${DB_PATH}.pre-restore-$(date -u +%Y%m%dT%H%M%SZ).bak"
did_stop=0

rollback() {
  echo "Rolling back to the pre-restore backup..." >&2
  if [[ -f "${RESTORE_BACKUP}" ]]; then
    install -m 0640 -o "${DB_OWNER}" -g "${DB_GROUP}" "${RESTORE_BACKUP}" "${DB_PATH}"
    for sidecar in "-wal" "-shm"; do
      [[ -f "${RESTORE_BACKUP}${sidecar}" ]] && install -m 0640 -o "${DB_OWNER}" -g "${DB_GROUP}" "${RESTORE_BACKUP}${sidecar}" "${DB_PATH}${sidecar}"
    done
  fi
  rm -f "${DB_PATH}-wal" "${DB_PATH}-shm"
  if [[ "${did_stop}" -eq 1 ]]; then
    systemctl start "${SERVICE}" || true
  fi
}

echo "Stopping ${SERVICE}..."
systemctl stop "${SERVICE}"
did_stop=1

if [[ -f "${DB_PATH}" ]]; then
  echo "Backing up current database to ${RESTORE_BACKUP} before replacing it..."
  cp -a "${DB_PATH}" "${RESTORE_BACKUP}"
  for sidecar in "-wal" "-shm"; do
    [[ -f "${DB_PATH}${sidecar}" ]] && cp -a "${DB_PATH}${sidecar}" "${RESTORE_BACKUP}${sidecar}"
  done
fi

echo "Installing snapshot as ${DB_PATH}..."
install -m 0640 -o "${DB_OWNER}" -g "${DB_GROUP}" "${SNAPSHOT}" "${DB_PATH}"
rm -f "${DB_PATH}-wal" "${DB_PATH}-shm"

echo "Starting ${SERVICE}..."
if ! systemctl start "${SERVICE}"; then
  echo "error: ${SERVICE} failed to start with the restored database" >&2
  rollback
  exit 1
fi

# Poll for either a stable "active" state or the specific encryption-key
# canary mismatch in the journal — checked together each iteration, rather
# than assuming one check after a single sleep is enough.
outcome=""
for _ in $(seq 1 30); do
  sleep 2
  if journalctl -u "${SERVICE}" --no-pager -n 100 2>/dev/null | grep -qF "${CANARY_MESSAGE}"; then
    outcome="wrong_key"
    break
  fi
  if systemctl is-active --quiet "${SERVICE}"; then
    outcome="active"
    break
  fi
  # Neither condition matched yet — still starting, or crash-looping
  # without having logged the canary line this iteration. Keep polling
  # until the loop budget above is exhausted.
done

case "${outcome}" in
  active)
    echo "Restore complete. ${SERVICE} is active. Pre-restore backup retained at ${RESTORE_BACKUP} (not auto-deleted)."
    exit 0
    ;;
  wrong_key)
    echo "error: the restored snapshot was sealed with a different HOSTFORGE_ENV_ENCRYPTION_KEY than this host's current key." >&2
    echo "This is an expected failure mode (ADR-0002 §20.4/§17.4), not a bug in this script:" >&2
    echo "restore the encryption key that was in effect when this snapshot was taken, then restart ${SERVICE}." >&2
    echo "The database file itself was restored successfully; it was NOT rolled back — the pre-restore backup is at ${RESTORE_BACKUP} if you need it." >&2
    exit 3
    ;;
  *)
    echo "error: ${SERVICE} did not become active within the poll window" >&2
    journalctl -u "${SERVICE}" -n 100 --no-pager >&2 || true
    rollback
    exit 1
    ;;
esac
