#!/usr/bin/env bash
# Redeploy Erebrus gateway v2 on the remote host (212.147.232.36 or SSH target).
# Run ON THE SERVER from a checkout of erebrus-gateway (v2 branch), or via:
#   ssh user@212.147.232.36 'bash -s' < scripts/deploy-v2-remote.sh
#
# Prerequisites on server:
#   - Docker
#   - .env in GATEWAY_DIR with MNEMONIC, DB_*, REDIS_HOST (do NOT rotate MNEMONIC)
#   - Postgres + Redis reachable from the gateway container
#
# Optional env overrides:
#   GATEWAY_DIR   — directory with .env (default: ~/gateway-v2)
#   GATEWAY_SRC   — erebrus-gateway checkout to build (skips docker pull)
#   GATEWAY_IMAGE — image tag (default: ghcr.io/netsepio/gateway:v2)
#   GATEWAY_PORT  — host port (default: 8080)
#   DOCKER_NETWORK — attach to existing compose network (default: netsepio_prod_network)
#   RESTART_NODE  — set to 1 to restart /opt/erebrus after gateway (default: 1)

set -euo pipefail

GATEWAY_DIR="${GATEWAY_DIR:-$HOME/gateway-v2}"
GATEWAY_IMAGE="${GATEWAY_IMAGE:-ghcr.io/netsepio/gateway:v2}"
GATEWAY_PORT="${GATEWAY_PORT:-8080}"
CONTAINER_NAME="${CONTAINER_NAME:-erebrus-gateway-v2}"
DOCKER_NETWORK="${DOCKER_NETWORK:-netsepio_prod_network}"
RESTART_NODE="${RESTART_NODE:-1}"

log() { printf '[deploy-v2] %s\n' "$*"; }
die() { printf '[deploy-v2] ERROR: %s\n' "$*" >&2; exit 1; }

if [[ ! -f "$GATEWAY_DIR/.env" ]]; then
  die "Missing $GATEWAY_DIR/.env — copy from .env.example and set MNEMONIC + DB_*"
fi

# Never deploy without a stable signer; ephemeral keys invalidate all tokens.
if ! grep -qE '^MNEMONIC=.+' "$GATEWAY_DIR/.env" && ! grep -qE '^PASETO_PRIVATE_KEY=.+' "$GATEWAY_DIR/.env"; then
  die ".env must set MNEMONIC or PASETO_PRIVATE_KEY before redeploy"
fi

if [[ -n "${GATEWAY_SRC:-}" ]]; then
  [[ -d "$GATEWAY_SRC" ]] || die "GATEWAY_SRC not a directory: $GATEWAY_SRC"
  [[ -f "$GATEWAY_SRC/Dockerfile.v2" ]] || die "Missing $GATEWAY_SRC/Dockerfile.v2"
  log "Building $GATEWAY_IMAGE from $GATEWAY_SRC"
  docker build -f "$GATEWAY_SRC/Dockerfile.v2" -t "$GATEWAY_IMAGE" "$GATEWAY_SRC" \
    || die "docker build failed"
else
  log "Pulling $GATEWAY_IMAGE"
  docker pull "$GATEWAY_IMAGE" || die "docker pull failed — set GATEWAY_SRC to build locally"
fi

log "Stopping old container (if any)"
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

log "Starting gateway on :$GATEWAY_PORT (network: $DOCKER_NETWORK)"
NET_ARGS=()
if docker network inspect "$DOCKER_NETWORK" >/dev/null 2>&1; then
  NET_ARGS=(--network "$DOCKER_NETWORK")
else
  log "Docker network $DOCKER_NETWORK not found — starting without --network"
fi

docker run -d \
  --name "$CONTAINER_NAME" \
  --restart unless-stopped \
  "${NET_ARGS[@]}" \
  --env-file "$GATEWAY_DIR/.env" \
  -p "${GATEWAY_PORT}:8080" \
  -p 9001:9001 \
  "$GATEWAY_IMAGE"

log "Waiting for /healthz"
for i in $(seq 1 30); do
  if curl -fsS --max-time 2 "http://127.0.0.1:${GATEWAY_PORT}/healthz" >/dev/null 2>&1; then
    log "Gateway healthy"
    curl -fsS "http://127.0.0.1:${GATEWAY_PORT}/healthz" || true
    echo
    break
  fi
  sleep 1
  if [[ "$i" -eq 30 ]]; then
    docker logs --tail 80 "$CONTAINER_NAME" || true
    die "Gateway did not become healthy within 30s"
  fi
done

log "Node directory (public)"
curl -fsS "http://127.0.0.1:${GATEWAY_PORT}/api/v2/nodes" | head -c 400 || true
echo

if [[ "$RESTART_NODE" == "1" ]] && [[ -d /opt/erebrus ]]; then
  log "Restarting erebrus node at /opt/erebrus"
  (cd /opt/erebrus && docker compose restart) || log "Node restart skipped (compose not running)"
  sleep 5
  curl -fsS "http://127.0.0.1:${GATEWAY_PORT}/api/v2/nodes" | head -c 400 || true
  echo
fi

log "Done — gateway v2 on port $GATEWAY_PORT"