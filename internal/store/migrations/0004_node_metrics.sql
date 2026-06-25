-- 0004: node-operator layer — ownership, access visibility, time-series metrics
-- (PROD-STREAMLINE-PLAN §4). Additive + idempotent.

-- Ownership + visibility on nodes.
--   owner_user_id: resolved from the wallet that registers the node (the operator
--     account). ON DELETE SET NULL so removing a user doesn't drop their nodes.
--   org_id: optionally set by the operator at node start, validated against org
--     membership; makes the node visible to the org.
--   access_mode: persisted from the WS hello capabilities. 'private' nodes are
--     excluded from public discovery; existing rows default to 'public'.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS owner_user_id uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS org_id uuid REFERENCES orgs(id) ON DELETE SET NULL;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS access_mode text NOT NULL DEFAULT 'public'; -- public | shared | private
CREATE INDEX IF NOT EXISTS idx_nodes_owner ON nodes (owner_user_id);
CREATE INDEX IF NOT EXISTS idx_nodes_org ON nodes (org_id);

-- Per-node time-series rollup: one row per node per minute bucket, written from
-- the heartbeat ingest (last write in a bucket wins). wg_peers/proxy_sessions are
-- gauges; rx_bytes/tx_bytes are the node's cumulative interface counters (charts
-- diff consecutive points for throughput). Retention prune drops old buckets.
CREATE TABLE IF NOT EXISTS node_metrics (
    node_id        uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    bucket         timestamptz NOT NULL,
    wg_peers       int    NOT NULL DEFAULT 0,
    proxy_sessions int    NOT NULL DEFAULT 0,
    rx_bytes       bigint NOT NULL DEFAULT 0,
    tx_bytes       bigint NOT NULL DEFAULT 0,
    cpu_pct        double precision NOT NULL DEFAULT 0,
    mem_pct        double precision NOT NULL DEFAULT 0,
    PRIMARY KEY (node_id, bucket)
);
CREATE INDEX IF NOT EXISTS idx_node_metrics_bucket ON node_metrics (bucket);
