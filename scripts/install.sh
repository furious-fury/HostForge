#!/usr/bin/env bash
# HostForge installer: build (or reuse) binaries, install under a prefix, optionally systemd.
# Idempotent: safe to re-run; does not overwrite an existing /etc/hostforge/hostforge.env.
#
# Usage (from repo clone):
#   ./scripts/install.sh [--prefix /usr/local] [--data-dir /var/lib/hostforge] [--with-systemd] [--interactive] [--skip-build]
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${REPO_ROOT}/scripts/lib/env-file.sh"
PREFIX="/usr/local"
DATA_DIR="/var/lib/hostforge"
WITH_SYSTEMD=0
SKIP_BUILD=0
INTERACTIVE=0

usage() {
  sed -n '1,80p' "$0" | sed -n '/^# /s/^# //p' | head -n 20
  cat <<'EOF'

Options:
  --prefix PATH     Install directory (default: /usr/local). Binaries: PREFIX/bin/
  --data-dir PATH   Server data directory used in systemd unit (default: /var/lib/hostforge)
  --with-systemd    Create hostforge user, data dirs, env example, systemd unit (requires root)
  --interactive     On first systemd install, prompt for the admin login secret and generate the remaining secrets
  --skip-build      Do not run go build; use ./hostforge-server in repo root
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
    --interactive)
      INTERACTIVE=1
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
	echo "Building hostforge-server..."
	(cd "${REPO_ROOT}" && go build -o "${TMP_BIN}/hostforge-server" ./cmd/server)
	if ! command -v npm >/dev/null 2>&1; then
		echo "error: npm is required to build web/dist; install Node.js 20+ and rerun the installer." >&2
		exit 1
	fi
	echo "Building HostForge web UI..."
	(cd "${REPO_ROOT}/web" && npm ci && npm run build)
  HF_SRV="${TMP_BIN}/hostforge-server"
else
  HF_SRV="${REPO_ROOT}/hostforge-server"
  if [[ ! -x "${HF_SRV}" ]]; then
    echo "error: --skip-build requires executable ${HF_SRV}" >&2
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
install_bin "${HF_SRV}" "hostforge-server"

if [[ "${SKIP_BUILD}" -eq 0 ]]; then
  rm -rf "${TMP_BIN}"
fi

if [[ "${WITH_SYSTEMD}" -eq 0 ]]; then
  cat <<EOF

Installed:
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

if ! command -v ss >/dev/null 2>&1; then
  echo "error: ss is required to verify TCP/5432 ownership; install the iproute2 package and rerun this installer." >&2
  exit 1
fi
if [[ -n "$(ss -H -ltn 'sport = :5432')" ]]; then
  managed_gateway_id=""
  if command -v docker >/dev/null 2>&1; then
    if ! managed_gateway_id="$(docker ps \
      --filter 'label=dev.hostforge.managed=true' \
      --filter 'label=dev.hostforge.resource-type=database-gateway-container' \
      --filter 'label=dev.hostforge.database-gateway-engine=postgresql' \
      --filter 'publish=5432' \
      --format '{{.ID}}' 2>/dev/null)"; then
      managed_gateway_id=""
    fi
  fi
  if [[ -z "${managed_gateway_id}" ]]; then
    echo "error: TCP/5432 is occupied by a listener HostForge does not own; setup cannot safely reserve the PostgreSQL gateway port." >&2
    exit 1
  fi
  echo "TCP/5432 is already owned by the active HostForge PostgreSQL gateway; continuing the idempotent update."
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
  if [[ "${INTERACTIVE}" -eq 1 ]]; then
    if ! command -v openssl >/dev/null 2>&1; then
      echo "error: openssl is required for interactive secret generation." >&2
      exit 1
    fi
    read -r -s -p "Choose HostForge admin login secret (minimum 16 characters): " admin_secret
    echo
    read -r -s -p "Confirm HostForge admin login secret: " admin_secret_confirm
    echo
    if [[ "${#admin_secret}" -lt 16 ]]; then
      echo "error: admin login secret must be at least 16 characters." >&2
      exit 1
    fi
    if [[ "${admin_secret}" != "${admin_secret_confirm}" ]]; then
      echo "error: admin login secrets do not match." >&2
      exit 1
    fi
    session_secret="$(openssl rand -base64 48 | tr -d '\n')"
    webhook_secret="$(openssl rand -hex 32)"
    encryption_key="$(openssl rand -base64 32 | tr -d '\n')"
    env_tmp="$(mktemp)"
    cat >"${env_tmp}" <<EOF
