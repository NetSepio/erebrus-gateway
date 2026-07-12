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
internal/nftgate/     Legacy NFT reward verification (optional)
deploy/               Production docker-compose + OTel collector config
```

### Product model

- **Auth:** Reown wallet signature only (`chain ∈ {evm, sol}`). Email links via
  Resend OTP — never a login method.
- **Entitlement (organization-only):** Product/Drop entitlement is derived
  **solely** from organization membership — the effective tier is the highest
  active org seat across all active memberships (owners and node operators map
  to the org plan; ordinary members to their `seat_tier`). Personal trials, `ActiveSubscription`,
  NFT grants, and personal subscription tiers are **no longer** entitlement
  sources. Every user owns a personal `basic` org (backfilled by migration
  `0026` for older users; created at first login for new ones), so an
  authenticated user always resolves to at least Free. Legacy `subscriptions`
  data is retained for a rollback window; `GET /api/v2/subscriptions`,
  `…/trial`, and `…/nft/refresh` return organization-derived compatibility
  responses while the webapp migrates.
- **Orgs:** Control-plane workspace with plans (`basic` | `starter` | `pro` | `business`
  | `enterprise`), billing status, verification status, and optional public profile.
  Members have a **role** (management) and **seat_tier** (premium access) — these are
  independent. New orgs start on `basic` with entitlements seeded from
  `org_entitlements`.
- **Org nodes:** Control-plane records in `org_nodes` (keyed by `peer_id`) with
  `deployment_profile` (`standard` | `shield` | `sentinel`), attached **services**
  (`org_node_services`), and optional Sentinel **firewall rules**.
- **Runtime nodes:** Operational VPN rows in `nodes` (internal UUID PK; **`peer_id` is
  the canonical external id** for discovery, org APIs, PASETO claims, WS hub, and
  REST heartbeat paths). Legacy UUID lookups still resolve during rollout.
- **Org invites:** Owners/admins invite by `wallet_address` and/or `email`. Resend
  sends an invite link (`EREBRUS_PUBLIC_BASE_URL/orgs/{slug}`). Email-only invites
  stay in `org_invites` until the invitee signs in and verifies that email; wallet
  invites activate on login (or email verify when applicable).
- **Node enrollment:** Scoped **registration tokens** (`ere_reg_*`), minted per org.
  `POST /api/v2/nodes/register` accepts a scoped `registration_token`. Node installer
  env: `EREBRUS_NODE_REGISTRATION_TOKEN`.
  Two-step machine challenge → node PASETO + per-node `node_key`.
- **Plans (gateway-side today):** Entitlement limits update on admin plan change.
  Managed-node **reservations** in `org_nodes` are created only when
  `MANAGED_NODE_PROVISIONING_ENABLED=true`; otherwise self-hosted nodes use plan
  entitlements directly. Shield/Sentinel lifecycle metadata via generic `/firewall/*`
  APIs. Actual deploy and runtime sync deferred (see below).
- **Social layer:** Referrals, XP/tiers, leaderboard, X/Telegram/email verify, perks.
- **Activity log:** Authenticated mutations logged with IP + device for user visibility.
- **Drop (Kubo storage):** Independent optional service (not a deployment
  profile) available on Standard/Shield/Sentinel — see the Drop section below.

---

### Drop (Kubo storage)

Drop lets users store files on Kubo (IPFS) nodes through the gateway. It is an
**independent optional service** advertised per node over the WS control plane
(`hello.capabilities.drop`, `heartbeat.drop`, `versions.kubo`,
`services.drop` — see `docs/ws-protocol.md`), not a fourth deployment profile.

- **Entitlement & quota:** Public storage quota is **per user across all public
  nodes**, resolved from the highest active org seat: Free `500,000,000`,
  Starter `1,000,000,000`, Pro `5,000,000,000`, Business `10,000,000,000` bytes
  (`drop_tier_limits`, seeded by migration `0026`; admins can retune without a
  redeploy). Enterprise currently mirrors Business — **provisional**, pending a
  product decision. Private-org storage is governed by org membership and node
  capacity, not the public per-user quota.
- **Reservations:** Uploads reserve quota **and** node capacity atomically
  (row-locked in one transaction) before any bytes move, keyed idempotently by
  `(owner_user_id, idempotency_key)`. Reserved bytes convert to used bytes on
  commit and are released on expiry/failure/delete. A background reconciliation
  pass (`ExpireDropReservations`) releases TTL-expired reservations.
- **Streaming:** Content streams gateway→node→Kubo (upload) and
  node→gateway→caller (download) without buffering, via a dedicated
  `internal/dropclient` with bounded dial/header timeouts and **no**
  whole-transfer timeout (the short control-plane `internal/nodeclient` is
  unchanged). Byte limits are enforced; upload bodies are never logged.
- **Pins & deletion:** One pin row per `(file_id, node_id)`. Duplicate CID
  references are tolerated; a physical unpin is issued only when the **last**
  active reference on a node is deleted. Deletion, quota release, and unpin are
  idempotent. Failed physical unpins and pins created before a metadata-commit
  failure are persisted and retried by reconciliation after shared-CID checks.
- **Direct retrieval + fallback:** Nodes advertise a public IPFS gateway base
  additively (`hello.capabilities.drop.public_gateway_url`, http(s), never the
  5001 RPC). Public file responses (`GET /drop/files`, `/drop/files/{id}`,
  `/drop/public/{file_id}`) surface `gateway_url` (primary node) and
  `gateway_urls` (all nodes holding the CID, from the `drop_pins` join;
  `public_gateway_url(s)` are accepted aliases) so browsers can fetch
  `<base>/ipfs/<cid>` directly. `GET /drop/public/{file_id}/content` is the
  durable proxy fallback and sources the CID from **any** healthy pinned node,
  so a single node going offline does not break retrieval. **Node-side
  requirement:** for direct browser fetches the node's `/ipfs/*` must return
  permissive CORS (`Access-Control-Allow-Origin` + `GET, HEAD, OPTIONS`); without
  it the webapp silently falls back to the proxy (functional, but no offload).
- **Security:** Public shares use opaque file ids, never raw CIDs, and enforce
  `visibility=public` + `status=active`. Private confidentiality relies on
  **client-side encryption**; the gateway stores only opaque encryption metadata
  and a versioned, bounded, encrypted **vault** (`drop_crypto_profiles`) — no
  plaintext key material or recovery secret. Owners/node-operators can inspect
  metadata and delete but receive **no** decryption keys. Kubo RPC stays private
  to the node's internal Docker network; the WebUI is reachable only through a
  short-lived, same-origin gateway proxy session that never reveals the node
  address. CIDs are validated and response filenames sanitized.

**Deployment ordering:** deploy the node with a running Kubo daemon (private
RPC on the internal Docker network) **before** relying on Drop APIs — the
gateway only treats a node as usable once it reports Drop capability on `hello`
and a healthy runtime `drop.state` on `heartbeat`. No gateway env var enables
Drop; discovery follows node health. Drop rate limits live in
`platform_settings` (`rate_limit_drop_write_per_min`,
`rate_limit_drop_read_per_min`).

Tables (migration `0026_drop.sql`): `drop_tier_limits`, `drop_uploads`,
`drop_files`, `drop_pins`, `drop_quota_usage`, `node_drop_status`,
`drop_crypto_profiles`. Migration `0027_drop_gateway_url.sql` adds
`node_drop_status.public_gateway_url`.

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

NFT verification collections are in **`nft_gate_contracts`** (migration `0012`):
IslandDAO + the historical Erebrus Free Trial NFT on Solana. Verification may
award social XP but never grants product access.

Product tunables (XP weights, retained legacy trial values, rate limits, PASETO TTL) live in
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

Drop (Kubo storage) metrics — labels are deliberately low-cardinality (fixed
enumerations only, never per-user/per-file):

- `netsepio_erebrus_gateway_drop_uploads_total{result,scope}`
- `netsepio_erebrus_gateway_drop_upload_bytes_total{scope}`
- `netsepio_erebrus_gateway_drop_download_bytes_total{scope}`
- `netsepio_erebrus_gateway_drop_quota_rejections_total{tier}`
- `netsepio_erebrus_gateway_drop_node_operations_total{operation,result}`
- `netsepio_erebrus_gateway_drop_reconciliation_jobs_total{operation,result}`

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
| 0012 | `nft_gate_contracts` (Solana NFT verification collections; no product entitlement) |
| 0013 | Node `zone` field |
| 0014 | `last_peer_handshake` on nodes |
| 0015 | Node wallet `chain` at enrollment |
| 0016 | Org plan model (`orgs` plan/billing/verification, `org_profiles`, `org_members` seat tiers, `org_entitlements`) |
| 0017 | `org_nodes`, `org_node_services`, `node_registration_tokens`, `sentinel_licenses` |
| 0018 | `firewall_rules` (Sentinel policy, gateway-managed) |
| 0019 | `org_invites` (pending email invites until verified) |

---

## Deferred work (requires erebrus node integration)

Items intentionally stubbed or split across gateway + node repos. Implement after
`erebrus` node/runtime is updated to consume registration tokens and service APIs.

| Area | Gateway today | Needs erebrus node |
|------|---------------|-------------------|
| ~~**Node ID duality**~~ | **Fixed:** `peer_id` is canonical in APIs, tokens, WS hub, and discovery; internal UUID retained for DB FKs only | Node installer + `hello` should send `peer_id` as `node_id` (see `ws-protocol.md`) |
| **Public node access tier** | Stored in `org_entitlements.public_node_access_tier` | Wire into discovery/VPN gating (replace or combine with XP `min_tier`) |
| **Seat tier → VPN access** | `seat_tier` on `org_members`; assign validates plan | Client provisioning checks organization membership and seats, never legacy personal trial/NFT rows |
| ~~**Firewall runtime**~~ | **Fixed:** `/firewall/sync` pushes rules via WS `sync_firewall`; restart/reset-credentials dispatch WS commands | Node proxies to Sentinel API / Shield admin |
| **Sentinel unlicensed** | `ReconcileUnlicensedSentinel` helper exists | Call when node reports Sentinel without license; surface user message |
| **Managed provisioning** | DB rows with `managed_by=erebrus`, status `pending`/`provisioning` | SSH/cloud deploy using `NODE_PROVISION_SSH_*` and image env vars |
| **Registration token lifecycle** | Mint + lookup; `used_at` recorded | Single-use enforcement, list/revoke APIs (optional) |
| **Billing / plan changes** | Admin `PATCH /admin/orgs/:id` with `plan` | Checkout webhooks, self-serve upgrade, Enterprise entitlements |
| **Extra Sentinel licenses** | `sentinel_licenses` table | Purchase flow and attach API |

---

## Node identity (summary)

Nodes derive from a single BIP39 mnemonic:

1. Wallet keypair — signs registration challenge
2. libp2p PeerID — stable identity anchor and **canonical `node_id`** everywhere external
3. DID — `did:erebrus:<PeerID>`

Registration returns `node_id` = `peer_id`. Node PASETO claims set both `node_id` and
`peer_id` to the libp2p id. The gateway keeps an internal UUID in `nodes.id` for
`vpn_clients` / metrics FKs only.

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

- [ ] Missing users are backfilled into a personal basic organization
- [ ] Free/Starter/Pro/Business resolve from active organization plan/seat data
- [ ] Personal trials, subscriptions, NFT rows, and rank grants never authorize product access
- [ ] Legacy subscription routes return compatibility data only; `/payments*` → 404

### S4 — Operator layer

- [ ] Migrations through `0018`; org plan model + registration tokens
- [ ] Registration gated by `registration_token`; machine challenge (not `/auth`)
- [ ] `access_mode=private` hidden from public `GET /nodes`, visible in operator view
- [ ] Heartbeats populate `node_metrics`; operator/admin chart endpoints work
- [ ] `GET /orgs/:id/nodes` returns org control-plane nodes; `GET /orgs/:id/runtime-nodes` returns runtime `nodes` rows

### S5 — Referrals

- [ ] Migration `0005`; `GET /referrals/me` returns stable code
- [ ] `ref` on signup binds referrer; first active org membership → referral XP once

### S6 — XP / tiers / leaderboard

- [ ] Migration `0006`; tiers from `xp_earned`; `GET /rank/me`, `GET /leaderboard`
- [ ] `POST /rank/claim` returns 410 and never creates personal entitlement
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
| Legacy subscription compatibility | `/subscriptions/*`, `/subscriptions/trial` (organization-derived; no grants) |
| Social | `/referrals/me`, `/rank/*`, `/leaderboard`, `/social/*`, `/perks/*` |
| Orgs | `/orgs`, `/orgs/:id/entitlements`, `/orgs/:id/profile`, `/orgs/:id/seats`, `/orgs/:id/nodes`, `/orgs/:id/nodes/:nodeId/firewall/*` |
| Public | `/public/orgs/:slug` |
| Operator | `/operator/nodes`, `/operator/nodes/:id/metrics` |
| Admin | `/admin/*` (incl. `PATCH /admin/orgs/:id` for plan + verification) |

Full OpenAPI: [`gateway-api.openapi.yaml`](gateway-api.openapi.yaml).
