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
apt-get install -y ca-certificates curl git gnupg ufw fail2ban snapd
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
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
EOF
systemctl restart hostforge-server

cat <<'EOF'

HostForge is installed and listening privately on 127.0.0.1:8080.
For first access, run from your computer:
  ssh -N -L 8080:127.0.0.1:8080 root@YOUR_VPS_IP
Then open http://localhost:8080 and sign in as admin with the secret you chose.

Next: configure a hostname in HostForge/Caddy for public HTTPS access.
EOF
