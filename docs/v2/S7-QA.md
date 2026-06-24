# S7 — Social verify + perks + tier-gated pools — QA / acceptance

Scope: `nodes.min_tier` tier-gated premium pool (discovery + provisioning);
social verification (Telegram HMAC, X OAuth2 token, email linkage) →
`social_verified` XP; perks catalog + grants. Commits `96e73c4` + `17aba75` +
`3b42d09` (branch `v2`, local — **not pushed**). Migration `0007_social_perks`.

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

DB-free coverage: Telegram HMAC verify (roundtrip + tamper + stale auth_date),
X verify via httptest; migration embed guard → `0007`.

## Live smoke test (run on a host with Postgres + Redis)

Not exercised here (no local DB). Run before push to `main`.

1. **Migration:** `\d nodes` has `min_tier`; tables `social_accounts`, `perks`,
   `user_perks` exist; `schema_migrations` has `0007_social_perks.sql`.

2. **Tier-gated pool:** admin `POST /api/v2/admin/nodes/{id}/min_tier {"min_tier":2}`.
   - `GET /api/v2/nodes` **without** a token (tier 0) → node absent.
   - With a tier-0 user's token → absent; with a tier≥2 user (or admin) → present
     (and `min_tier:2` in the payload). Cache key carries the tier (no leak).
   - `POST /api/v2/vpn/clients` for that node as a tier-0 user → **403**; as
     tier≥2 / admin → proceeds.

3. **Social — Telegram:** with `TELEGRAM_BOT_TOKEN` set, POST a valid Login
   Widget payload to `/api/v2/social/telegram` → 200 `newly_linked:true`; a
   `social_verified,75` xp_event appears (once); second call → `newly_linked:false`,
   no extra XP. Tampered hash → 401. Unset bot token → 503. Same Telegram id from
   another account → 409.

4. **Social — X:** `POST /api/v2/social/x {access_token}` with a valid token
   (verified via `X_API_BASE_URL/2/users/me`) → links + `social_verified` once.
   `GET /api/v2/social/accounts` lists telegram + x (+ email if linked via
   `/auth/email`).

5. **Perks:** admin `POST /api/v2/admin/perks {id,name,type:node_pool,min_tier:2}`.
   - `GET /api/v2/perks` as a tier-1 user → that perk shows `unlocked:false`;
     tier-2 → `unlocked:true`.
   - admin `POST /api/v2/admin/perks/{id}/grant {wallet}` → `GET /perks/me` lists
     it; re-grant is idempotent.
