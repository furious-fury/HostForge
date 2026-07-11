#!/usr/bin/env bash
# HostForge installer: build (or reuse) binaries, install under a prefix, optionally systemd.
# Idempotent: safe to re-run; does not overwrite an existing /etc/hostforge/hostforge.env.
#
# Usage (from repo clone):
#   ./scripts/install.sh [--prefix /usr/local] [--data-dir /var/lib/hostforge] [--with-systemd] [--skip-build]
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREFIX="/usr/local"
DATA_DIR="/var/lib/hostforge"
WITH_SYSTEMD=0
SKIP_BUILD=0

usage() {
  sed -n '1,80p' "$0" | sed -n '/^# /s/^# //p' | head -n 20
  cat <<'EOF'

Options:
  --prefix PATH     Install directory (default: /usr/local). Binaries: PREFIX/bin/
  --data-dir PATH   Server data directory used in systemd unit (default: /var/lib/hostforge)
  --with-systemd    Create hostforge user, data dirs, env example, systemd unit (requires root)
  --skip-build      Do not run go build; use ./hostforge and ./hostforge-server in repo root
  -h, --help        Show this help

Examples:
  ./scripts/install.sh
  sudo ./scripts/install.sh --with-systemd
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prefix)
      PREFIX="${2:?}"
      shift 2
      ;;
    --data-dir)
      DATA_DIR="${2:?}"
      shift 2
      ;;
    --with-systemd)
      WITH_SYSTEMD=1
      shift
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -f "${REPO_ROOT}/go.mod" ]]; then
  echo "error: go.mod not found; run this script from a HostForge repository clone." >&2
  exit 1
fi

BIN_DIR="${PREFIX}/bin"
TMP_BIN="${REPO_ROOT}/.install-build"
mkdir -p "${BIN_DIR}" 2>/dev/null || true

if [[ "${SKIP_BUILD}" -eq 0 ]]; then
  mkdir -p "${TMP_BIN}"
  echo "Building hostforge and hostforge-server..."
  (cd "${REPO_ROOT}" && go build -o "${TMP_BIN}/hostforge" ./cmd/cli)
  (cd "${REPO_ROOT}" && go build -o "${TMP_BIN}/hostforge-server" ./cmd/server)
  HF_CLI="${TMP_BIN}/hostforge"
  HF_SRV="${TMP_BIN}/hostforge-server"
else
  HF_CLI="${REPO_ROOT}/hostforge"
  HF_SRV="${REPO_ROOT}/hostforge-server"
  if [[ ! -x "${HF_CLI}" || ! -x "${HF_SRV}" ]]; then
    echo "error: --skip-build requires executable ${HF_CLI} and ${HF_SRV}" >&2
    exit 1
  fi
fi

install_bin() {
  local src="$1" name="$2"
  if [[ -w "${BIN_DIR}" ]]; then
    install -m 0755 "${src}" "${BIN_DIR}/${name}"
  else
    echo "Installing ${name} to ${BIN_DIR} (may prompt for sudo)..."
    sudo install -m 0755 "${src}" "${BIN_DIR}/${name}"
  fi
}

echo "Installing binaries to ${BIN_DIR}/ ..."
install_bin "${HF_CLI}" "hostforge"
install_bin "${HF_SRV}" "hostforge-server"

if [[ "${SKIP_BUILD}" -eq 0 ]]; then
  rm -rf "${TMP_BIN}"
fi

if [[ "${WITH_SYSTEMD}" -eq 0 ]]; then
  cat <<EOF

Installed:
  ${BIN_DIR}/hostforge
  ${BIN_DIR}/hostforge-server

Next steps:
  - Set secrets (see README "Authentication" and scripts/hostforge-server.env.example).
  - Run: hostforge-server -data-dir <dir> -listen <addr>
  - Install Caddy separately for TLS; point a route at HostForge if exposing the UI.
  - For systemd + data under ${DATA_DIR}, re-run: sudo $0 --prefix ${PREFIX} --data-dir ${DATA_DIR} --with-systemd${SKIP_BUILD:+ --skip-build}
EOF
  exit 0
fi

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: --with-systemd requires root (sudo)." >&2
  exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "error: systemctl not found; omit --with-systemd or install systemd." >&2
  exit 1
fi

if ! getent passwd hostforge >/dev/null 2>&1; then
  echo "Creating system user and group hostforge..."
  useradd --system --user-group --home-dir "${DATA_DIR}" --create-home --shell /usr/sbin/nologin hostforge
else
  echo "User hostforge already exists."
fi

echo "Creating data directory ${DATA_DIR}..."
install -d -m 0750 -o hostforge -g hostforge "${DATA_DIR}"

ETC_DIR="/etc/hostforge"
install -d -m 0750 -o root -g hostforge "${ETC_DIR}"

ENV_EXAMPLE="${REPO_ROOT}/scripts/hostforge-server.env.example"
ENV_FILE="${ETC_DIR}/hostforge.env"
if [[ ! -f "${ENV_FILE}" ]]; then
  if [[ -f "${ENV_EXAMPLE}" ]]; then
    env_tmp="$(mktemp)"
    sed \
      -e "s|^HOSTFORGE_DATA_DIR=.*|HOSTFORGE_DATA_DIR=${DATA_DIR}|" \
      -e "s|^HOSTFORGE_LISTEN=.*|HOSTFORGE_LISTEN=127.0.0.1:8080|" \
      "${ENV_EXAMPLE}" >"${env_tmp}"
    install -m 0640 -o root -g hostforge "${env_tmp}" "${ENV_FILE}"
    rm -f "${env_tmp}"
    echo "Created ${ENV_FILE} — edit and set HOSTFORGE_API_TOKEN, HOSTFORGE_SESSION_SECRET, HOSTFORGE_WEBHOOK_SECRET before starting."
  else
    echo "warning: ${ENV_EXAMPLE} missing; create ${ENV_FILE} manually." >&2
  fi
else
  echo "Keeping existing ${ENV_FILE}"
fi

UNIT_PATH="/etc/systemd/system/hostforge-server.service"
SERVER_BIN="${BIN_DIR}/hostforge-server"
cat >"${UNIT_PATH}" <<UNIT
[Unit]
Description=HostForge control plane (API, UI, webhooks)
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
User=hostforge
Group=hostforge
EnvironmentFile=-${ENV_FILE}
# HOSTFORGE_LISTEN and all secrets come from EnvironmentFile; cmd/server defaults -listen from env.
ExecStart=${SERVER_BIN} -data-dir ${DATA_DIR}
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT

if getent group docker >/dev/null 2>&1; then
  echo "Adding hostforge to docker group (restart hostforge-server after first deploy setup if needed)..."
  usermod -aG docker hostforge 2>/dev/null || true
fi

systemctl daemon-reload
systemctl enable hostforge-server.service

cat <<EOF

systemd unit installed: ${UNIT_PATH}
Environment file: ${ENV_FILE} (edit secrets before start)

Commands:
  sudo systemctl start hostforge-server
  sudo systemctl status hostforge-server

Caddy: install separately; open 80/443; reverse_proxy to HostForge if not on loopback.
GitHub webhook URL uses path from HOSTFORGE_WEBHOOK_BASE_PATH (default /hooks/github).
EOF