# Created by HostForge interactive installer. Keep this file private.
HOSTFORGE_API_TOKEN=${admin_secret}
HOSTFORGE_SESSION_SECRET=${session_secret}
HOSTFORGE_WEBHOOK_SECRET=${webhook_secret}
HOSTFORGE_ENV_ENCRYPTION_KEY=${encryption_key}
HOSTFORGE_LISTEN=127.0.0.1:8080
HOSTFORGE_DATA_DIR=${DATA_DIR}
HOSTFORGE_WEBHOOK_RATE_LIMIT_PER_MINUTE=60
EOF
    install -m 0640 -o root -g hostforge "${env_tmp}" "${ENV_FILE}"
    rm -f "${env_tmp}"
    unset admin_secret admin_secret_confirm session_secret webhook_secret encryption_key
    echo "Created ${ENV_FILE} with generated secrets."
  elif [[ -f "${ENV_EXAMPLE}" ]]; then
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
WorkingDirectory=${REPO_ROOT}
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
if getent group caddy >/dev/null 2>&1; then
  echo "Adding hostforge to caddy group for managed route snippets..."
  usermod -aG caddy hostforge
  bash "${REPO_ROOT}/scripts/migrate-caddy-layout.sh"
  CADDY_STORAGE_ROOT="$(hostforge_read_env_value "${ENV_FILE}" HOSTFORGE_CADDY_STORAGE_ROOT)"
  if [[ -z "${CADDY_STORAGE_ROOT}" ]]; then
    CADDY_STORAGE_ROOT="/var/lib/caddy/.local/share/caddy"
  elif [[ "${CADDY_STORAGE_ROOT}" != /* ]]; then
    echo "error: HOSTFORGE_CADDY_STORAGE_ROOT must be an absolute path so installer ACLs can be scoped safely." >&2
    exit 1
  fi
  CADDY_CERTIFICATE_ROOT="${CADDY_STORAGE_ROOT}/certificates"
  if ! getent passwd caddy >/dev/null 2>&1; then
    echo "error: the caddy group exists but the caddy service account is unavailable." >&2
    exit 1
  fi
  # Establish the default ACL before Caddy writes or renews gateway material.
  install -d -m 0700 -o caddy -g caddy "${CADDY_CERTIFICATE_ROOT}"
  if [[ -d "${CADDY_CERTIFICATE_ROOT}" ]]; then
    if ! command -v setfacl >/dev/null 2>&1; then
      echo "error: the acl package is required to grant HostForge read-only access to the reserved Caddy certificate pair." >&2
      echo "Install it with 'apt-get install acl' and rerun this installer." >&2
      exit 1
    fi
    echo "Granting hostforge narrow read-only traversal of Caddy certificate storage..."
    current_path="${CADDY_STORAGE_ROOT}"
    while [[ "${current_path}" != "/var/lib" && "${current_path}" != "/" ]]; do
      setfacl -m u:hostforge:--x "${current_path}"
      current_path="$(dirname "${current_path}")"
    done
    setfacl -R -m u:hostforge:rX "${CADDY_CERTIFICATE_ROOT}"
    setfacl -d -m u:hostforge:rX "${CADDY_CERTIFICATE_ROOT}"
    if ! runuser -u hostforge -- test -r "${CADDY_CERTIFICATE_ROOT}" || ! runuser -u hostforge -- test -x "${CADDY_CERTIFICATE_ROOT}"; then
      echo "error: HostForge cannot read and traverse ${CADDY_CERTIFICATE_ROOT} after ACL setup." >&2
      exit 1
    fi
    hostforge_ensure_env_default "${ENV_FILE}" HOSTFORGE_CADDY_STORAGE_ROOT "${CADDY_STORAGE_ROOT}"
    chown root:hostforge "${ENV_FILE}"
    chmod 0640 "${ENV_FILE}"
    echo "Caddy certificate discovery root is configured at ${CADDY_STORAGE_ROOT}."
  fi
fi

if command -v ufw >/dev/null 2>&1; then
  echo "Reserving UFW TCP/5432 for the opt-in HostForge PostgreSQL gateway..."
  ufw allow 5432/tcp >/dev/null
else
  echo "NOTICE: UFW is not installed. Allow inbound TCP/5432 manually before enabling the PostgreSQL gateway."
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
