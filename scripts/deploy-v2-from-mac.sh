#!/usr/bin/env bash
# Deploy erebrus-gateway v2 to the remote server from your Mac.
#
# Usage:
#   cd /Users/shachindra/Projects/NetSepio/erebrus-gateway
#   bash scripts/deploy-v2-from-mac.sh
#
# Optional env:
#   SERVER_HOST=212.147.232.36
#   SSH_USER=ubuntu          # auto-detected if unset
#   SSH_KEY=~/.ssh/id_ed25519
#   SKIP_COMMIT=1            # skip local git commit
#   SKIP_RSYNC=1             # skip rsync (server already has latest tree)
#   SKIP_NODE_RESTART=1      # don't restart /opt/erebrus on server
#   GATEWAY_DIR=/root/gateway  # set if auto-detect fails (run discover script first)
#   SKIP_COMMIT=1 SKIP_RSYNC=1  # re-run deploy only after rsync succeeded
#
# Paste the full terminal output back to the agent when done.

set -euo pipefail

SERVER_HOST="${SERVER_HOST:-212.147.232.36}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH_USER="${SSH_USER:-}"
GATEWAY_IMAGE="${GATEWAY_IMAGE:-erebrus-gateway:v2-local}"
REPORT_FILE="${REPORT_FILE:-/tmp/erebrus-gateway-v2-deploy-$(date +%Y%m%d-%H%M%S).log}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

log()  { printf '[deploy-mac] %s\n' "$*"; }
step() { printf '\n========== %s ==========\n' "$*"; }

exec > >(tee -a "$REPORT_FILE") 2>&1

log "Report file: $REPORT_FILE"
log "Repo: $REPO_ROOT"
log "Target: $SERVER_HOST"

ssh_base() {
  ssh -o BatchMode=yes -o ConnectTimeout=15 -i "$SSH_KEY" "$@"
}

detect_ssh_user() {
  if [[ -n "$SSH_USER" ]]; then
    return 0
  fi
  for u in ubuntu root ec2-user; do
    log "Probing SSH user: $u"
    if ssh_base "${u}@${SERVER_HOST}" 'echo ok' >/dev/null 2>&1; then
      SSH_USER="$u"
      log "Using SSH user: $SSH_USER"
      return 0
    fi
  done
  echo "ERROR: Could not SSH as ubuntu, root, or ec2-user. Set SSH_USER=..." >&2
  exit 1
}

step "1/6 — SSH check"
[[ -f "$SSH_KEY" ]] || { echo "ERROR: SSH key not found: $SSH_KEY"; exit 1; }
detect_ssh_user
ssh_base "${SSH_USER}@${SERVER_HOST}" 'whoami && hostname && date -u'

step "2/6 — Local git (optional commit)"
cd "$REPO_ROOT"
git branch --show-current || true
git status --short || true
if [[ "${SKIP_COMMIT:-0}" != "1" ]]; then
  if [[ -n "$(git status --porcelain 2>/dev/null || true)" ]]; then
    git add internal/api/subscriptions.go \
            internal/store/subscriptions.go \
            internal/api/vpn.go \
            scripts/deploy-v2-remote.sh \
            scripts/deploy-v2-from-mac.sh \
            .github/workflows/docker-publish.yml 2>/dev/null || true
    if git diff --cached --quiet; then
      log "Nothing staged to commit"
    else
      git commit -m "feat(v2): trial_consumed in subscription GET, admin entitlement bypass"
      log "Committed local changes"
    fi
  else
    log "Working tree clean — no commit"
  fi
else
  log "SKIP_COMMIT=1 — skipping commit"
fi

step "3/6 — rsync discover script (always)"
ssh_base "${SSH_USER}@${SERVER_HOST}" "mkdir -p ~/erebrus-gateway/scripts"
scp -i "$SSH_KEY" -o BatchMode=yes \
  "$REPO_ROOT/scripts/discover-gateway-env.sh" \
  "$REPO_ROOT/scripts/deploy-v2-remote.sh" \
  "${SSH_USER}@${SERVER_HOST}:~/erebrus-gateway/scripts/" 2>/dev/null || \
  rsync -avz -e "ssh -i $SSH_KEY -o BatchMode=yes" \
    "$REPO_ROOT/scripts/discover-gateway-env.sh" \
    "$REPO_ROOT/scripts/deploy-v2-remote.sh" \
    "${SSH_USER}@${SERVER_HOST}:~/erebrus-gateway/scripts/"

step "3b/6 — discover gateway .env on server"
ssh_base "${SSH_USER}@${SERVER_HOST}" 'bash ~/erebrus-gateway/scripts/discover-gateway-env.sh' || true

step "4/6 — rsync full repo"
REMOTE_REPO="~/erebrus-gateway"
if [[ "${SKIP_RSYNC:-0}" != "1" ]]; then
  rsync -avz \
    --exclude '.git' \
    --exclude 'node_modules' \
    --exclude '.env' \
    -e "ssh -i $SSH_KEY -o BatchMode=yes" \
    "$REPO_ROOT/" \
    "${SSH_USER}@${SERVER_HOST}:${REMOTE_REPO}/"
else
  log "SKIP_RSYNC=1 — skipping rsync"
fi

step "5/6 — Remote build + deploy + node restart"
RESTART_NODE=1
[[ "${SKIP_NODE_RESTART:-0}" == "1" ]] && RESTART_NODE=0

ssh_base "${SSH_USER}@${SERVER_HOST}" bash -s <<REMOTE
set -euo pipefail
export GATEWAY_DIR="${GATEWAY_DIR:-}"
if [[ -z "\${GATEWAY_DIR:-}" ]]; then
  GATEWAY_DIR=\$(bash "\$HOME/erebrus-gateway/scripts/discover-gateway-env.sh") || {
    echo ""
    echo "HINT: discover only:"
    echo "  ssh ${SSH_USER}@${SERVER_HOST} 'bash ~/erebrus-gateway/scripts/discover-gateway-env.sh'"
    echo "Then:"
    echo "  GATEWAY_DIR=/path/to/dir SKIP_COMMIT=1 SKIP_RSYNC=1 bash scripts/deploy-v2-from-mac.sh"
    exit 1
  }
fi
echo "Using GATEWAY_DIR=\$GATEWAY_DIR"
export GATEWAY_DIR
export GATEWAY_SRC="\$HOME/erebrus-gateway"
export GATEWAY_IMAGE="${GATEWAY_IMAGE}"
export RESTART_NODE=${RESTART_NODE}
bash "\$HOME/erebrus-gateway/scripts/deploy-v2-remote.sh"
REMOTE

step "6/6 — Public verification (from Mac)"
echo "--- healthz ---"
curl -sS --max-time 10 "http://${SERVER_HOST}:8080/healthz" || echo "healthz FAILED"
echo
echo "--- nodes ---"
curl -sS --max-time 10 "http://${SERVER_HOST}:8080/api/v2/nodes" || echo "nodes FAILED"
echo

step "DONE — paste everything above (or this file) back to the agent"
log "Full log: $REPORT_FILE"
echo ""
echo "Quick copy:"
echo "  cat $REPORT_FILE | pbcopy"