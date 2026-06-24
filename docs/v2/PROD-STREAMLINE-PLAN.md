# Erebrus Gateway v2 — Streamline & Production Plan

Goal: cut the codebase down to the v2 surface, lock the auth/entitlement model to
**Reown + Solana/Ethereum mainnet + Resend**, finish the **node-operator** views,
and add the **social layer** (referrals, leaderboard, XP/ranking → perks & faster
nodes). One repo, one binary, Postgres + Redis, Docker-first.

---

## 0. Current state (grounded)

- **v2 = `internal/gw/*`** (config, store [2 migrations], token PASETO, wallet
  verify, identity, nodehub WS, nodeclient, nftgate, cache, api [10 handlers,
  full route surface]). Entry: `cmd/gateway/main.go`.
- **v1 legacy still present and compiled-adjacent:** `internal/{api, caching,
  database, p2p-Node, routines, server}`, `models/`, `app/`, `utils/`, and a
  second entry `cmd/{main.go, server.go}` (gorm `app.Init`).
- **Dead weight in `go.mod`:** `jinzhu/gorm`+`gorm.io/*` (v1 ORM), `libp2p/*`
  (v1 status pubsub), `chromedp/*` (v1 scraping), `sashabaranov/go-openai`
  (Cyrene AI), most of `go-ethereum` (only sig-recovery is needed).
- **Keep:** `lib/pq` (or move to `pgx`), `redis/go-redis`, `resend-go`,
  `vk-rv/pvx` (PASETO), a thin EVM ecrecover, Solana base58/ed25519.

---

## 1. Streamlining (PR S1 — do first, unblocks everything)

1. Delete v1: `internal/{api,caching,database,p2p-Node,routines,server}`,
   `models/`, `app/`, `utils/` (audit `utils/` for anything v2 reuses — move the
   2–3 helpers into `internal/gw/...` first), `cmd/main.go`, `cmd/server.go`,
   `contract/`, the v1 `Dockerfile`.
2. One entry point: `cmd/gateway/main.go` → rename to `cmd/erebrus-gateway`.
   `Dockerfile.v2` → `Dockerfile`; `docker-compose.v2.yml` → `docker-compose.yml`.
3. `go mod tidy` — expect ~40% fewer deps; drop gorm/libp2p/chromedp/openai.
4. CI: build/vet/test `./internal/gw/... ./cmd/...` only; gitleaks stays.
5. Acceptance: `go build ./... && go vet ./... && go test ./...` green with no
   `internal/gw`-external packages; image builds; no behavior change to `/api/v2`.

> This is the single biggest PROD-readiness win and makes the repo legible.

---

## 2. Auth — Reown + Solana/Ethereum mainnet + Resend (PR S2)

Frontend uses **Reown AppKit** only; backend verifies signatures + issues PASETO.

- **Wallet login** (unify already done under `GET/POST /api/v2/auth`):
  keep **EVM (ethereum)** and **Solana** only. **Drop Aptos/Sui** verifiers and
  their deps. Validate `chain ∈ {evm, sol}`. Bind the challenge to a nonce +
  short TTL (already via `flow_ids`); message text says "Erebrus".
- **Email (Resend), fully optional:** `POST /api/v2/auth/email` (send 6-digit
  OTP / magic link via Resend) + `POST /api/v2/auth/email/verify`. Email only
  *links* to the wallet account (for perks/ranking + recovery); it is never
  required to use the VPN.
- **Profile optional:** `account/profile` already PATCHable; nothing required.
- **PASETO:** signer key already derived from `MNEMONIC` (stable across restarts).
  Confirm prod sets a real `MNEMONIC`; 24h tokens; `role` claim drives admin.
- Acceptance: login with a Reown-signed Solana **and** ETH mainnet message;
  optional email-link flow; tokens verify; apt/sui removed.

---

## 3. Entitlements — trial + NFT + rank perks (PR S3)

- **General trial: 7 days**, one per user (`idx_subs_one_trial`). _Change the
  current 14d constant to 7d._
