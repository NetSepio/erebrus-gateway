# S6 — XP / tiers / leaderboard / earn-claim — QA / acceptance

Scope: tiers from lifetime `xp_earned`; `GET /rank/me`; XP drivers; `GET /leaderboard`;
`POST /rank/claim`. Thresholds and costs live in `platform_settings` (defaults in
migration `0009`). Migration `0006_xp_claims`.

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

DB-free: `tierForXP`, `TierThresholds`, `clampInt`; migration embed through `0006`.

## Live smoke test (Postgres + Redis)

1. **Migration:** `xp_claims`, `xp_events.dedup_key`; `0006` applied.

2. **Tiers + rank/me:** user with ~125 XP → `tier:1`, `next_tier_at:500`, `xp_claimable` matches earned.

3. **Idempotent drivers:** email (+25 once), NFT (+50/mo), uptime (+20/node/day, cap 5).

4. **Leaderboard:** `GET /leaderboard?metric=xp&period=all` ordered; `my_rank` present; 30s Redis cache.

5. **Earn vs claim:** with claimable ≥ `xp_free_days_cost` (default 500):
   ```
   POST /api/v2/rank/claim {"reward":"free_days"}
   ```
   → `xp_claimed` +500, `source:rank` entitlement +7 days. Too little claimable → 409.

6. **Tunable via admin:** `PATCH /api/v2/admin/settings`:
   ```json
   {"settings":{"xp_free_days_cost":"400","xp_tier_thresholds":"0,100,500,2000,10000"}}
   ```
   New XP awards use updated tier thresholds immediately.