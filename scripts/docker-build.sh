#!/usr/bin/env bash
# Build the gateway image with an incrementing version and commit-hash tags.
#
# Version: 2.0.<commit-count>  Tag: <short-sha>  (injected via -ldflags)
# Tags:    ghcr.io/netsepio/gateway:<full-sha> and :<branch>
#
# Usage (local):
#   ./scripts/docker-build.sh
#   ./scripts/docker-build.sh --push
#
# CI sets IMAGE, GITHUB_SHA, and GITHUB_REF_NAME.

set -euo pipefail

PUSH=false
for arg in "$@"; do
  case "$arg" in
    --push) PUSH=true ;;
  esac
done

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

IMAGE="${IMAGE:-ghcr.io/netsepio/gateway}"
SHA="${GITHUB_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="${SHORT_SHA:-$(git rev-parse --short HEAD)}"
BRANCH="${GITHUB_REF_NAME:-$(git rev-parse --abbrev-ref HEAD)}"
COUNT="$(git rev-list --count HEAD)"
VERSION="2.0.${COUNT}"
TAG="${SHORT_SHA}"

SHA_TAG="${IMAGE}:${SHA}"
BRANCH_TAG="${IMAGE}:${BRANCH}"

echo "Building version=${VERSION} tag=${TAG}"
echo "Tags: ${SHA_TAG}, ${BRANCH_TAG}"

docker build -f Dockerfile \
  --build-arg "version=${VERSION}" \
  --build-arg "tag=${TAG}" \
  -t "$SHA_TAG" \
  -t "$BRANCH_TAG" \
  .

if [[ "$PUSH" == "true" ]]; then
  docker push "$SHA_TAG"
  docker push "$BRANCH_TAG"
fi