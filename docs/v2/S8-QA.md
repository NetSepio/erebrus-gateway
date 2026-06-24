# S8 — Production hardening — QA / acceptance

Scope: per-IP rate limiting, trusted proxies, `/readyz`, `/metrics`, CORS.
Rate limits configured in `platform_settings` (`rate_limit_auth_per_min`,
`rate_limit_register_per_min`). No migration.

**Deploy:** `gateway.erebrus.io` behind reverse proxy; webapp origins in `ALLOWED_ORIGIN`.

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

## Live smoke test (Postgres + Redis)

1. **/readyz + /metrics:** DB up → `/readyz` 200; DB down → 503. `/metrics` exposes
   `gateway_http_requests_total`, `gateway_ws_nodes_connected`, DB pool stats.

2. **Rate limiting:** hammer `GET /api/v2/auth?...` over `rate_limit_auth_per_min`
   (default 30) from one IP → 429 + `Retry-After`. Redis down → fail-open.
   Tune live: `PATCH /api/v2/admin/settings {"settings":{"rate_limit_auth_per_min":"60"}}`.
   Set `rate_limit_auth_per_min` to `0` → disabled.

3. **Trusted proxies:** with `TRUSTED_PROXIES` set to the proxy IP, rate-limit and
   activity log see the real client IP.

4. **CORS:** `https://erebrus.io` and `https://dev.erebrus.io` pass preflight when
   listed in `ALLOWED_ORIGIN`; `X-Erebrus-Client` accepted.

## Ops items (deploy-time, not gateway code)

- DB `statement_timeout` on the `erebrus` role.
- Gateway→node HTTPS or private network for node API calls.
- Firewall `/metrics` to internal scrapers.
- Postgres PITR / backups.