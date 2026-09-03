#!/usr/bin/env bash
# HostForge Ubuntu 24.04 bootstrapper. Run as root on a fresh VPS.
#
# Example:
#   curl -fsSL https://raw.githubusercontent.com/furious-fury/HostForge/main/scripts/bootstrap-ubuntu.sh | sudo bash
#
# Installs the latest published release: a checksum-verified prebuilt binary
# and UI, downloaded over plain HTTPS. Nothing is compiled here, and git is
# not used or required -- the host needs neither the Go nor the Node
# toolchain to run HostForge, only to build it.
#
# To install unreleased code from a branch instead, which does clone and
# build and therefore does install Go and Node:
#   curl -fsSL .../scripts/bootstrap-ubuntu.sh | sudo bash -s -- --from-source
#   HOSTFORGE_REF=my-branch sudo bash bootstrap-ubuntu.sh
#
# Set HOSTFORGE_VERSION=vX.Y.Z to pin a specific release instead of the
# latest one.
#
# The control plane remains loopback-bound. Complete initial access through an
# SSH tunnel until a hostname is configured; do not expose password login over
# unauthenticated HTTP on a public IP address.
set -euo pipefail

REPOSITORY="https://github.com/furious-fury/HostForge.git"
GITHUB_REPO="furious-fury/HostForge"
INSTALL_DIR="/opt/hostforge"
RAILPACK_VERSION="v0.23.0"
BUILDKIT_IMAGE="moby/buildkit@sha256:0168606be2315b7c807a03b3d8aa79beefdb31c98740cebdffdfeebf31190c9f"
RAILPACK_FRONTEND="ghcr.io/railwayapp/railpack-frontend@sha256:ba4c430961d9ee3215c64807727a4b11e2198daac31250e9db9eaf9cee4624d6"

FROM_SOURCE=0
for arg in "$@"; do
  case "${arg}" in
    --from-source)
      FROM_SOURCE=1
      ;;
    -h|--help)
      sed -n '2,22p' "$0" | sed -n '/^#/s/^# \{0,1\}//p'
      exit 0
      ;;
    *)
      echo "unknown option: ${arg}" >&2
      exit 2
      ;;
  esac
done
# Asking for a specific ref only makes sense against a checkout, so it
# selects source mode on its own.
if [[ -n "${HOSTFORGE_REF:-}" ]]; then
  FROM_SOURCE=1
fi
REF="${HOSTFORGE_REF:-main}"

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
# git is deliberately absent: a release install fetches over plain HTTPS and
# never invokes it. Source mode installs it below, alongside the toolchains
# only that mode needs.
apt-get install -y acl ca-certificates curl gnupg iproute2 ufw fail2ban snapd
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
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker

# Node is a build dependency of the web UI, not a runtime dependency of the
# server: application builds run through Railpack and BuildKit inside Docker,
# and the server itself only ever executes caddy, railpack, and buildctl. A
# release install ships a prebuilt web/dist, so nothing here needs it.
if [[ "${FROM_SOURCE}" -eq 1 ]]; then
  apt-get install -y git
  curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
  apt-get update
  apt-get install -y nodejs
fi

apt-get install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/gpg.key | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt >/etc/apt/sources.list.d/caddy-stable.list
chmod o+r /usr/share/keyrings/caddy-stable-archive-keyring.gpg /etc/apt/sources.list.d/caddy-stable.list
apt-get update
apt-get install -y caddy

# Likewise Go: only a source build needs a compiler on the host.
if [[ "${FROM_SOURCE}" -eq 1 ]] && ! command -v go >/dev/null 2>&1; then
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

# resolve_release_tag prints the release to install: HOSTFORGE_VERSION if
# pinned, otherwise the latest published tag.
#
# scripts/install.sh has the same lookup, but it cannot be reused here: the
# tag is needed to fetch the tree that install.sh itself lives in.
resolve_release_tag() {
  if [[ -n "${HOSTFORGE_VERSION:-}" ]]; then
    printf '%s' "${HOSTFORGE_VERSION}"
    return
  fi
  echo "Looking up the latest HostForge release..." >&2
  local tag
  tag="$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
    | sed -n 's/^ *"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
  if [[ -z "${tag}" ]]; then
    echo "error: could not resolve the latest HostForge release from the GitHub API." >&2
    echo "Pin one with HOSTFORGE_VERSION=vX.Y.Z, or install unreleased code with --from-source." >&2
    exit 1
  fi
  printf '%s' "${tag}"
}

