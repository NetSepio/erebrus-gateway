# S2 — Auth (EVM+SOL, optional email) — QA / acceptance

Scope: `wallet/` limited to EVM+Solana (apt/sui removed); optional verified email
linking via Resend OTP. Commits `68fe987`, `22097cf`, `809e233` (branch `v2`,
local — **not pushed**).

## Automated (passing on this branch)

```
go build ./...   # ok
go vet ./...     # ok
go test ./...    # ok (wallet, mailer, api email helpers, store migrations embed)
gofmt -l internal/ cmd/   # clean for S2 files
ruby -ryaml -e "YAML.load_file('docs/v2/gateway-api.openapi.yaml')"  # valid
```

Unit coverage added: apt/sui/unknown → `ErrUnsupportedChain`; mailer request
shape + disabled + non-2xx (httptest); `normalizeEmail`/`generateOTP`/`hashOTP`;
migration embed presence + ordering.

## Live smoke test (run on a host with Postgres + Redis)

This machine has no Docker/Postgres, so the DB-dependent paths below are NOT yet
exercised. Run these before the eventual push to `main`.

Bring up deps + gateway (compose has Postgres+Redis):

```
docker compose up -d db redis
make build && ./bin/erebrus-gateway          # applies migrations on boot
```

1. **Migrations applied** — `0003_email_auth` recorded, schema present:
   ```sql
   SELECT version FROM schema_migrations ORDER BY version;          -- includes 0003_email_auth.sql
   \d users        -- has email_verified boolean NOT NULL DEFAULT false
   \d email_otps   -- id/user_id/email/code_hash/attempts/expires_at/created_at
   ```

2. **Chain validation** (apt/sui gone):
   ```
   curl -s 'localhost:8080/api/v2/auth?wallet_address=0xabc&chain=apt'   # 400 unsupported chain
   curl -s 'localhost:8080/api/v2/auth?wallet_address=0xabc&chain=evm'   # 200 {flow_id, message}
   curl -s 'localhost:8080/api/v2/auth?wallet_address=Sol..&chain=sol'   # 200
   ```
   Then complete a real Reown-signed login on **both** ETH mainnet and Solana;
   confirm the returned PASETO verifies on an authenticated route.

3. **Email disabled path** (no `RESEND_API_KEY`): with a valid user bearer,
   ```
   curl -s -X POST localhost:8080/api/v2/auth/email -H "Authorization: Bearer $TOK" \
        -H 'Content-Type: application/json' -d '{"email":"me@example.com"}'   # 503 not configured
   ```
   Unauthenticated → 401.

4. **Email enabled path** (`RESEND_API_KEY` + verified `RESEND_FROM` domain):
   - `POST /auth/email {email}` → 200 `{status:sent, expires_in}`; a 6-digit code
     arrives in the inbox.
   - Immediate resend → 429 (60s cooldown).
   - `POST /auth/email/verify {email, code}` with the wrong code 5× → 401 with a
     decreasing "attempts left", then 429; a fresh code resets attempts.
   - Correct code → 200, profile shows `email` + `email_verified:true`.
   - From a **second** account, `POST /auth/email` with the same email → 409.
   - Restart the gateway → email still linked (persisted), token still valid.

5. **Profile** — `PATCH /account/profile {email:"x@y.z"}` must NOT change the
   email (only `name` is patchable now); email is set only via the verified flow.
