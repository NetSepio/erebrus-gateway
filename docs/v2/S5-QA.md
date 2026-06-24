# S5 — Referrals (+ XP ledger foundation) — QA / acceptance

Scope: shareable `referral_code`; `?ref=CODE` binds `referred_by_user_id` once on
signup; the referee's **first trial start** awards XP (referrer +100, referee
+25) into the new append-only `xp_events` ledger; `GET /referrals/me`. Commit
`ec6cfaa` (branch `v2`, local — **not pushed**). Migration `0005_referrals_xp`.

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

DB-free coverage: `genReferralCode` (length, alphabet, no ambiguous chars,
distinctness), `truncWallet`; migration embed guard updated for `0005`.

## Live smoke test (run on a host with Postgres + Redis)

Not exercised here (no local DB). Run before push to `main`. Use two accounts:
**A** (referrer) and **B** (referee), each a wallet PASETO.

1. **Migration applied:** `\d users` shows `referral_code, referred_by_user_id,
   xp_earned, xp_claimed, tier`; `\d xp_events` exists; `schema_migrations` has
   `0005_referrals_xp.sql`.

2. **Code is stable + shareable:** `GET /api/v2/referrals/me` as A returns a
   `code` (8 chars, no I/L/O/U). Call again → **same** code.

3. **Binding on signup:** authenticate B with `{...,"ref":"<A's code>"}`.
   `SELECT referred_by_user_id FROM users WHERE id=B` = A.
   - Re-auth B with a different ref → unchanged (immutable, one referrer).
   - Self-referral (A authenticates with A's own code) → not bound.
   - `GET /referrals/me` as A → `referred_count` includes B; `recent[]` lists B
     (truncated wallet, `qualified:false` until B starts a trial).
   - `GET /referrals/me` as B → `referred_by` = A's truncated wallet.

4. **Qualifying action awards XP once:** B `POST /api/v2/subscriptions/trial`.
   - `SELECT kind,points FROM xp_events WHERE user_id=A` → `referral_qualified,100`.
   - `SELECT ... WHERE user_id=B` → `referral_qualified,25`.
   - `SELECT xp_earned FROM users WHERE id IN (A,B)` → 100 / 25.
   - `GET /referrals/me` as A → B now shows `qualified:true`.
   - B retries trial → 409, **no** additional xp_events (fires once).

5. **No referrer = no XP:** a user C with no `referred_by` starts a trial → no
   `referral_qualified` events.

6. **Weights tunable:** set `XP_REFERRER_POINTS=250` → a new qualifying referral
   awards 250 to the referrer.