# fetch_source_tree downloads the repository at a tag as a tarball over plain
# HTTPS, rather than cloning it.
#
# This is not a stylistic preference. git's smart-HTTP transport POSTs to
# /git-upload-pack, and GitHub's anti-abuse layer intermittently answers that
# POST with a 401 from data-centre IP ranges -- which git surfaces as a
# username prompt on a public repository, mid-install. The archive endpoint
# is an ordinary GET and is not subject to it. It is also the same request
# shape that fetched this script.
fetch_source_tree() {
  local tag="$1" parent tmp extracted old
  # Staged beside INSTALL_DIR so the swap below is a rename on one
  # filesystem. From /tmp it could be a cross-device copy, which can fail
  # halfway and leave the host with no install tree at all.
  parent="$(dirname "${INSTALL_DIR}")"
  install -d -m 0755 "${parent}"
  tmp="$(mktemp -d "${parent}/.hostforge-install.XXXXXX")"
  old="${INSTALL_DIR}.replaced.$$"
  echo "Downloading HostForge ${tag} source..."
  if ! curl -fsSL "https://github.com/${GITHUB_REPO}/archive/refs/tags/${tag}.tar.gz" \
    | tar -xz -C "${tmp}"; then
    rm -rf "${tmp}"
    echo "error: could not download the HostForge ${tag} source archive." >&2
    exit 1
  fi
  extracted="$(find "${tmp}" -mindepth 1 -maxdepth 1 -type d | head -n1)"
  if [[ -z "${extracted}" || ! -f "${extracted}/scripts/install.sh" ]]; then
    rm -rf "${tmp}"
    echo "error: the HostForge ${tag} source archive did not contain scripts/install.sh." >&2
    exit 1
  fi
  # Replace the old tree only once the new one is known good, and move it
  # aside rather than deleting it, so an interrupted swap leaves something to
  # recover from. Data lives in /var/lib/hostforge and /etc/hostforge and is
  # untouched by any of this.
  if [[ -d "${INSTALL_DIR}" ]]; then
    mv "${INSTALL_DIR}" "${old}"
  fi
  if ! mv "${extracted}" "${INSTALL_DIR}"; then
    [[ -d "${old}" ]] && mv "${old}" "${INSTALL_DIR}"
    rm -rf "${tmp}"
    echo "error: could not install the ${tag} tree at ${INSTALL_DIR}." >&2
    exit 1
  fi
  rm -rf "${tmp}" "${old}"
}

if [[ "${FROM_SOURCE}" -eq 1 ]]; then
  if [[ -d "${INSTALL_DIR}/.git" ]]; then
    git -C "${INSTALL_DIR}" fetch --depth 1 origin "${REF}"
    git -C "${INSTALL_DIR}" checkout -B "${REF}" "origin/${REF}"
  else
    git clone --depth 1 --branch "${REF}" "${REPOSITORY}" "${INSTALL_DIR}"
  fi
  cd "${INSTALL_DIR}"
  ./scripts/install.sh --with-systemd --interactive
else
  RELEASE_TAG="$(resolve_release_tag)"
  fetch_source_tree "${RELEASE_TAG}"
  cd "${INSTALL_DIR}"
  # Pinned so the scripts and the binary are the same release, rather than
  # letting install.sh resolve "latest" a second time and possibly land on a
  # newer tag published between the two lookups.
  HOSTFORGE_VERSION="${RELEASE_TAG}" ./scripts/install.sh \
    --with-systemd --interactive --download-release
fi

# The server resolves the UI from "web/dist" relative to its working
# directory, which is INSTALL_DIR, and starts happily without it -- one
# warning in the journal and an API with no UI. install.sh checks this on
# every path it takes; check it again here, because the silent version of
# this failure is the expensive one to diagnose.
if [[ ! -f "${INSTALL_DIR}/web/dist/index.html" ]]; then
  echo "error: ${INSTALL_DIR}/web/dist/index.html is missing after install; the UI would not be served." >&2
  exit 1
fi

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
HOSTFORGE_RAILPACK_MIN_FREE_DISK_BYTES=10737418240
HOSTFORGE_DEPLOY_CONCURRENCY=2
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

# `tls internal`, not a bare `tls`: Caddy 2.11 rejects the directive with no
# argument outright, so `caddy validate` below fails and -- under set -e --
# the bootstrapper dies before ever starting HostForge, leaving an enabled
# service that was never started.
#
# `internal` is also the honest choice for a raw IP. There is no public CA
# issuance here, so this is Caddy's own local CA and browsers will warn until
# a real domain is configured in onboarding.
cat >/etc/caddy/hostforge.d/control-plane.caddy <<EOF
# generated by hostforge bootstrap; HTTPS only, internal CA on a raw IP
https://${PUBLIC_IP} {
    tls internal
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
# HostForge if Caddy cannot serve HTTPS at all.
#
# --insecure is required and is not a weakening of this check. The certificate
# is issued by Caddy's internal CA, which this host does not trust by
# construction, so a verifying curl fails every attempt no matter how healthy
# Caddy is -- the gate would stop Caddy and abort a working install. What is
# being proven here is that Caddy is listening and completing a TLS handshake
# on 443, which is exactly what --insecure still proves.
for attempt in $(seq 1 24); do
  if curl --silent --show-error --insecure --output /dev/null --connect-timeout 5 "https://${PUBLIC_IP}/"; then
    break
  fi
  if [[ "${attempt}" -eq 24 ]]; then
    systemctl stop caddy
    echo "error: Caddy did not serve HTTPS on ${PUBLIC_IP}; HostForge was not started." >&2
    exit 1
  fi
  sleep 5
done
systemctl restart hostforge-server

cat <<EOF

HostForge is available over bootstrap HTTPS:
  https://${PUBLIC_IP}/

This certificate is issued by Caddy's internal CA, because a raw IP address
cannot get a publicly trusted one. Your browser will warn on first visit;
that is expected here and goes away once a real domain is configured.

Sign in with the admin secret prompted during installation. Configure the GitHub App and permanent platform domain in onboarding. Once permanent HTTPS validates, HostForge atomically removes the IP bootstrap route.
EOF
