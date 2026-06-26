# Gateway security, resilience, and test plan

Production gateway (`https://gateway.erebrus.io`) is a **single point of failure** for the mobile app, node control plane, and VPN provisioning. This document captures the current posture, data flows, pentest/stress procedures (non-disruptive), quick fixes, and a longer-term HA roadmap.

**Last verified:** 2026-06-26 (commit `ac059bd`, prod version `2.0.696`)

---

## CI/CD status

Latest push (`ac059bd` — drop `deploy_dev` until dev server exists):

| Branch | Workflow | Result |
|--------|----------|--------|
| `main` | `ci` | success |
| `main` | `Docker publish` (build + push `:main` only) | success |
| `prod` | `ci` | success |
| `prod` | `Docker publish` (`build_push` + `deploy_prod`) | success |

Prod server matches CI: **tag `ac059bd`**, healthy.

**Pipeline behavior today**

- `main` → builds and pushes `ghcr.io/netsepio/gateway:main` (no SSH deploy; no dev server yet).
- `prod` → builds `:prod`, copies compose manifests, SSH deploy to `~/gateway`, `docker compose pull && up -d`, waits for `/healthz`.

---

## Security snapshot (prod)

Checked via SSH + public curls on `185.26.48.195` / `gateway.erebrus.io`.

### Good

| Item | Status |
|------|--------|
| Gateway port | `127.0.0.1:8080` only (Traefik terminates TLS on 443) |
| Postgres / Redis | Docker network only — not host-bound |
| `/metrics` public | **404** (Traefik `!Path(/metrics)` rule works) |
| TLS | Let's Encrypt via Traefik |
| Auth | PASETO (user / node / admin), admin role gate, org API keys hashed |
| CI secrets scan | gitleaks on push |
| Discovery | No top-level node IP; dial host under `endpoints.wireguard.host` only |

### Gaps / risks (prioritized)

1. **Single VPS SPOF** — Traefik, Postgres, Redis, and gateway on one host; host failure = full outage.
2. **Deploy blip** — each prod push runs `docker compose up -d`, which recreates the gateway container (~seconds downtime).
3. **Rate limits fail-open** — if Redis is down, `cache.Allow` returns true (availability over abuse protection). See `internal/cache/cache.go`.
4. **Redis no password** — infra compose runs `redis-server` without `requirepass`.
5. **No WAF / edge DDoS** — only provider network in front of Traefik (no Cloudflare proxy yet).
6. **`/readyz` is public** — exposes DB/Redis readiness (low severity; recon-friendly).
7. **Secrets on disk** — `~/gateway/.env` holds `MNEMONIC`, DB password, Resend key; host compromise = full compromise.
8. **Dependabot** — 7 open vulns on repo (3 high) as of 2026-06-26; image/OS audit separate.
9. **Backups** — `backup-postgres.sh` exists in ops repo; confirm cron + off-site copy + restore drill.

---

## Data collection audit

What the gateway stores, where it lives, and who can read it.

| Data | Storage | Visibility | Notes |
|------|---------|------------|-------|
| Wallet address, profile, verified email | Postgres `users` | User; admin | Email linked only via OTP flow |
| Activity log (IP, UA, device, action) | Postgres `activity_log` | User `/account/activity`; admin fleet | Successful mutations on authed routes |
| VPN client records, configs | Postgres | User provisioning | WG pubkey reuse on reconnect |
| Node IP | Postgres `nodes.ip` | Not top-level in discovery | Injected as `endpoints.wireguard.host` for client ping |
| Node speedtest | Postgres + `GET /api/v2/nodes` | Public discovery | Node → internet throughput; from node heartbeats |
| Org enrollment secrets, API keys | Postgres | Org owners; `X-Api-Key` routes | API key plaintext shown once on create |
| PASETO tokens | Client-held | Not stored server-side | Signer derived from `MNEMONIC` |
| Prometheus metrics | `GET /metrics` | Internal scrape only | Request counts, latency, errors by route/client |
| Telemetry | `POST /telemetry/event` | Optional integration | Review payload before expanding |

### Data-hygiene actions

- Define activity-log retention (TTL or archive).
- Align IP logging with app privacy copy (`erebrus-vpn` privacy view).
- Encrypt backup dumps; store off-server (S3, rsync to second host).
- Never commit or log `.env` values; restrict `~/gateway/.env` to `600`.

---

## Pentest plan (non-disruptive)

**Constraints:** gateway must stay up; no destructive tests (no DROP, mass deletes, sustained 10k RPS).

Run from a **separate machine**, business hours, with an abort rule: stop if p95 latency > 2s or 5xx rate > 1% for 2 minutes.

### Phase 0 — Prep (~30 min)

1. Baseline: `curl /version`, `/healthz`, `/api/v2/nodes`; scrape `/metrics` from **inside** the server (`127.0.0.1:8080`).
2. Snapshot `docker stats`, error counters in Prometheus.
3. Assign on-call for rollback (re-pull previous `:prod` image tag).

### Phase 1 — Recon (passive)

- `nmap -sV gateway.erebrus.io` — expect 80/443 only.
- Confirm 5432 / 6379 / 8080 **not** reachable from the internet.
- TLS: SSL Labs or `testssl.sh`.
- Headers: `curl -I https://gateway.erebrus.io/healthz`.

### Phase 2 — Auth and authorization

