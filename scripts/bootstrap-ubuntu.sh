#!/usr/bin/env bash
# HostForge Ubuntu 24.04 bootstrapper. Run as root on a fresh VPS.
#
# Example:
#   curl -fsSL https://raw.githubusercontent.com/furious-fury/HostForge/main/scripts/bootstrap-ubuntu.sh | sudo bash
#
# The control plane remains loopback-bound. Complete initial access through an
# SSH tunnel until a hostname is configured; do not expose password login over
# unauthenticated HTTP on a public IP address.
set -euo pipefail

REPOSITORY="https://github.com/furious-fury/HostForge.git"
REF="main"
INSTALL_DIR="/opt/hostforge"
RAILPACK_VERSION="v0.23.0"
BUILDKIT_IMAGE="moby/buildkit@sha256:0168606be2315b7c807a03b3d8aa79beefdb31c98740cebdffdfeebf31190c9f"
RAILPACK_FRONTEND="ghcr.io/railwayapp/railpack-frontend@sha256:ba4c430961d9ee3215c64807727a4b11e2198daac31250e9db9eaf9cee4624d6"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: run as root (for example: sudo bash bootstrap-ubuntu.sh)" >&2
  exit 1
fi
if ! grep -q '^ID=ubuntu' /etc/os-release; then
  echo "error: this bootstrapper currently supports Ubuntu only." >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y acl ca-certificates curl git gnupg iproute2 ufw fail2ban snapd
if [[ -n "$(ss -H -ltn 'sport = :5432')" ]]; then
  echo "error: TCP/5432 is already occupied; remove or reconfigure the existing listener before installing HostForge." >&2
  exit 1
fi
ufw allow OpenSSH
ufw allow 443/tcp
# Reserve the single PostgreSQL gateway ingress. No process listens here until
# the feature is explicitly enabled and its fail-closed checks pass.
ufw allow 5432/tcp
ufw --force enable
systemctl enable --now fail2ban

install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg
source /etc/os-release
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" >/etc/apt/sources.list.d/docker.list
curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin nodejs
systemctl enable --now docker

apt-get install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/gpg.key | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt >/etc/apt/sources.list.d/caddy-stable.list
chmod o+r /usr/share/keyrings/caddy-stable-archive-keyring.gpg /etc/apt/sources.list.d/caddy-stable.list
apt-get update
apt-get install -y caddy

if ! command -v go >/dev/null 2>&1; then
  snap install go --classic
  ln -sfn /snap/bin/go /usr/local/bin/go
fi

docker pull "${BUILDKIT_IMAGE}"
docker pull "${RAILPACK_FRONTEND}"
cat >/etc/systemd/system/buildkitd.service <<EOF
[Unit]
Description=HostForge BuildKit daemon
After=docker.service
Requires=docker.service
[Service]
ExecStartPre=-/usr/bin/docker rm -f buildkitd
ExecStart=/usr/bin/docker run --name buildkitd --privileged -v /run/buildkit:/run/buildkit ${BUILDKIT_IMAGE} --addr unix:///run/buildkit/buildkitd.sock
ExecStop=/usr/bin/docker stop buildkitd
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now buildkitd

if [[ ! -x /root/.local/bin/mise ]]; then
  curl https://mise.run | sh
fi
/root/.local/bin/mise use -g "github:railwayapp/railpack@${RAILPACK_VERSION}"
RAILPACK_REAL="$(/root/.local/bin/mise which railpack)"
install -d -m 0755 /usr/local/lib/hostforge
install -m 0755 "${RAILPACK_REAL}" /usr/local/lib/hostforge/railpack
ln -sfn /usr/local/lib/hostforge/railpack /usr/local/bin/railpack
cat >/usr/local/bin/buildctl <<EOF
#!/bin/sh
exec /usr/bin/docker run --rm --entrypoint buildctl -v /run/buildkit:/run/buildkit -v /var/lib/hostforge:/var/lib/hostforge:ro ${BUILDKIT_IMAGE} "\$@"
EOF
chmod 0755 /usr/local/bin/buildctl

if [[ -d "${INSTALL_DIR}/.git" ]]; then
  git -C "${INSTALL_DIR}" fetch --depth 1 origin "${REF}"
  git -C "${INSTALL_DIR}" checkout -B "${REF}" "origin/${REF}"
else
  git clone --depth 1 --branch "${REF}" "${REPOSITORY}" "${INSTALL_DIR}"
fi
cd "${INSTALL_DIR}"
./scripts/install.sh --with-systemd --interactive
usermod -aG docker hostforge
cat >>/etc/hostforge/hostforge.env <<EOF

