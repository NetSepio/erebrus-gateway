# Erebrus Gateway

Single Go binary: wallet auth, node discovery + control plane (WebSocket hub), VPN
client provisioning, entitlements, referrals/XP, and admin.

| | |
|---|---|
| **API** | `https://gateway.erebrus.io` |
| **Webapp** | `https://erebrus.io` (prod) · `https://dev.erebrus.io` (dev) |
| **Entry** | `cmd/gateway/main.go` |
| **Image** | `ghcr.io/netsepio/gateway:prod` / `:main` |

Further reference: [`gateway-api.openapi.yaml`](gateway-api.openapi.yaml) (HTTP API),
[`ws-protocol.md`](ws-protocol.md) (node WebSocket, frozen v2.0).

---

## Architecture

```text
Clients (webapp / Android / iOS)
        │  HTTPS  X-Erebrus-Client: webapp|android|ios
        ▼
Traefik  →  gateway:8080  (/api/v2/*, /healthz, /version)
        │                  (/metrics NOT public)
        │
Nodes    →  WSS /api/v2/nodes/ws  (node PASETO)
        │
        ├─ Postgres (source of truth, migrations on boot)
        ├─ Redis (discovery cache, rate limits — optional)
        └─ Resend (optional email OTP)

Metrics (prod):
  otel-collector  →  scrape gateway:8080/metrics (Docker network)
                 →  OTLP push https://otel.netsepio.com
```

### Repository layout

```text
cmd/gateway/          HTTP server entry
internal/api/         Gin routes + handlers
internal/store/       Postgres + SQL migrations
internal/nodehub/     Node WebSocket control plane
internal/nodeclient/  Gateway → node HTTP (provision peers)
internal/metrics/     Prometheus vectors (netsepio_erebrus_gateway_*)
internal/middleware/  HTTP metrics middleware
internal/config/      Env config + platform_settings loader
internal/token/       PASETO (user / node / admin)
internal/wallet/      EVM + Solana signature verify
internal/nftgate/     NFT entitlement check (optional)
deploy/               Production docker-compose + OTel collector config
```

### Product model

- **Auth:** Reown wallet signature only (`chain ∈ {evm, sol}`). Email links via
  Resend OTP — never a login method.
- **User entitlements (legacy):** 7-day trial (once), NFT → 30 days direct, rank perks,
  admin bypass. Per-user; separate from org plans below.
- **Orgs:** Control-plane workspace with plans (`basic` | `starter` | `pro` | `business`
  | `enterprise`), billing status, verification status, and optional public profile.
  Members have a **role** (management) and **seat_tier** (premium access) — these are
  independent. New orgs start on `basic` with entitlements seeded from
  `org_entitlements`.
- **Org nodes:** Control-plane records in `org_nodes` (keyed by `peer_id`) with
  `deployment_profile` (`erebrus` | `shield` | `sentinel`), attached **services**
  (`org_node_services`), and optional Sentinel **firewall rules**.
