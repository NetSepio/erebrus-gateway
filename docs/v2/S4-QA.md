# S4 — Operator layer (ownership, access_mode, metrics) — QA / acceptance

Scope: nodes carry `owner_user_id` (from the registering wallet), optional
`org_id` (membership-validated), `access_mode` (from WS hello); `node_metrics`
time-series from the heartbeat ingest; operator + admin chart endpoints; private
nodes excluded from public discovery. Commits `3dea83a` + `c1b6690` (branch `v2`,
local — **not pushed**). Migration `0004_node_metrics`.

## Automated (passing on this branch)

```
go build ./... && go vet ./... && go test ./...   # ok
```

DB-free coverage: `parseDuration` range/step defaults + clamping; migration embed
guard updated for `0004`.

## Live smoke test (run on a host with Postgres + Redis + a real node)

This machine has no Docker/Postgres, so the DB/WS flows below are not exercised
here. Run before push to `main`.

1. **Migration applied:** `\d nodes` shows `owner_user_id, org_id, access_mode`;
   `\d node_metrics` exists (PK node_id+bucket). `SELECT version FROM
   schema_migrations` includes `0004_node_metrics.sql`.

2. **Ownership at registration:** register a node signing with wallet W →
   `SELECT owner_user_id FROM nodes` resolves to the user for W (created if new).
   - With `org_id` for an org W belongs to → node row has that `org_id`.
   - With an `org_id` W is NOT a member of → registration **403**.

3. **access_mode + discovery:** start the node with `access_mode=private` in its
   hello capabilities →
   - `GET /api/v2/nodes` (public) does **not** list it; `public`/`shared` nodes do
     (and carry `access_mode`).
   - `GET /api/v2/operator/nodes` (as owner W) **does** list it, with full detail.

4. **Metrics ingest + charts:** let the node send heartbeats for a few minutes.
   - `GET /api/v2/operator/nodes/{id}/metrics?range=1h&step=5m` → `points[]` with
     `wg_peers, proxy_sessions, rx_bytes, tx_bytes, cpu_pct, mem_pct` per bucket,
     ascending. `SELECT count(*) FROM node_metrics WHERE node_id=...` grows ~1/min.
   - A different (non-owner, non-org) user → **403** on that node's metrics.
   - Admin → `GET /api/v2/admin/nodes/{id}/metrics` returns the series for any node.
   - `range`/`step` honored; absurd values clamped (range≤90d, step≥1m).

5. **Retention:** with `NODE_METRICS_RETENTION=1h`, buckets older than 1h are
   pruned by the maintenance loop (insert a backdated row, wait a tick, confirm
   it's gone).
