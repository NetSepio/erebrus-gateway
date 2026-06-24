#!/usr/bin/env bash
# Find gateway .env directory. Prints the directory path to stdout on success.
# Logs go to stderr. Exit 0 = found, 1 = not found.

set -euo pipefail

log() { printf '[discover] %s\n' "$*" >&2; }

if [[ -n "${GATEWAY_DIR:-}" && -f "${GATEWAY_DIR}/.env" ]]; then
  log "using GATEWAY_DIR=$GATEWAY_DIR"
  echo "$GATEWAY_DIR"
  exit 0
fi

candidates=(
  "${HOME}/gateway-v2"
  "${HOME}/gateway"
  "${HOME}/gateway-dev"
  "/opt/gateway-v2"
  "/opt/gateway"
  "/opt/erebrus-gateway"
)

for d in "${candidates[@]}"; do
  if [[ -f "$d/.env" ]]; then
    log "found $d/.env"
    echo "$d"
    exit 0
  fi
done

mapfile -t cids < <(docker ps -q --filter "publish=8080" 2>/dev/null || true)
if [[ ${#cids[@]} -eq 0 ]]; then
  mapfile -t cids < <(docker ps --format '{{.ID}} {{.Ports}}' 2>/dev/null | awk '/8080/ {print $1}' || true)
fi

for cid in "${cids[@]}"; do
  [[ -n "$cid" ]] || continue
  while IFS= read -r src; do
    [[ -f "$src" && "$(basename "$src")" == ".env" ]] || continue
    log "found mounted .env at $src"
    dirname "$src"
    exit 0
  done < <(docker inspect -f '{{range .Mounts}}{{.Source}}{{"\n"}}{{end}}' "$cid" 2>/dev/null)
done

while IFS= read -r envfile; do
  if grep -qE '^(MNEMONIC|GATEWAY_MNEMONIC|PASETO_PRIVATE_KEY|APP_PORT)=' "$envfile" 2>/dev/null; then
    log "found $envfile"
    dirname "$envfile"
    exit 0
  fi
done < <(find "$HOME" /opt -maxdepth 5 -name '.env' 2>/dev/null | head -40)

log "no gateway .env found"
log "docker ps:"
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}' >&2 2>/dev/null || true
exit 1