- **Runtime nodes:** Operational VPN rows in `nodes` (UUID PK, WS heartbeats). See
  [Deferred work](#deferred-work-requires-erebrus-node-integration) for how these
  layers will fully converge.
- **Node enrollment:** Scoped **registration tokens** (`ere_reg_*`), minted per org.
  `POST /api/v2/nodes/register` accepts `registration_token` (legacy alias:
  `enrollment_secret`). Node installer env: `EREBRUS_NODE_REGISTRATION_TOKEN`.
  Two-step machine challenge → node PASETO + per-node `node_key`.
- **Plans (gateway-side today):** Entitlement limits and managed-node **reservations**
  in DB on admin plan change. Shield/Sentinel lifecycle metadata via generic
  `/firewall/*` APIs. Actual deploy and runtime sync deferred (see below).
- **Social layer:** Referrals, XP/tiers, leaderboard, X/Telegram/email verify, perks.
- **Activity log:** Authenticated mutations logged with IP + device for user visibility.

---

## Configuration

Copy [`.env.example`](../.env.example) to the server deploy directory (`~/gateway/.env`).
The same file is used by the gateway container (`env_file`) and compose variable substitution.

| Variable | Purpose |
|----------|---------|
| `MNEMONIC` | Derives PASETO signer (required in prod) |
| `DB_*` | Postgres |
| `REDIS_HOST` | Cache (gateway runs without it) |
| `ENVIRONMENT` | `production` / `staging` / `dev` — metrics + `/version` |
| `ALLOWED_ORIGIN` | CORS (erebrus.io, dev.erebrus.io) |
| `TRUSTED_PROXIES` | Traefik Docker network CIDR for real ClientIP |
| `ADMIN_WALLET_ADDRESS` | First admin on boot |
| `SOLANA_RPC_URL` | Solana JSON-RPC for NFT gating (DAS-capable in prod) |
| `GATEWAY_IMAGE`, `GATEWAY_HOST`, … | Compose/Traefik only (ignored by gateway binary) |
| `OTEL_AUTH_TOKEN` | Optional — starts otel-collector when set |
| `MANAGED_NODE_PROVISIONING_ENABLED` | When true, reserved managed nodes use `provisioning` status (no cloud deploy yet) |
| `MANAGED_NODE_DEFAULT_REGION`, `MANAGED_NODE_DEFAULT_IMAGE`, `SENTINEL_IMAGE` | Managed-node defaults (DB reservation today) |
| `EREBRUS_PUBLIC_BASE_URL`, `GATEWAY_PUBLIC_BASE_URL` | URLs embedded in generated installer/config (node repo) |

Gating collections are in **`nft_gate_contracts`** (migration `0012`): IslandDAO + Erebrus Free Trial NFT on Solana; more chains/addresses can be inserted later.

Product tunables (XP weights, trial length, rate limits, PASETO TTL) live in
**`platform_settings`** (DB, migration `0009`) — editable via
`PATCH /api/v2/admin/settings` without redeploy.

---

## Build & version

```bash
make build    # Version=2.0.<commit-count>  Tag=<short-sha>
make test
./scripts/docker-build.sh [--push]
```

Injected at link time into `internal/version/`:

- `Version` — auto-incremented per commit (`2.0.428`)
- `Tag` — git short SHA

`GET /version` returns `{product, service, environment, version, tag}`.

---

## Local development

```bash
docker compose up -d postgres redis
make run          # or: make build && ./gateway
```

Migrations apply on boot. Default MNEMONIC in compose is **dev only**.

---

## Production deployment

Prod is **two layers**: infra (manual, one-time) and gateway (CI on push to `prod`).
CI never installs Docker, Postgres, Redis, or Traefik — bootstrap the host first.

### Server layout

```text
~/infra/                       # manual — Postgres, Redis, Traefik (your compose)
  docker-compose.yml
  traefik/acme.json            # Let's Encrypt store (chmod 600)

~/gateway/                     # prod (~/gateway-dev for main branch)
  .env                         # single .env.example (app + compose vars)
  docker-compose.yml           # synced by CI from deploy/
  otel-collector-config.yaml
```

All services share external Docker network **`erebrus_gateway_network`**.

### First-time bootstrap (manual — before merging to `prod`)

1. **Host** — Docker Engine + Compose plugin; deploy SSH key on server; DNS `gateway.erebrus.io` → IP; firewall **80/443** only.
2. **Network** — `docker network create erebrus_gateway_network`
3. **Infra** (`~/infra`, not deployed by CI) — Postgres (`postgres`), Redis (`redis`), Traefik **v3.6+** on `erebrus_gateway_network`; cert resolver `letsencrypt`; ACME email **support@netsepio.com**. `POSTGRES_PASSWORD` must match `DB_PASSWORD` in `~/gateway/.env`.
4. **Gateway `.env`** — `DB_HOST=postgres`, `REDIS_HOST=redis:6379`, `DB_SSLMODE=disable`, `MNEMONIC`, `TRUSTED_PROXIES` (Traefik CIDR), DAS-capable `SOLANA_RPC_URL` for NFT refresh.
5. **GitHub secrets** — `GHCR_*`, `PROD_REMOTE_SERVER_*`.
6. **Smoke test** — `docker compose up -d` in `~/gateway`; `curl http://127.0.0.1:8080/readyz`; `curl https://gateway.erebrus.io/healthz`.
7. **Merge `main` → `prod`** — CI deploys gateway image + compose to `~/gateway`.

### What CI deploys (gateway only)

1. **gateway** — Traefik labels, `127.0.0.1:8080` for health checks; `!Path(/metrics)` on public router.
2. **otel-collector** (optional) — `--profile telemetry` when `OTEL_AUTH_TOKEN` is set.

CI does **not** overwrite `~/gateway/.env`.

```bash
cd ~/gateway
docker compose pull && docker compose up -d
curl http://127.0.0.1:8080/healthz
curl https://gateway.erebrus.io/healthz
```

### Traefik

```yaml
traefik.http.routers.gateway.rule=Host(`gateway.erebrus.io`) && !Path(`/metrics`)
```

### Ongoing ops

- [ ] Postgres backups (private ops script + cron)
- [ ] `OTEL_AUTH_TOKEN` when telemetry is live
- [ ] New NFT collections: `INSERT INTO nft_gate_contracts ...` then restart gateway

---

## Metrics & observability

| Endpoint | Purpose |
|----------|---------|
| `GET /healthz` | Liveness (`ok`) |
| `GET /readyz` | Readiness (DB required, Redis optional) |
| `GET /version` | Build metadata |
| `GET /metrics` | Prometheus (internal scrape only) |
| `POST /telemetry/event` | Safe app events (whitelisted labels) |

Key metrics: `netsepio_erebrus_gateway_requests_total`,
`netsepio_erebrus_gateway_errors_total`,
`netsepio_erebrus_gateway_request_duration_seconds`,
`netsepio_erebrus_gateway_node_registration_total`,
`netsepio_erebrus_gateway_node_heartbeat_total`,
`netsepio_erebrus_gateway_vpn_config_generated_total`,
`netsepio_erebrus_gateway_active_node_sessions`,
`netsepio_erebrus_gateway_app_events_total`,
`netsepio_erebrus_gateway_build_info`.

Clients should send `X-Erebrus-Client: webapp|android|ios|node` for HTTP metrics.

---

## CI/CD

**Workflow:** `.github/workflows/docker-publish.yml`

| Branch | Image tag | Server dir | Environment |
|--------|-----------|------------|-------------|
| `main` | `:main` | `~/gateway-dev` | staging |
| `prod` | `:prod` | `~/gateway` | production |

On push:

1. **build_push** — `scripts/docker-build.sh --push` → GHCR
2. **deploy_*** — SCP `deploy/docker-compose.yml` + `otel-collector-config.yaml`,
   `docker compose pull && up -d`, wait for `/healthz`

**CI tests:** `.github/workflows/ci.yml` — vet, build, test, gitleaks on `main`/`prod`.

First-time server: see **First-time bootstrap** above.

### GitHub Actions secrets (any VPS — not cloud-specific)

| Secret | Job | Purpose |
|--------|-----|---------|
| `GHCR_TOKEN` | build + deploy | PAT with `read:packages` + `write:packages` for `ghcr.io/netsepio/gateway` |
| `GHCR_USERNAME` | build + deploy | GitHub user that owns the token |
| `DEV_REMOTE_SERVER_ADDRESS` | `main` deploy | Dev host IP or hostname |
| `DEV_REMOTE_SERVER_USERNAME` | `main` deploy | SSH user (e.g. `root`) |
| `DEV_SSH_PORT` | `main` deploy | SSH port (usually `22`) |
| `DEV_REMOTE_SERVER_KEY` | `main` deploy | Deploy-only SSH private key |
| `PROD_REMOTE_SERVER_*` | `prod` deploy | Same four fields for production host |

Legacy secret names `AWS_*` are not used — rename them to `DEV_*` in repo Settings → Secrets.

---

## Database migrations

Applied automatically on startup (`internal/store/migrations/`):

| Migration | Content |
|-----------|---------|
| 0001 | Core schema (users, nodes, clients, subs) |
| 0002 | Orgs + API keys |
| 0003 | Email OTP auth |
| 0004 | Node metrics rollup |
| 0005 | Referrals + XP events |
| 0006 | XP claims |
| 0007 | Social accounts + perks |
| 0008 | Activity log |
| 0009 | Platform settings (DB-backed config) |
| 0010 | Remove payments scaffolding |
| 0011 | Org node model (`enrollment_secret`, `node_key`, drop `owner_user_id`, public/private `access_mode`) |
| 0012 | `nft_gate_contracts` (Solana gating collections; IslandDAO + Erebrus Free Trial NFT) |
| 0013 | Node `zone` field |
| 0014 | `last_peer_handshake` on nodes |
| 0015 | Node wallet `chain` at enrollment |
| 0016 | Org plan model (`orgs` plan/billing/verification, `org_profiles`, `org_members` seat tiers, `org_entitlements`) |
| 0017 | `org_nodes`, `org_node_services`, `node_registration_tokens`, `sentinel_licenses` |
| 0018 | `firewall_rules` (Sentinel policy, gateway-managed) |

---

## Deferred work (requires erebrus node integration)

Items intentionally stubbed or split across gateway + node repos. Implement after
`erebrus` node/runtime is updated to consume registration tokens and service APIs.

| Area | Gateway today | Needs erebrus node |
|------|---------------|-------------------|
| **Node ID duality** | `nodes.id` (UUID) for runtime/WS; `org_nodes.node_id` (peer_id) for org APIs | Align installer, WS `hello`, and dashboards on one canonical id |
| **Heartbeat sync** | WS updates `nodes`; REST `POST /nodes/:id/heartbeat` also touches `org_nodes.last_seen_at` | WS path should update `org_nodes` too |
| **Public node access tier** | Stored in `org_entitlements.public_node_access_tier` | Wire into discovery/VPN gating (replace or combine with XP `min_tier`) |
| **Seat tier → VPN access** | `seat_tier` on `org_members`; assign validates plan | Client provisioning should check org seat, not only user trial/NFT |
| **Firewall runtime** | `/firewall/restart`, `/sync`, `reset-credentials` update gateway metadata | Push rules/config to Shield (AdGuard) and Sentinel (Unbound) on node |
| **Sentinel unlicensed** | `ReconcileUnlicensedSentinel` helper exists | Call when node reports Sentinel without license; surface user message |
| **Managed provisioning** | DB rows with `managed_by=erebrus`, status `pending`/`provisioning` | SSH/cloud deploy using `NODE_PROVISION_SSH_*` and image env vars |
| **Registration token lifecycle** | Mint + lookup; `used_at` recorded | Single-use enforcement, list/revoke APIs (optional) |
| **Billing / plan changes** | Admin `PATCH /admin/orgs/:id` with `plan` | Checkout webhooks, self-serve upgrade, Enterprise entitlements |
| **Extra Sentinel licenses** | `sentinel_licenses` table | Purchase flow and attach API |

---

## Node identity (summary)

Nodes derive from a single BIP39 mnemonic:

1. Wallet keypair — signs registration challenge
2. libp2p PeerID — stable identity anchor
3. DID — `did:erebrus:<PeerID>`

Public APIs never expose raw node IP; only `ip_hash` (SHA3-256 of IPv4) may appear
off the authenticated channel. Raw IP is used only on the node↔gateway WS for
endpoint construction. Full spec: historical `identity.md` content is folded here;
WS message shapes are in [`ws-protocol.md`](ws-protocol.md).

---

## QA acceptance

Run against a host with **Postgres + Redis** before production cutover.

```bash
go build ./... && go vet ./... && go test ./...
```

### S1 — Streamline

- [ ] Single binary `cmd/gateway`; no v1 packages
- [ ] `docker compose up` + image build succeeds

### S2 — Auth (EVM + Solana, optional email)

- [ ] Migrations through `0003`; `users.email_verified`, `email_otps` exist
- [ ] `chain=apt|sui` → 400; `evm`/`sol` challenge → signed login → valid PASETO
- [ ] Auth is `GET+POST /api/v2/auth` only (no `/auth/flowid`)
- [ ] Email: `RESEND_API_KEY` unset → 503; set → OTP flow, verify, 409 duplicate email
- [ ] `PATCH /account/profile` cannot set email directly

### S3 — Entitlements

- [ ] Trial ≈ 7d (`trial_period` in platform_settings); second trial → 409
- [ ] No trial → provision **402**; with trial → succeeds
- [ ] NFT refresh → 30d `source:nft` (when `nft_gate_contracts` + `SOLANA_RPC_URL` set)
- [ ] Admin bypass works; `/payments*` → 404

### S4 — Operator layer

- [ ] Migrations through `0018`; org plan model + registration tokens
- [ ] Registration gated by `registration_token`; machine challenge (not `/auth`)
- [ ] `access_mode=private` hidden from public `GET /nodes`, visible in operator view
- [ ] Heartbeats populate `node_metrics`; operator/admin chart endpoints work
- [ ] `GET /orgs/:id/nodes` returns org control-plane nodes; `GET /orgs/:id/runtime-nodes` returns runtime `nodes` rows

### S5 — Referrals

- [ ] Migration `0005`; `GET /referrals/me` returns stable code
- [ ] `ref` on signup binds referrer; first trial → referral XP once

### S6 — XP / tiers / leaderboard

- [ ] Migration `0006`; tiers from `xp_earned`; `GET /rank/me`, `GET /leaderboard`
- [ ] `POST /rank/claim` spends XP → `source:rank` entitlement
- [ ] Drivers idempotent: email, NFT monthly, operator uptime

### S7 — Social + perks + tier pools

- [ ] Migration `0007`; `min_tier` gates discovery + provisioning
- [ ] Telegram + X verify → `social_verified` XP once per provider
- [ ] Perks catalog + admin grant

### S8 — Production hardening

- [ ] `/readyz` 200 when DB up, 503 when DB down
- [ ] `/metrics` exposes `netsepio_erebrus_gateway_*` (not public via Traefik)
- [ ] Rate limits on `/auth` and `/nodes/register` (tunable in platform_settings)
- [ ] `TRUSTED_PROXIES` → real client IP in rate limit + activity log
- [ ] CORS allows erebrus.io origins + `X-Erebrus-Client`

### S8b — Activity log

- [ ] Migration `0008`; mutations logged with `ip`, `device`, `app`
- [ ] `GET /account/activity` paginated; failures not logged; admin fleet view

### S9 — Platform settings

- [ ] Migration `0009`; tunables editable via admin settings PATCH without restart

---

## API quick reference

| Area | Routes |
|------|--------|
| Ops | `/healthz`, `/readyz`, `/version`, `/metrics`, `/telemetry/event` |
| Auth | `GET+POST /api/v2/auth`, `/auth/email`, `/auth/email/verify` |
| Nodes | `GET /nodes`, `POST /nodes/register`, `POST /nodes/:id/heartbeat`, `GET /nodes/ws` |
| VPN | `/vpn/clients`, `/vpn/clients/:id/config` |
| Account | `/account/profile`, `/account/activity` |
| Subs | `/subscriptions/*`, `/subscriptions/trial` |
| Social | `/referrals/me`, `/rank/*`, `/leaderboard`, `/social/*`, `/perks/*` |
| Orgs | `/orgs`, `/orgs/:id/entitlements`, `/orgs/:id/profile`, `/orgs/:id/seats`, `/orgs/:id/nodes`, `/orgs/:id/nodes/:nodeId/firewall/*` |
| Public | `/public/orgs/:slug` |
| Operator | `/operator/nodes`, `/operator/nodes/:id/metrics` |
| Admin | `/admin/*` (incl. `PATCH /admin/orgs/:id` for plan + verification) |

Full OpenAPI: [`gateway-api.openapi.yaml`](gateway-api.openapi.yaml).
