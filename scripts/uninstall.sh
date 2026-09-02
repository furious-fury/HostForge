#!/usr/bin/env bash
# Remove a HostForge installation made by scripts/install.sh --with-systemd
# or scripts/bootstrap-ubuntu.sh, for a clean re-install on the same host.
# Counterpart to install.sh; every step is best-effort so re-running after
# an interruption is safe.
#
# Does NOT touch Docker, Node, Caddy, fail2ban, or other packages
# bootstrap-ubuntu.sh installs -- those are shared system packages, and
# bootstrap-ubuntu.sh reconfigures Caddy's Caddyfile unconditionally on
# every run anyway. This only removes what is specifically HostForge's.
#
# Usage:
#   sudo HF_CONFIRM_UNINSTALL=UNINSTALL ./scripts/uninstall.sh
#   sudo ./scripts/uninstall.sh --yes [--keep-data] [--keep-user]
#
# --keep-data   Leave /var/lib/hostforge (the sqlite database, worktrees,
#               build artifacts, and deploy logs) in place.
# --keep-user   Leave the hostforge system user/group in place.
set -uo pipefail

DATA_DIR="${HF_DATA_DIR:-/var/lib/hostforge}"
PREFIX="${HF_PREFIX:-/usr/local}"
INSTALL_DIR="${HF_INSTALL_DIR:-/opt/hostforge}"

# This script rm -rf's the directories named above, and two of them can be
# pointed anywhere by an environment variable. A typo or an unset variable
# upstream should not be able to take out something else -- refuse anything
# that is not clearly a dedicated HostForge directory.
#
# Deliberately duplicated in vps-update-and-smoke.sh rather than shared
# through scripts/lib: an uninstaller is what you reach for when the install
# is already broken, and it should not depend on the tree being intact.
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
  # At least two components: /opt/hostforge passes, /opt does not.
  case "${trimmed#/}" in
    */*) ;;
    *)
      echo "error: ${name} is too close to the filesystem root to delete: '${dir}'" >&2
      exit 2
      ;;
  esac
  case "${trimmed}" in
    /usr/bin|/usr/lib|/usr/local|/usr/sbin|/usr/share|/var/lib|/var/log|/etc/caddy|/home/*/|/root/*/)
      echo "error: ${name} names a shared system directory, refusing to delete: '${dir}'" >&2
      exit 2
      ;;
  esac
}
assert_removable_dir "${INSTALL_DIR}" HF_INSTALL_DIR
assert_removable_dir "${DATA_DIR}" HF_DATA_DIR
CONFIRM="${HF_CONFIRM_UNINSTALL:-}"
KEEP_DATA=0
KEEP_USER=0

for arg in "$@"; do
  case "$arg" in
    --yes|-y) CONFIRM="UNINSTALL" ;;
    --keep-data) KEEP_DATA=1 ;;
    --keep-user) KEEP_USER=1 ;;
    -h|--help)
      sed -n '1,20p' "$0" | sed -n '/^# /s/^# //p'
      exit 0
      ;;
    *)
      echo "unknown option: $arg" >&2
      exit 2
      ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: run as root (for example: sudo $0 --yes)" >&2
  exit 1
fi
if [[ "${CONFIRM}" != "UNINSTALL" ]]; then
  echo "error: this stops HostForge and deletes its systemd units, /etc/hostforge, and (unless --keep-data) ${DATA_DIR}." >&2
  echo "rerun with --yes, or HF_CONFIRM_UNINSTALL=UNINSTALL $0" >&2
  exit 1
fi

echo "==> Stopping HostForge"
systemctl stop hostforge-server 2>/dev/null || true
systemctl disable hostforge-server 2>/dev/null || true
rm -f /etc/systemd/system/hostforge-server.service

echo "==> Stopping the BuildKit daemon (bootstrap-ubuntu.sh)"
systemctl stop buildkitd 2>/dev/null || true
systemctl disable buildkitd 2>/dev/null || true
rm -f /etc/systemd/system/buildkitd.service
docker rm -f buildkitd 2>/dev/null || true

systemctl daemon-reload 2>/dev/null || true

echo "==> Removing HostForge binaries and Railpack/BuildKit helpers"
rm -f "${PREFIX}/bin/hostforge-server"
rm -f /usr/local/bin/railpack /usr/local/bin/buildctl
rm -rf /usr/local/lib/hostforge

echo "==> Removing /etc/hostforge (secrets, config)"
rm -rf /etc/hostforge

# The install tree: a source checkout, or the release tree the bootstrapper
# unpacks. Leaving it behind means the next install silently starts from
# whatever version was there before, which is exactly the confusion an
# uninstall is meant to clear. Any staging directories from an interrupted
# install or update go with it.
echo "==> Removing ${INSTALL_DIR}"
rm -rf "${INSTALL_DIR}" "${INSTALL_DIR}".replaced.* 2>/dev/null || true
rm -rf "$(dirname "${INSTALL_DIR}")"/.hostforge-install.* "$(dirname "${INSTALL_DIR}")"/.hostforge-update.* 2>/dev/null || true

if [[ "${KEEP_DATA}" -eq 1 ]]; then
  echo "==> Keeping ${DATA_DIR} (--keep-data)"
else
  echo "==> Removing ${DATA_DIR} (database, worktrees, build artifacts, deploy logs)"
  rm -rf "${DATA_DIR}"
fi

echo "==> Removing HostForge's Caddy routes"
rm -f /etc/caddy/hostforge.d/control-plane.caddy /etc/caddy/hostforge.d/routes.caddy
rmdir /etc/caddy/hostforge.d 2>/dev/null || true
# Caddy itself, and any non-HostForge Caddyfile content, is left alone.
# bootstrap-ubuntu.sh overwrites /etc/caddy/Caddyfile unconditionally on
# every run, so there is nothing to restore here.

if [[ "${KEEP_USER}" -eq 1 ]]; then
  echo "==> Keeping the hostforge system user (--keep-user)"
else
  echo "==> Removing the hostforge system user and group"
  userdel hostforge 2>/dev/null || true
  groupdel hostforge 2>/dev/null || true
fi

if command -v ufw >/dev/null 2>&1; then
  echo "==> Removing the UFW rule for HostForge's PostgreSQL gateway (5432/tcp)"
  ufw delete allow 5432/tcp 2>/dev/null || true
fi

cat <<'EOF'

HostForge removed. Docker, Node.js, Caddy, and fail2ban were left installed
-- bootstrap-ubuntu.sh reuses them idempotently on the next run.

Re-install with:
  curl -fsSL https://raw.githubusercontent.com/furious-fury/HostForge/main/scripts/bootstrap-ubuntu.sh | sudo bash
EOF
