# S3 — Entitlements (7d trial + 30d NFT-direct) — QA / acceptance

Scope: general trial 7 days (config `TRIAL_PERIOD`, default 168h); NFT holders
get 30 days directly (`NFT_GATE_PERIOD`, 720h), upgrading any active trial;
`GET /subscriptions.trial_consumed` = ever-started; entitlement gates
provisioning; admin bypass. Commit `fbc2c84` (branch `v2`, local — **not pushed**).
No new migration.

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

`config_test.go` locks the product numbers without a DB: trial default **168h
(7d)**, NFT default **720h (30d)**, token **24h**, plus the `TRIAL_PERIOD`
override path.

## Live smoke test (run on a host with Postgres + Redis)

This machine has no Docker/Postgres, so the DB-backed flows below are not yet
exercised. Run before push to `main`. (`$TOK` = a user PASETO from §S2-QA step 2.)

1. **Trial is 7 days, once:**
   ```
   curl -s -X POST localhost:8080/api/v2/subscriptions/trial -H "Authorization: Bearer $TOK"   # 201
   ```
   - Response `current_period_end` ≈ now + 168h (set `TRIAL_PERIOD=24h` to spot-check the knob).
   - Repeat → **409** already used.
   - `GET /api/v2/subscriptions` → `entitled:true, source:trial, status:active, trial_consumed:true`.

2. **Entitlement gates provisioning:**
   - Fresh user (no trial) → `POST /api/v2/vpn/clients` returns **402** "no active subscription".
   - After starting the trial → provisioning succeeds (subject to plan `max_clients`).

3. **NFT-direct 30 days, upgrades trial** (needs `NFT_GATE_CONTRACT` +
   DAS `NFT_GATE_RPC_URL`, and a wallet holding the collection):
   - As a brand-new user (no trial): `POST /api/v2/subscriptions/nft/refresh` →
     **200**; `GET /subscriptions` → `source:nft`, `current_period_end` ≈ now+30d.
   - As a user **mid 7-day trial**: refresh NFT → `GET /subscriptions` now reports
     `source:nft` and ~30d out (the NFT row outlasts the trial). `trial_consumed`
     stays **true** (they did use the trial earlier).
   - Re-call refresh while held → period extends (one `nft` row, idx_subs_one_nft).
   - Wallet without the NFT → **403**; gateway with NFT gating unset → **503**.

4. **Admin bypass:** with an admin PASETO, `GET /subscriptions` →
   `entitled:true, source:admin`; provisioning works with no subscription row.

5. **No money:** `/api/v2/payments*` must **404** (routes removed; spec updated).
