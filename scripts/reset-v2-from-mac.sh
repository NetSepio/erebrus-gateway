#!/usr/bin/env bash
# Wipe v2 gateway Postgres subscription data from your Mac via SSH.
#
# Examples:
#   bash scripts/reset-v2-from-mac.sh --subscriptions
#   WALLET=YourSolanaPubkey bash scripts/reset-v2-from-mac.sh --wallet-trial
#   FORCE=1 bash scripts/reset-v2-from-mac.sh --all
#
# After wipe: sign out + sign in again in the VPN app, then tap START FREE TRIAL.

set -euo pipefail

SERVER_HOST="${SERVER_HOST:-212.147.232.36}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH_USER="${SSH_USER:-root}"
GATEWAY_DIR="${GATEWAY_DIR:-/opt/erebrus-gateway}"
MODE="${1:---subscriptions}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

ssh_base() {
  ssh -o BatchMode=yes -o ConnectTimeout=20 -i "$SSH_KEY" "$@"
}

[[ -f "$SSH_KEY" ]] || { echo "SSH key missing: $SSH_KEY" >&2; exit 1; }

scp -i "$SSH_KEY" -o BatchMode=yes \
  "$SCRIPT_DIR/reset-v2-db.sh" \
  "${SSH_USER}@${SERVER_HOST}:/tmp/reset-v2-db.sh"

ssh_base "${SSH_USER}@${SERVER_HOST}" \
  "chmod +x /tmp/reset-v2-db.sh && GATEWAY_DIR='${GATEWAY_DIR}' WALLET='${WALLET:-}' FORCE='${FORCE:-}' bash /tmp/reset-v2-db.sh '${MODE}'"

echo ""
echo "Restarting gateway..."
ssh_base "${SSH_USER}@${SERVER_HOST}" \
  "cd '${GATEWAY_DIR}' && docker compose restart gateway"

echo "Done. In the VPN app: Log out → sign in → Settings → START FREE TRIAL."