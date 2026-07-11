#!/usr/bin/env bash
# Expose local HostForge (API + UI + webhooks) on a public HTTPS URL via ngrok free tier.
#
# Prereqs:
#   1) Install ngrok: https://ngrok.com/download
#   2) One-time: ngrok config add-authtoken <token>   (from https://dashboard.ngrok.com/get-started/your-authtoken)
#   3) HostForge server running (e.g. source .hostforge.env && go run ./cmd/server)
#
# Usage (from repo root):
#   ./scripts/ngrok-dev.sh
#   NGROK_REGION=eu ./scripts/ngrok-dev.sh
#
# GitHub webhook URL (default path):  https://<your-subdomain>.ngrok-free.app/hooks/github
# Free tier: URL changes when the tunnel restarts unless you use a paid reserved domain.

set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ -f .hostforge.env ]]; then
	set -a
	# shellcheck disable=SC1091
	source .hostforge.env
	set +a
fi

LISTEN="${HOSTFORGE_LISTEN:-127.0.0.1:8080}"
PORT="${LISTEN##*:}"
if ! [[ "${PORT}" =~ ^[0-9]+$ ]]; then
	PORT=8080
fi

if ! command -v ngrok >/dev/null 2>&1; then
	echo "error: ngrok not found in PATH. Install from https://ngrok.com/download" >&2
	exit 1
fi

echo "ngrok: tunneling https -> http://127.0.0.1:${PORT}"
echo "       (start HostForge on HOSTFORGE_LISTEN if it is not already running)"
echo ""

REGION_ARGS=()
if [[ -n "${NGROK_REGION:-}" ]]; then
	REGION_ARGS=(--region "${NGROK_REGION}")
fi

exec ngrok http "127.0.0.1:${PORT}" --log=stdout "${REGION_ARGS[@]}"