- **NFT holders: 30-day, directly.** Proving the gating NFT grants a fresh 30-day
  entitlement regardless of trial state — a new user goes straight to 30 days; a
  user mid-7d-trial is upgraded to 30. Refreshable while held (`nftgate`, Solana
  Metaplex Core via DAS today; ETH ERC-721 checker already present). One per user
  (`idx_subs_one_nft`).
- `GET /api/v2/subscriptions` returns `{status, source, entitled, trial_consumed,
  current_period_end}` (trial_consumed already added).
- **No money in v2** (locked). Rank-based perks (below) can *grant* entitlement
  (`source='rank'`) — e.g. top referrers get free days.
- Acceptance: 7d trial; NFT 30d; admin bypass; entitlement gates provisioning.

---

## 4. Node-operator layer — ownership, visibility, charts (PR S4)

Operators log in (wallet) and see **their** nodes (public, private, org) with
live + historical charts.

- **Ownership:** nodes already carry `wallet_address` (operator). Add
  `owner_user_id` (resolve at registration from the signing wallet) and optional
  `org_id`. **Visibility/access:** persist `access_mode ∈ {public, private}` from
  the node hello (the WS `Capabilities.access_mode` exists); `org` = node with an
  `org_id`.
- **Time-series:** new `node_metrics` rollup (per node, per minute/hour:
  `wg_peers, proxy_sessions, rx_bytes, tx_bytes, cpu_pct, mem_pct`), written from
  the existing heartbeat/usage_report ingest. Retention via a periodic prune.
- **Endpoints:**
  - `GET /api/v2/operator/nodes` — my nodes (public/private/org), live snapshot.
  - `GET /api/v2/operator/nodes/:id/metrics?range=24h&step=5m` — charts (peers,
    bandwidth, load).
  - Admin already has fleet-wide views; add `GET /api/v2/admin/nodes/:id/metrics`.
- Acceptance: operator sees only owned nodes; charts render bandwidth + peers
  over a selectable window; private nodes never appear in public discovery.

---

## 5. Social layer — referrals, leaderboard, XP/ranking, perks (PR S5–S7)

The differentiator. "Prove your social layer → faster nodes + perks."

### 5a. Referrals (PR S5)
- Each user gets a stable `referral_code` (short, shareable). On signup, an
  optional `?ref=CODE` binds `referred_by_user_id` (immutable, self-referral
  blocked, one referrer per user).
- `GET /api/v2/referrals/me` → `{code, referred_count, referred_by, recent[]}`.
- **Qualifying action = the referee's first trial start** → referrer earns
  **+100 XP** (referee +25). XP is *earned* immediately; rewards are *claimed*
  separately (see §5b). `free_days` for top-N referrers each month is a
  claimable perk.

### 5b. XP & ranking (PR S6) — DRIVERS LOCKED
Four XP sources (all confirmed). `xp_events(user_id, kind, points, meta,
created_at)` is an append-only ledger; `users.xp` is the cached sum; `users.tier`
is derived. Starting weights are **tunable** (config, not hard-coded):

| Driver | Event kind | XP | Cadence | Anti-gaming |
|---|---|---|---|---|
| **Referrals** | `referral_qualified` | +100 referrer / +25 referee | per qualified referee | only on the referee's *qualifying* action (not signup); cap N/period; unique referee; self-referral blocked |
| **Social verification** | `social_verified` | +75 | once per provider (max 3) | one provider id → one user; provider-account age check |
| **Operator uptime** | `operator_uptime_day` | +20 | per healthy node, per UTC day | node must pass heartbeat health (consistent beats, min hours/day); cap ~5 nodes counted; ties S4 ownership |
| **NFT held** | `nft_held` | +50 | monthly while held | proven via `nftgate` DAS refresh |
| **Email verified** | `email_verified` | +25 | once | Resend OTP |

- **Tiers** (XP thresholds, tunable): T0 Newcomer 0 · T1 Connected 100 ·
  T2 Contributor 500 · T3 Guardian 2 000 · T4 Architect 10 000. `users.tier` is
  recomputed on each XP event; tier drives perks + node-pool access (5e).
- `GET /api/v2/rank/me` → `{xp, tier, next_tier_at, breakdown_by_kind}`.
- **Operator↔user synergy:** uptime XP means a node operator's infra
  contribution raises their *own* rank → they get faster nodes as a user. One
  identity, both roles.