# Provisioned build runtime (pinned by bootstrap-ubuntu.sh).
HOSTFORGE_RAILPACK_ENABLED=true
HOSTFORGE_RAILPACK_BIN=/usr/local/bin/railpack
HOSTFORGE_RAILPACK_VERSION=${RAILPACK_VERSION}
HOSTFORGE_RAILPACK_FRONTEND_IMAGE=${RAILPACK_FRONTEND}
HOSTFORGE_BUILDKIT_BIN=/usr/local/bin/buildctl
HOSTFORGE_BUILDKIT_ADDRESS=unix:///run/buildkit/buildkitd.sock
HOSTFORGE_RAILPACK_ARTIFACTS_DIR=/var/lib/hostforge/railpack
HOSTFORGE_RAILPACK_BUILD_CONCURRENCY=1
HOSTFORGE_RAILPACK_MIN_FREE_DISK_BYTES=10737418240
# Database gateways remain disabled for the first staging rollout. Configure a
# digest-pinned PgBouncer >=1.25.2 image before enabling this flag.
HOSTFORGE_DATABASE_GATEWAYS_ENABLED=false
HOSTFORGE_DATABASE_GATEWAY_OPERATION_CONCURRENCY=1
EOF
# Bootstrap ingress is the sole public control-plane listener. Caddy obtains an
# IP-address certificate before HostForge is started; there is no HTTP fallback.
PUBLIC_IP="$(curl -4fsS https://api.ipify.org)"
BOOTSTRAP_EXPIRES_AT="$(date -u -d '+24 hours' +%Y-%m-%dT%H:%M:%SZ)"
usermod -aG caddy hostforge
# Grant HostForge read/traverse access only to Caddy's certificate subtree so
# it can validate and synchronize the reserved gateway SAN/key pair. Default
# ACLs carry the narrow permission onto renewed certificate files.
install -d -m 0700 -o caddy -g caddy /var/lib/caddy/.local/share/caddy/certificates
setfacl -R -m u:hostforge:rX /var/lib/caddy/.local/share/caddy/certificates
setfacl -d -m u:hostforge:rX /var/lib/caddy/.local/share/caddy/certificates
install -d -m 2770 -o root -g caddy /etc/caddy/hostforge.d
cat >/etc/caddy/hostforge.d/control-plane.caddy <<EOF
# generated by hostforge bootstrap; HTTPS only
https://${PUBLIC_IP} {
    tls
    reverse_proxy 127.0.0.1:8080
}
EOF
chown root:caddy /etc/caddy/hostforge.d/control-plane.caddy
chmod 0640 /etc/caddy/hostforge.d/control-plane.caddy
install -m 0640 -o root -g caddy /dev/null /etc/caddy/hostforge.d/routes.caddy
cat >/etc/caddy/Caddyfile <<EOF
{
    auto_https disable_redirects
}

# HostForge control-plane and deployment routes are managed in writable imports.
import /etc/caddy/hostforge.d/control-plane.caddy
import /etc/caddy/hostforge.d/routes.caddy
EOF
cat >>/etc/hostforge/hostforge.env <<EOF
HOSTFORGE_BOOTSTRAP_ENABLED=true
HOSTFORGE_BOOTSTRAP_PUBLIC_IP=${PUBLIC_IP}
HOSTFORGE_BOOTSTRAP_HTTPS_PORT=443
HOSTFORGE_BOOTSTRAP_EXPIRES_AT=${BOOTSTRAP_EXPIRES_AT}
HOSTFORGE_CADDY_ROOT_CONFIG=/etc/caddy/Caddyfile
HOSTFORGE_CADDY_CONTROL_PLANE_PATH=/etc/caddy/hostforge.d/control-plane.caddy
HOSTFORGE_CADDY_GENERATED_PATH=/etc/caddy/hostforge.d/routes.caddy
HOSTFORGE_CADDY_STORAGE_ROOT=/var/lib/caddy/.local/share/caddy
HOSTFORGE_SYNC_CADDY=true
HOSTFORGE_SESSION_COOKIE_SECURE=true
EOF
caddy validate --config /etc/caddy/Caddyfile
systemctl restart caddy
# A successful TLS handshake is the certificate-provisioning gate. Do not start
# HostForge if Caddy cannot obtain an IP certificate.
for attempt in $(seq 1 24); do
  if curl --silent --show-error --output /dev/null --connect-timeout 5 "https://${PUBLIC_IP}/"; then
    break
  fi
  if [[ "${attempt}" -eq 24 ]]; then
    systemctl stop caddy
    echo "error: bootstrap IP HTTPS certificate was not provisioned; HostForge was not started." >&2
    exit 1
  fi
  sleep 5
done
systemctl restart hostforge-server

cat <<EOF

HostForge is available over verified bootstrap HTTPS:
  https://${PUBLIC_IP}/

Sign in with the admin secret prompted during installation. Configure the GitHub App and permanent platform domain in onboarding. Once permanent HTTPS validates, HostForge atomically removes the IP bootstrap route.
EOF
