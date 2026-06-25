# Erebrus Gateway

Control plane for the Erebrus network: wallet auth, node discovery, WebSocket
node hub, VPN client provisioning, entitlements, referrals/XP, and admin APIs.

**Production API:** [gateway.erebrus.io](https://gateway.erebrus.io)  
**Webapp:** [erebrus.io](https://erebrus.io)

## Quick start

```bash
cp .env.example .env          # set MNEMONIC, DB_PASSWORD for non-dev
docker compose up -d postgres redis
make run
```

## Documentation

All docs live in [`docs/`](docs/):

| File | Contents |
|------|----------|
| [**GATEWAY.md**](docs/GATEWAY.md) | Architecture, config, build, deploy, CI/CD, metrics, QA |
| [gateway-api.openapi.yaml](docs/gateway-api.openapi.yaml) | HTTP API (OpenAPI 3) |
| [ws-protocol.md](docs/ws-protocol.md) | Node ↔ gateway WebSocket (frozen v2.0) |

## Build

```bash
make test && make build
./scripts/docker-build.sh     # Docker image with version 2.0.<count> + tag <sha>
```

## Production deploy

Server directory (`~/gateway`) holds `.env` + compose files from [`deploy/`](deploy/).
CI (`docker-publish.yml` on `main`/`prod`) builds the image, pushes to GHCR, SSHs to your
VPS (dev or prod), syncs compose manifests, and runs `docker compose up`. Secrets are
provider-agnostic (`DEV_*` / `PROD_*` / `GHCR_*`) — see [docs/GATEWAY.md § CI/CD](docs/GATEWAY.md#cicd).

## License

See [LICENSE](LICENSE).