- **Earn vs claim (auditable).** XP is *earned* into the append-only `xp_events`
  ledger; **lifetime earned XP drives rank/tier and never decreases.** Rewards
  (`free_days`, NFT eligibility, other perks) are *claimed* by the user at any
  time, each recorded in `xp_claims(user_id, kind, xp_spent, reward, ip, device,
  created_at)`. `users.xp_earned` vs `users.xp_claimed` are cached sums, so the
  UI always shows **earned, claimed, and claimable**. Every claim is also an
  activity-log entry (§6.5) with IP + device.
- `POST /api/v2/rank/claim` `{reward_id}` → grants the reward, logs the claim.

### 5c. Leaderboard (PR S6)
- `GET /api/v2/leaderboard?metric=referrals|xp&period=all|30d` (Redis-cached,
  paginated) → rank, handle/wallet (truncated), count/xp. Plus my own rank.

### 5d. Social verification (PR S7) — providers: X, Telegram, email
- **X (Twitter)** and **Telegram** via OAuth / bot-proof, plus **email** (the
  Resend OTP flow already in §2) → `social_accounts(user_id, provider,
  provider_id, handle, verified_at)`, `UNIQUE(provider, provider_id)`. Each first
  verification emits `social_verified` XP (email emits `email_verified`).
- Providers are pluggable; ship X + Telegram + email now. Privacy: store the
  provider id + handle only, never tokens.

### 5e. Perks → faster nodes — MECHANIC LOCKED: tier-gated premium pool
- **Tier-gated node pool:** premium/high-throughput nodes carry `min_tier`
  (default 0). Discovery (`GET /api/v2/nodes`) and provisioning filter by the
  caller's `tier`, so higher tiers *see and can connect to* the fast pool;
  everyone keeps the open pool. No per-node QoS work required on the node side.
- Perks registry: `perks(id, name, type[nft|xp|free_days|node_pool], min_tier,
  meta)`; `user_perks` grants. NFT-collection perks reuse `nftgate` for proof or
  a future mint hook. Top referrers/tiers can be granted `free_days`
  (entitlement `source='rank'`).
- Acceptance: referrals tracked both directions; leaderboard ranks; XP/tier
  computed from the 4 drivers; **tier-gated fast pool enforced in discovery +
  provisioning**; perks grantable + queryable.

---

## 6. Production hardening (PR S8 — parallel)

- **Secrets/config:** all via env (already), `sslmode=require`, real `MNEMONIC`,
  `RESEND_API_KEY`, treasury/RPC for nftgate. `.env.example` audited.
- **Migrations:** keep explicit SQL runner; all new tables additive +
  idempotent; never automigrate. Add `0003_node_metrics`, `0004_social_xp`,
  `0005_referrals_perks`.
- **Rate limiting:** per-IP on `/auth/*` and `/nodes/register`; per-API-key on
  org routes (already metered). Use Redis token-bucket.
- **CORS:** lock `ALLOWED_ORIGIN` to the webapp origins (already configurable).
- **Observability:** structured logs (done), add Prometheus `/metrics`
  (request counts, WS conns, provision latency, DB pool), `/healthz` + `/readyz`
  (DB+Redis ping).
