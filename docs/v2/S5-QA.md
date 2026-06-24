# S5 — Referrals (+ XP ledger foundation) — QA / acceptance

Scope: shareable `referral_code`; `ref` in `POST /api/v2/auth` binds referrer once;
referee's **first trial start** awards XP (defaults: referrer 100, referee 25 from
`platform_settings`); `GET /referrals/me`. Migration `0005_referrals_xp`.

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

DB-free: `genReferralCode`, migration embed guard through `0005`.

## Live smoke test (Postgres + Redis)

Accounts **A** (referrer) and **B** (referee), each with a wallet PASETO.

1. **Migration:** `users.referral_code`, `referred_by_user_id`, `xp_events`; `0005` applied.

2. **Code:** `GET /api/v2/referrals/me` as A → stable 8-char `code`.

3. **Binding on signup:** `POST /api/v2/auth` as B with `{flow_id, signature, ref:"<A's code>"}`.
   - `referred_by_user_id` = A; immutable on re-auth.
   - Self-referral → not bound.

4. **Qualifying XP once:** B `POST /subscriptions/trial` → A gets +100, B +25 in `xp_events`.
   - B retries trial → 409, no extra XP.

5. **No referrer:** user C (no referrer) starts trial → no `referral_qualified` events.

6. **Weights tunable (no restart):** admin `PATCH /api/v2/admin/settings`:
   ```json
   {"settings":{"xp_referrer_points":"250"}}
   ```
   New qualifying referral awards 250 to referrer.