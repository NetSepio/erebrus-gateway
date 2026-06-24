# S8 — Production hardening — QA / acceptance

Scope: per-IP rate limiting, trusted proxies, `/readyz`, `/metrics`, CORS header.
Commits `6c9e44d` + `4543614` (branch `v2`, local — **not pushed**). No migration.

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

## Live smoke test (host with Postgres + Redis)

1. **/readyz + /metrics:** with DB up, `GET /readyz` → 200 `{status:ready,db:up,
   redis:up|disabled}`; stop Postgres → 503. `GET /metrics` → Prometheus text with
   `gateway_http_requests_total{class=...}`, `gateway_ws_nodes_connected`,
   `gateway_db_open_connections/in_use/idle`. `/healthz` still 200 (liveness).

2. **Rate limiting:** hammer `GET /api/v2/auth?...` > `RATE_LIMIT_AUTH_PER_MIN`
   from one IP within a minute → 429 + `Retry-After: 60`; a new minute resets.
   `POST /nodes/register` limited at `RATE_LIMIT_REGISTER_PER_MIN`. Stop Redis →
   requests still succeed (fail-open). Set `RATE_LIMIT_AUTH_PER_MIN=0` → disabled.

3. **Trusted proxies:** behind the reverse proxy, set `TRUSTED_PROXIES` to the
   proxy IP and confirm rate-limit/activity IPs are the real client (not the
   proxy). Empty `TRUSTED_PROXIES` → ClientIP is the direct peer (XFF ignored).

4. **CORS:** preflight from a non-allowed origin is rejected; `ALLOWED_ORIGIN`
   origins pass; `X-Erebrus-Client` is an accepted request header.

## Ops items (documented here; enforce at deploy — not gateway code)

- **DB statement_timeout:** set a server/role-level `statement_timeout` (e.g.
  `ALTER ROLE erebrus SET statement_timeout = '15s';`) as defense-in-depth.
  Per-request context timeouts already bound handler queries; pool caps are set
  in `store.Open` (MaxOpenConns=25/MaxIdle=5/MaxLifetime=30m).
- **Gateway→node transport:** the node API is plaintext HTTP today (mirrors the
  node audit's F3). In prod, require HTTPS or a private network between
  gateway↔node and register nodes with an `https://…` `api_base_url`. Firewall the
  node API to the gateway.
- **/metrics exposure:** scrape over a private network / firewall it; it is
  unauthenticated by design for Prometheus.
- **Backups/PITR:** enable managed Postgres PITR or scheduled `pg_dump`.