| Test | Target | Expected |
|------|--------|----------|
| No bearer | `POST /api/v2/vpn/clients` | 401 |
| User token | `GET /api/v2/admin/stats` | 403 |
| Node token | `GET /api/v2/account/profile` | 403 |
| Bad / expired PASETO | any authed route | 401 |
| IDOR | `/orgs/{otherOrgId}/members` as member of org A | 403 |
| API key scope | org A key on org B resources | 401 / 403 |
| Auth replay | reuse spent `flow_id` + signature | 401 |
| Email OTP brute | 6+ wrong codes | 429 / lockout message |
| Node enroll | wrong `enrollment_secret` | 401 |

### Phase 3 — Input and logic

- Oversized JSON on register, profile patch, email endpoints.
- SQLi strings in `region`, `status` query params (should be parameterized).
- Path traversal on `:id` route params.
- CORS: `Origin: https://evil.com` from browser — no credentialed cross-origin abuse for allowed origins.
- Rate limit: hammer `GET/POST /api/v2/auth` → 429; **finding** if Redis stopped and limits disappear.

### Phase 4 — Infrastructure

- Traefik Docker socket mount (container escape surface).
- SSH: key-only, fail2ban, non-default port optional.
- `.env` permissions and backup destination security.
- GitHub `PROD_*` secret rotation drill.

### Phase 5 — Node control plane

- `GET /api/v2/nodes/ws` without node token → 401.
- Low-rate oversized WebSocket frames (no flood).

**Suggested tools:** Burp Suite Community, `httpx`, `nuclei` (low-rate templates), manual curl, light `k6` on auth flows only.

---

## Stress test plan (controlled load)

Find limits **without** outage.

### Rules

- Start **50 VUs max**; steps: 10 → 25 → 50.
- **3–5 min** per step, **2 min** cool-down between steps.
- **Read-heavy first:** `/healthz`, `GET /api/v2/nodes`, `GET /api/v2/subscriptions/plans`.
- Auth flows **≤ 5 RPS** (wallet challenge is expensive).
- Run from **2+ external IPs**.
- Watch: `docker stats`, internal `/metrics`, Postgres connections, Traefik CPU.

### Example k6 (read-only)

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE = __ENV.BASE || 'https://gateway.erebrus.io';

export const options = {
  vus: 25,
  duration: '3m',
};

export default function () {
  const r = Math.random();
  if (r < 0.7) {
    http.get(`${BASE}/api/v2/nodes?status=online`);
  } else if (r < 0.9) {
    http.get(`${BASE}/healthz`);
  } else {
    http.get(`${BASE}/version`);
  }
  sleep(0.5);
}
```

```bash
k6 run --vus 25 --duration 3m -e BASE=https://gateway.erebrus.io script.js
```

### Success criteria

- p95 < 500ms on `GET /api/v2/nodes` at 25 VUs.
- 0% 5xx during read load.
- Postgres connections stable (< 20).
- No OOM on gateway container.

### Do not stress yet (user impact)

- `POST /api/v2/vpn/clients` (provisions on nodes, writes DB).
- `POST /api/v2/auth/email` (Resend quota).
- Node WebSocket flood.
- Deploys during the test window.

---

## Quick fixes (low risk, do now)

1. **Uptime monitoring** — external check on `https://gateway.erebrus.io/healthz`; internal check on `http://127.0.0.1:8080/readyz`.
2. **Backup cron + monthly restore drill** — `erebrus-gateway-ops/scripts/backup-postgres.sh` to off-server storage.
3. **Redis `requirepass`** — infra compose + gateway `REDIS_PASSWORD`; brief cache miss blip only.
4. **SSH hardening** — key-only, disable password auth, fail2ban.
5. **Cloudflare proxy** on `gateway.erebrus.io` — DDoS/WAF; set `TRUSTED_PROXIES` to Cloudflare ranges + `172.18.0.0/16` (Traefik Docker network).
6. **Hide `/readyz` on public Traefik router** — extend rule: `!Path(/metrics) && !Path(/readyz)`; keep liveness checks on localhost.
7. **Deploy notes** — document rollback: `GATEWAY_IMAGE=ghcr.io/netsepio/gateway:<previous-sha> docker compose up -d`.
8. **Incident runbook** — restart order: postgres → redis → gateway → traefik.

---

## Major resilience (after pentest / stress findings)

| Tier | Change | Benefit |
|------|--------|---------|
| HA compute | Second gateway VM behind Traefik LB | Survive single container/host loss |
| Managed data | Managed Postgres (PITR) + Redis Cloud | Faster recovery, less ops risk |
| Edge | Cloudflare + rate limits at edge | Absorb abuse before origin |
| Observability | OTel collector profile + alerting on 5xx, latency, DB pool, WS drops | Detect before users |
| Secrets | Vault / cloud secret manager for `MNEMONIC` and API keys | Smaller blast radius |
| Chaos | Game-day: kill Redis, kill gateway, restore backup | Validate runbooks |

---

## Recommended order of work

1. **This week** — quick fixes above + read-only stress test (25 VU).
2. **Next** — pentest Phases 1–3 in a low-traffic window.
3. **After findings** — prioritize HA / managed DB based on what broke under load or what pentest reports.

---

## Related docs

- [GATEWAY.md](./GATEWAY.md) — architecture, deploy, metrics, QA checklist
- [gateway-api.openapi.yaml](./gateway-api.openapi.yaml) — HTTP surface
- [ws-protocol.md](./ws-protocol.md) — node WebSocket protocol
- `erebrus-gateway-ops/scripts/backup-postgres.sh` — Postgres backup (private ops repo)