# S3 — Entitlements (7d trial + 30d NFT-direct) — QA / acceptance

Scope: general trial 7 days (`trial_period` in `platform_settings`, default 168h);
NFT holders get 30 days directly (`nft_gate_period`, 720h), upgrading any active trial;
`GET /subscriptions.trial_consumed` = ever-started; entitlement gates provisioning;
admin bypass. Migration `0010` drops payment scaffolding (no `/payments`).

**Deploy:** API at `gateway.erebrus.io`; webapp CORS from `https://erebrus.io` and
`https://dev.erebrus.io` (`ALLOWED_ORIGIN`).

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

`config/platform_test` locks trial 168h, NFT 720h defaults (DB-seeded, not env).

## Live smoke test (run on a host with Postgres + Redis)

`$TOK` = user PASETO from S2-QA step 2.

1. **Trial is 7 days, once:**
   ```
   curl -s -X POST https://gateway.erebrus.io/api/v2/subscriptions/trial \
     -H "Authorization: Bearer $TOK"   # 201
   ```
   - Response `current_period_end` ≈ now + 168h.
   - To spot-check the knob: `PATCH /api/v2/admin/settings` with
     `{"settings":{"trial_period":"24h"}}`, then a **new** user trial ends ≈ +24h.
   - Repeat trial on same user → **409** already used.
   - `GET /api/v2/subscriptions` → `entitled:true, source:trial, trial_consumed:true`.

2. **Entitlement gates provisioning:**
   - Fresh user (no trial) → `POST /api/v2/vpn/clients` returns **402**.
   - After trial → provisioning succeeds (subject to plan `max_clients`).

3. **NFT-direct 30 days** (needs `NFT_GATE_CONTRACT` + DAS `NFT_GATE_RPC_URL`):
   - New user: `POST /subscriptions/nft/refresh` → **200**; `source:nft`, ~30d out.
   - Mid-trial user: refresh NFT → `source:nft` (~30d); `trial_consumed` stays true.
   - Wallet without NFT → **403**; NFT gating unset → **503**.

4. **Admin bypass:** admin PASETO → `GET /subscriptions` shows `source:admin`; provisioning works.

5. **No payments:** `/api/v2/payments*` → **404**; `crypto_payments` table dropped (0010).