- **Node↔gateway transport:** the node API is HTTP today; require TLS or a
  private network between gateway↔node in prod (mirror the node audit's F3).
  Gateway→node calls should use the per-node `api_base_url` over HTTPS.
- **DB:** connection pool caps set; add statement timeouts; backups/PITR doc.
- **Abuse:** wallet-login is free → Sybil risk for the social layer; gate XP/
  perks behind *qualifying* actions (NFT, social verify, paid-equivalent) not
  raw signups; cap referral XP; leaderboard anti-gaming (unique referee actions).

---

## 6.5 Activity & audit log (PR S8b) — full user visibility

Every meaningful action a user takes in the webapp **or** mobile app is recorded
and shown back to them in an **Activity** section — a security feature so users
can spot anything they didn't do and react.

- **What's logged:** auth (login, email/social verify), VPN client
  provision/delete, org + API-key create/revoke, subscription/trial/NFT changes,
  XP claims, node ownership changes, profile edits. (Never traffic content — this
  is account activity, not browsing.)
- **Each entry:** `activity_log(id, user_id, action, target, ip, user_agent,
  device, app[web|ios|android|desktop], meta jsonb, created_at)`. IP + device are
  captured server-side from the request (`X-Forwarded-For` behind the proxy +
  `User-Agent`; the apps also send an `X-Erebrus-Client` device hint).
- **Capture:** a Gin middleware on authenticated mutating routes writes an entry
  after success; sensitive values are referenced by id, never dumped.
- **Endpoints:** `GET /api/v2/account/activity?cursor=&limit=` (the user's own,
  paginated, newest first); admins get `GET /api/v2/admin/activity` fleet-wide
  (already stubbed) for incident response.
- **Anomaly hooks (later):** flag new-IP / new-country / new-device logins; the
  schema supports it now, alerting can come after.

## 7. Schema additions (summary)

```
ALTER nodes ADD owner_user_id uuid, org_id uuid NULL, access_mode text, min_tier int DEFAULT 0;
  -- owner_user_id resolved from the registering wallet; org_id set by the operator
  -- when starting the node (orgs are the existing API-key orgs, created via API).
node_metrics(node_id, bucket timestamptz, wg_peers, proxy_sessions, rx_bytes, tx_bytes, cpu_pct, mem_pct)  PK(node_id, bucket)
users ADD referral_code text UNIQUE, referred_by_user_id uuid NULL,
  xp_earned bigint DEFAULT 0, xp_claimed bigint DEFAULT 0, tier int DEFAULT 0,
  email text, email_verified bool;
xp_events(id, user_id, kind, points, meta jsonb, created_at)              -- earned ledger (lifetime)
xp_claims(id, user_id, kind, xp_spent bigint, reward, ip, device, meta jsonb, created_at)  -- claim ledger
social_accounts(user_id, provider['x'|'telegram'|'email'], provider_id, handle, verified_at)  UNIQUE(provider, provider_id)
perks(id, name, type['nft'|'xp'|'free_days'|'node_pool'], min_tier, meta jsonb)
user_perks(user_id, perk_id, granted_at, meta)
activity_log(id, user_id, action, target, ip, user_agent, device, app, meta jsonb, created_at)  -- §6.5
```

---

## 8. Sequencing (PRs, each: tests + migration notes + docs)

```
S1  streamline: delete v1, one binary, prune deps          ✅ DONE (7eb96f3)
S2  auth: Reown/EVM+SOL only, Resend email (optional)
S3  entitlements: 7d trial + 30d NFT-direct + rank source
S4  operator nodes: ownership, visibility, node_metrics + charts
S5  referrals (qualify on referee trial start)
S6  XP earn/claim, tiers, leaderboard
S7  social verification (X/Telegram/email) + perks + tiered node pools
S8  prod hardening (parallel with S4–S7)
S8b activity & audit log (IP + device) — webapp Activity section
```

---

## 9. Decisions

**Resolved:**
- ✅ **Faster nodes** = tier-gated premium pool (`nodes.min_tier`), enforced in
  discovery + provisioning. (§5e)
- ✅ **XP drivers** = referrals + social verification + operator uptime + NFT
  held + email verified, with the weights/anti-gaming in §5b.

**All resolved (2026-06-21):**
1. ✅ **Trial vs NFT** — coexist; **NFT grants 30d directly** (new user → 30d;
   trial user → upgraded to 30d).
2. ✅ **Referral** — qualifies on the **referee's first trial start** → referrer
   **+100 XP** (earned). XP is claimable anytime; every claim logged
   (`xp_claims`, earned-vs-claimed). `free_days` for top-N monthly referrers.
3. ✅ **Social providers** — **X, Telegram, email** only for now.
4. ✅ **Org nodes** — reuse the existing `orgs`; the **operator sets `org_id`
   when starting the node**. Orgs (id + details) and **API keys** are created
   from the webapp/mobile via the org APIs, so operators can build their own apps
   on Erebrus infra.
5. ✅ **Activity log** — every webapp/mobile action logged with **IP + device**
   and shown in the webapp **Activity** section for security visibility. (§6.5)
```
