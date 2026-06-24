#!/usr/bin/env bash
# Reset v2 gateway Postgres data (subscriptions and/or full wipe).
#
# Run ON THE SERVER (default GATEWAY_DIR=/opt/erebrus-gateway):
#   bash scripts/reset-v2-db.sh --subscriptions
#   bash scripts/reset-v2-db.sh --all
#   WALLET=YourSolanaAddress bash scripts/reset-v2-db.sh --wallet-trial
#
# From Mac via SSH:
#   ssh root@212.147.232.36 'GATEWAY_DIR=/opt/erebrus-gateway bash -s -- --subscriptions' \
#     < scripts/reset-v2-db.sh

set -euo pipefail

GATEWAY_DIR="${GATEWAY_DIR:-/opt/erebrus-gateway}"
MODE="${1:-}"

log() { printf '[reset-v2-db] %s\n' "$*"; }
die() { printf '[reset-v2-db] ERROR: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage:
  reset-v2-db.sh --subscriptions   Delete all rows in subscriptions (keeps users)
  reset-v2-db.sh --wallet-trial    Delete trial row for WALLET env var only
  reset-v2-db.sh --all             TRUNCATE users CASCADE (full v2 identity wipe)
  reset-v2-db.sh --status          Show subscription counts

Env:
  GATEWAY_DIR   compose project dir (default: /opt/erebrus-gateway)
  WALLET        Solana wallet address for --wallet-trial
EOF
  exit 1
}

[[ -n "$MODE" ]] || usage

cd "$GATEWAY_DIR" || die "Missing $GATEWAY_DIR"

compose_file() {
  for f in docker-compose.yml docker-compose.yaml; do
    [[ -f "$f" ]] && echo "$f" && return 0
  done
  return 1
}

CF="$(compose_file)" || die "No docker-compose file in $GATEWAY_DIR"
COMPOSE=(docker compose -f "$CF")

pg() {
  "${COMPOSE[@]}" exec -T postgres psql -U erebrus -d erebrus -v ON_ERROR_STOP=1 "$@"
}

log "Using compose file: $CF"

case "$MODE" in
  --status)
    pg -c "SELECT count(*) AS users FROM users;"
    pg -c "SELECT count(*) AS subscriptions FROM subscriptions;"
    pg -c "SELECT u.wallet_address, s.source, s.status, s.current_period_end
           FROM subscriptions s
           JOIN users u ON u.id = s.user_id
           ORDER BY s.created_at DESC
           LIMIT 20;"
    ;;
  --subscriptions)
    log "Deleting ALL subscription rows (trials can be started again)"
    pg -c "DELETE FROM subscriptions;"
    pg -c "SELECT count(*) AS remaining_subscriptions FROM subscriptions;"
    ;;
  --wallet-trial)
    [[ -n "${WALLET:-}" ]] || die "Set WALLET=<solana address> for --wallet-trial"
    log "Deleting trial for wallet: $WALLET"
    pg -c "DELETE FROM subscriptions
           WHERE source = 'trial'
             AND user_id = (SELECT id FROM users WHERE wallet_address = '$WALLET');"
    pg -c "SELECT u.wallet_address, s.source, s.status, s.current_period_end
           FROM users u
           LEFT JOIN subscriptions s ON s.user_id = u.id
           WHERE u.wallet_address = '$WALLET';"
    ;;
  --all)
    log "TRUNCATE users CASCADE — wipes users, subscriptions, vpn_clients, etc."
    if [[ "${FORCE:-}" != "1" ]]; then
      read -r -p "Type YES to wipe all v2 user data: " confirm
      [[ "$confirm" == "YES" ]] || die "Aborted"
    fi
    pg -c "TRUNCATE users CASCADE;"
    pg -c "SELECT count(*) AS users FROM users; SELECT count(*) AS subscriptions FROM subscriptions;"
    ;;
  *)
    usage
    ;;
esac

restart_gateway() {
  if "${COMPOSE[@]}" ps --status running gateway &>/dev/null; then
    "${COMPOSE[@]}" restart gateway
    return 0
  fi
  local gw
  gw=$(docker ps --format '{{.Names}}' | grep -iE 'gateway' | grep -v postgres | head -1 || true)
  if [[ -n "$gw" ]]; then
    docker restart "$gw"
    return 0
  fi
  log "Could not auto-restart gateway — run: docker ps | grep gateway"
}

log "Done. Restarting gateway..."
restart_gateway || true