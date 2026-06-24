# S8b — Activity / audit log — QA / acceptance

Scope: every meaningful authenticated mutation recorded with IP + device;
`GET /account/activity` (own) + `GET /admin/activity` (fleet-wide). Commit
`ea30fc9` (branch `v2`, local — **not pushed**). Migration `0008_activity_log`.

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

DB-free coverage: `clientApp` platform classification; action-map shape;
migration embed guard through `0010`.

## Live smoke test (host with Postgres + Redis)

Not exercised here (no local DB). Run before push to `main`.

1. **Migration:** `\d activity_log` exists; `schema_migrations` has
   `0008_activity_log.sql`.

2. **Mutations are logged (middleware):** as a user, start a trial, provision +
   delete a VPN client, update profile, create an org. Then `GET
   /api/v2/account/activity` → entries `subscription.trial_start`,
   `vpn.client.provision`, `vpn.client.delete`, `profile.update`, `org.create`,
   newest first, each with `ip`, `device`, `app`, and `target` (the path id for
   the delete). Send `X-Erebrus-Client: erebrus-ios/1.0` → `app:ios`.

3. **Explicit events:** login (`POST /auth`) records `auth.login`; email request/
   verify record `auth.email.request` / `auth.email.verify`.

4. **Failures + reads not logged:** a 4xx mutation (e.g. duplicate trial → 409)
   writes **no** entry; GET requests write none.

5. **Pagination:** with > limit entries, the response includes `next_cursor`;
   passing it as `?cursor=` returns the next page (older), no overlap.

6. **Admin:** `GET /api/v2/admin/activity` returns fleet-wide entries with the
   actor's truncated `wallet`; admin actions (set min_tier, grant perk, node
   command) appear as `admin.*`. Non-admin → 403.

7. **IP correctness:** behind the proxy with `TRUSTED_PROXIES` set (S8), logged
   `ip` is the real client, not the proxy.
