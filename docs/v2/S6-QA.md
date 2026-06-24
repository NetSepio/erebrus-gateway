# S6 — XP / tiers / leaderboard / earn-claim — QA / acceptance

Scope: tiers from lifetime `xp_earned`; `GET /rank/me`; XP drivers wired
(email_verified, nft_held, operator_uptime_day; referrals from S5); `GET
/leaderboard` (Redis-cached + my_rank); earn-vs-claim `xp_claims` + `POST
/rank/claim`. Commits `4fb4aab` + `643553f` (branch `v2`, local — **not pushed**).
Migration `0006_xp_claims`.

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

DB-free coverage: `tierForXP` (all band boundaries), `TierThresholds`
default/override, XP config defaults, `clampInt`; migration embed guard → `0006`.

## Live smoke test (run on a host with Postgres + Redis)

Not exercised here (no local DB). Run before push to `main`.

1. **Migration:** `\d xp_claims` exists; `\d xp_events` has `dedup_key` with a
   partial unique index; `schema_migrations` has `0006_xp_claims.sql`.

2. **Tiers + rank/me:** with a user who earned 120 XP (e.g. one qualified
   referral as referrer +100 plus email_verified +25 = 125), `GET
   /api/v2/rank/me` → `tier:1 (Connected)`, `next_tier_at:500`,
   `breakdown_by_kind` lists the kinds, `xp_claimable == xp_earned` (none claimed).

3. **Drivers (idempotent):**
   - Verify an email → one `email_verified,25` event; verify again (re-link) → no
     second event (dedup `email_verified:<uid>`).
   - NFT refresh (gated env + holding wallet) → `nft_held,50`; refresh again same
     month → no duplicate (dedup `nft_held:<uid>:<yyyymm>`).
   - A node online with recent heartbeats → after a maintenance tick its owner
     gets `operator_uptime_day,20`; further ticks the same UTC day add nothing
     (dedup `uptime:<node>:<yyyymmdd>`); cap 5 nodes/owner.

4. **Leaderboard:** `GET /api/v2/leaderboard?metric=xp&period=all` → entries
   ordered by `xp_earned` desc with ranks; `my_rank`/`my_value` present.
   `metric=referrals` ranks by qualified referrals; `period=30d` windows by
   `xp_events.created_at`. Repeated calls within 30s are Redis-cached. Bad metric/
   period → 400.

5. **Earn vs claim:** with claimable ≥ `XP_FREE_DAYS_COST` (500), `POST
   /api/v2/rank/claim {"reward":"free_days"}` → 200; `rank/me` shows `xp_claimed`
   up by 500, `xp_earned` unchanged (tier holds), `xp_claimable` down by 500;
   `GET /subscriptions` shows a `source:rank` entitlement extended by 7 days on
   top of any existing period; an `xp_claims` row logs ip+device. Claiming with
   too little claimable → 409.
