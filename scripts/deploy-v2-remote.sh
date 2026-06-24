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

# Stop any container (running or created) bound to GATEWAY_PORT.
stop_port_holders() {
  local port="$1"
  local ids=()
  while IFS= read -r id; do
    [[ -n "$id" ]] && ids+=("$id")
  done < <(docker ps -aq --filter "publish=${port}" 2>/dev/null || true)

  if [[ ${#ids[@]} -eq 0 ]]; then
    while IFS= read -r id; do
      [[ -n "$id" ]] && ids+=("$id")
    done < <(docker ps -aq --format '{{.ID}} {{.Ports}}' 2>/dev/null | awk -v p=":${port}->" '$0 ~ p {print $1}')
  fi

  for id in "${ids[@]}"; do
    local name
    name=$(docker inspect -f '{{.Name}}' "$id" 2>/dev/null | sed 's#^/##' || echo "$id")
    log "Stopping container on :${port}: $name"
    docker rm -f "$id" || true
  done
}

# Prefer compose network from GATEWAY_DIR, else postgres container network.
detect_docker_network() {
  if docker network inspect "$DOCKER_NETWORK" >/dev/null 2>&1; then
    echo "$DOCKER_NETWORK"
    return 0
  fi
  if [[ -f "$GATEWAY_DIR/docker-compose.yml" ]] || [[ -f "$GATEWAY_DIR/docker-compose.yaml" ]]; then
    local net
    net=$(cd "$GATEWAY_DIR" && docker compose config 2>/dev/null | awk '/^name:/ {print $2; exit}')
    if [[ -n "$net" ]] && docker network inspect "${net}_default" >/dev/null 2>&1; then
      echo "${net}_default"
      return 0
    fi
  fi
  local pg
  pg=$(docker ps --format '{{.Names}}' 2>/dev/null | grep -iE 'postgres|erebrus.*pg' | head -1 || true)
  if [[ -n "$pg" ]]; then
    docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{"\n"}}{{end}}' "$pg" 2>/dev/null | head -1
    return 0
  fi
  return 1
}

if [[ ! -f "$GATEWAY_DIR/.env" ]]; then
  die "Missing $GATEWAY_DIR/.env — copy from .env.example and set MNEMONIC + DB_*"
fi

# Normalize .env for docker: gateway reads MNEMONIC, some installs use GATEWAY_MNEMONIC.
ENV_FILE="$(mktemp)"
trap 'rm -f "$ENV_FILE"' EXIT
cp "$GATEWAY_DIR/.env" "$ENV_FILE"

has_signer() {
  grep -qE '^MNEMONIC=.+' "$ENV_FILE" \
    || grep -qE '^PASETO_PRIVATE_KEY=.+' "$ENV_FILE" \
    || grep -qE '^GATEWAY_MNEMONIC=.+' "$ENV_FILE"
}

if ! has_signer; then
  die ".env must set MNEMONIC, GATEWAY_MNEMONIC, or PASETO_PRIVATE_KEY before redeploy"
fi

if ! grep -qE '^MNEMONIC=.+' "$ENV_FILE" && grep -qE '^GATEWAY_MNEMONIC=.+' "$ENV_FILE"; then
  grep '^GATEWAY_MNEMONIC=' "$ENV_FILE" | sed 's/^GATEWAY_MNEMONIC=/MNEMONIC=/' >> "$ENV_FILE"
  log "Mapped GATEWAY_MNEMONIC → MNEMONIC for container"
fi

compose_file_in_dir() {
  local dir="$1"
  for f in docker-compose.yml docker-compose.yaml docker-compose.yml; do
    if [[ -f "$dir/$f" ]]; then
      echo "$dir/$f"
      return 0
    fi
  done
  return 1
}

patch_env_for_compose_network() {
  local f="$1"
  if grep -qE '^DB_HOST=(localhost|127\.0\.0\.1)$' "$f"; then
    sed -i.bak -E 's/^DB_HOST=(localhost|127\.0\.0\.1)$/DB_HOST=postgres/' "$f"
    log "Patched DB_HOST=postgres for docker network"
  fi
  if grep -qE '^REDIS_HOST=localhost' "$f"; then
    sed -i.bak -E 's/^REDIS_HOST=localhost(:6379)?$/REDIS_HOST=redis:6379/' "$f"
    log "Patched REDIS_HOST=redis:6379 for docker network"
  fi
}

wait_for_healthz() {
  local log_target="${1:-$CONTAINER_NAME}"
  for i in $(seq 1 45); do
    if curl -fsS --max-time 2 "http://127.0.0.1:${GATEWAY_PORT}/healthz" >/dev/null 2>&1; then
      log "Gateway healthy"
      curl -fsS "http://127.0.0.1:${GATEWAY_PORT}/healthz" || true
      echo
      return 0
    fi
    sleep 1
  done
  if [[ -n "$(compose_file_in_dir "$GATEWAY_DIR" 2>/dev/null || true)" ]]; then
    (cd "$GATEWAY_DIR" && docker compose logs --tail 80 gateway) 2>/dev/null || true
  else
    docker logs --tail 80 "$log_target" 2>/dev/null || true
  fi
  die "Gateway did not become healthy within 45s"
}

log "Stopping standalone gateway containers on :$GATEWAY_PORT"
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
stop_port_holders "$GATEWAY_PORT"

COMPOSE_FILE=""
if COMPOSE_FILE="$(compose_file_in_dir "$GATEWAY_DIR")"; then
  log "Compose deploy via $COMPOSE_FILE"
  cd "$GATEWAY_DIR"
  OVERRIDE="$GATEWAY_DIR/docker-compose.deploy-override.yml"
  if [[ -n "${GATEWAY_SRC:-}" ]]; then
    [[ -d "$GATEWAY_SRC" ]] || die "GATEWAY_SRC not a directory: $GATEWAY_SRC"
    [[ -f "$GATEWAY_SRC/Dockerfile" ]] || die "Missing $GATEWAY_SRC/Dockerfile"
    cat > "$OVERRIDE" <<EOF
services:
  gateway:
    image: ${GATEWAY_IMAGE}
    build:
      context: ${GATEWAY_SRC}
      dockerfile: Dockerfile
EOF
    COMPOSE_ARGS=(-f "$COMPOSE_FILE" -f "$OVERRIDE")
  else
    COMPOSE_ARGS=(-f "$COMPOSE_FILE")
  fi

  log "Ensuring postgres + redis are running"
  docker compose "${COMPOSE_ARGS[@]}" up -d postgres redis 2>/dev/null \
    || docker compose "${COMPOSE_ARGS[@]}" up -d 2>/dev/null \
    || true

  log "Building + recreating gateway service"
  docker compose "${COMPOSE_ARGS[@]}" build gateway \
    || die "compose build gateway failed"
  docker compose "${COMPOSE_ARGS[@]}" up -d --no-deps --force-recreate gateway \
    || die "compose up gateway failed"
  rm -f "$OVERRIDE"
else
  log "No compose file in $GATEWAY_DIR — standalone docker run"
  if [[ -n "${GATEWAY_SRC:-}" ]]; then
    [[ -d "$GATEWAY_SRC" ]] || die "GATEWAY_SRC not a directory: $GATEWAY_SRC"
    [[ -f "$GATEWAY_SRC/Dockerfile" ]] || die "Missing $GATEWAY_SRC/Dockerfile"
    log "Building $GATEWAY_IMAGE from $GATEWAY_SRC"
    docker build -f "$GATEWAY_SRC/Dockerfile" -t "$GATEWAY_IMAGE" "$GATEWAY_SRC" \
      || die "docker build failed"
  else
    log "Pulling $GATEWAY_IMAGE"
    docker pull "$GATEWAY_IMAGE" || die "docker pull failed — set GATEWAY_SRC to build locally"
  fi

  RESOLVED_NETWORK=""
  if RESOLVED_NETWORK="$(detect_docker_network 2>/dev/null)"; then
    log "Using docker network: $RESOLVED_NETWORK"
    patch_env_for_compose_network "$ENV_FILE"
  else
    log "No compose network detected — container uses host port mapping only"
  fi

  NET_ARGS=()
  if [[ -n "$RESOLVED_NETWORK" ]]; then
    NET_ARGS=(--network "$RESOLVED_NETWORK")
  fi

  docker run -d \
    --name "$CONTAINER_NAME" \
    --restart unless-stopped \
    "${NET_ARGS[@]}" \
    --env-file "$ENV_FILE" \
    -p "${GATEWAY_PORT}:8080" \
    -p 9001:9001 \
    "$GATEWAY_IMAGE"
fi

log "Waiting for /healthz"
wait_for_healthz

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