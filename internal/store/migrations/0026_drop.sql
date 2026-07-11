-- 0026: Erebrus Drop (Kubo-backed storage).
--
-- Adds Drop persistence and moves product entitlement to organization
-- membership only. Legacy personal `subscriptions` rows are intentionally
-- retained (not dropped) so a rollback window is available while the webapp
-- migrates; nothing here reads them at runtime anymore.

-- ── entitlement backfill ─────────────────────────────
-- Every account is expected to own a personal basic organization so the
-- organization-only resolver can always return at least the Free Drop tier.
-- Older accounts created before that invariant are backfilled here.
WITH orphans AS (
    SELECT u.id AS user_id
    FROM users u
    WHERE NOT EXISTS (SELECT 1 FROM orgs o WHERE o.owner_user_id = u.id)
),
ins_orgs AS (
    INSERT INTO orgs (name, slug, plan, billing_status, verification_status, owner_user_id)
    SELECT 'Personal Workspace',
           'ws-' || left(replace(gen_random_uuid()::text, '-', ''), 12)
                 || '-' || left(replace(user_id::text, '-', ''), 8),
           'basic', 'active', 'unverified', user_id
    FROM orphans
    RETURNING id, owner_user_id
),
ins_profiles AS (
    INSERT INTO org_profiles (org_id, display_name)
    SELECT id, 'Personal Workspace' FROM ins_orgs
    RETURNING org_id
),
ins_entitlements AS (
    INSERT INTO org_entitlements (org_id, plan, public_node_access_tier, support_tier)
    SELECT id, 'basic', 'free', 'community' FROM ins_orgs
    RETURNING org_id
)
INSERT INTO org_members (org_id, user_id, role, seat_tier, status)
SELECT id, owner_user_id, 'owner', 'free', 'active' FROM ins_orgs;

-- ── drop_tier_limits ─────────────────────────────────
-- Public Drop quota policy per effective tier. Admin-tunable without code change.
CREATE TABLE IF NOT EXISTS drop_tier_limits (
    tier                 text PRIMARY KEY,
    public_storage_bytes bigint NOT NULL,
    max_file_bytes       bigint NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

INSERT INTO drop_tier_limits (tier, public_storage_bytes, max_file_bytes) VALUES
    ('free',        500000000,   1000000000),
    ('starter',     1000000000,  1000000000),
    ('pro',         5000000000,  1000000000),
    ('business',    10000000000, 1000000000),
    -- Provisional: enterprise mirrors business until product review sets a value.
    ('enterprise',  10000000000, 1000000000)
ON CONFLICT (tier) DO NOTHING;

-- ── drop_quota_usage ─────────────────────────────────
-- Atomically maintained counters. v1 charges public storage per user across all
-- public nodes; principal_type leaves room for org-scoped quotas later.
CREATE TABLE IF NOT EXISTS drop_quota_usage (
    principal_type text   NOT NULL,
    principal_id   text   NOT NULL,
    used_bytes     bigint NOT NULL DEFAULT 0,
    reserved_bytes bigint NOT NULL DEFAULT 0,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (principal_type, principal_id),
    CHECK (used_bytes >= 0 AND reserved_bytes >= 0)
);

-- ── node_drop_status ─────────────────────────────────
-- Latest Drop capability/health/capacity for a node, from hello + heartbeat.
-- Exact capacity lives here (Postgres), never as Prometheus labels.
CREATE TABLE IF NOT EXISTS node_drop_status (
    node_id                text PRIMARY KEY,
    enabled                boolean NOT NULL DEFAULT false,
    accepts_public_uploads boolean NOT NULL DEFAULT false,
    webui_available        boolean NOT NULL DEFAULT false,
    state                  text NOT NULL DEFAULT 'disabled',
    kubo_version           text,
    repo_size_bytes        bigint NOT NULL DEFAULT 0,
    storage_max_bytes      bigint NOT NULL DEFAULT 0,
    num_objects            bigint NOT NULL DEFAULT 0,
    reserved_bytes         bigint NOT NULL DEFAULT 0,
    last_reported_at       timestamptz,
    updated_at             timestamptz NOT NULL DEFAULT now(),
    CHECK (reserved_bytes >= 0)
);

-- ── drop_uploads ─────────────────────────────────────
-- Short-lived reservation + idempotency record for one logical upload.
CREATE TABLE IF NOT EXISTS drop_uploads (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id              uuid REFERENCES orgs(id) ON DELETE SET NULL,
    entitlement_org_id  uuid REFERENCES orgs(id) ON DELETE SET NULL,
    node_id             text NOT NULL,
    storage_scope       text NOT NULL,
    visibility          text NOT NULL DEFAULT 'private',
    filename            text NOT NULL DEFAULT '',
    content_type        text NOT NULL DEFAULT '',
    declared_size_bytes bigint NOT NULL DEFAULT 0,
    reserved_bytes      bigint NOT NULL DEFAULT 0,
    sha256              text,
    encrypted           boolean NOT NULL DEFAULT false,
    encryption_metadata jsonb,
    status              text NOT NULL DEFAULT 'reserved',
    idempotency_key     text NOT NULL,
    cid                 text,
    error               text,
    expires_at          timestamptz NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_user_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_drop_uploads_owner ON drop_uploads (owner_user_id);
CREATE INDEX IF NOT EXISTS idx_drop_uploads_status_expiry ON drop_uploads (status, expires_at);

-- ── drop_files ───────────────────────────────────────
-- User-visible logical file, backed by one or more physical pins.
CREATE TABLE IF NOT EXISTS drop_files (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    upload_id           uuid REFERENCES drop_uploads(id) ON DELETE SET NULL,
    owner_user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id              uuid REFERENCES orgs(id) ON DELETE SET NULL,
    entitlement_org_id  uuid REFERENCES orgs(id) ON DELETE SET NULL,
    node_id             text NOT NULL,
    storage_scope       text NOT NULL,
    cid                 text NOT NULL,
    filename            text NOT NULL DEFAULT '',
    content_type        text NOT NULL DEFAULT '',
    size_bytes          bigint NOT NULL DEFAULT 0,
    visibility          text NOT NULL DEFAULT 'private',
    encrypted           boolean NOT NULL DEFAULT false,
    encryption_metadata jsonb,
    status              text NOT NULL DEFAULT 'active',
    created_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz
);
CREATE INDEX IF NOT EXISTS idx_drop_files_owner ON drop_files (owner_user_id) WHERE status <> 'deleted';
CREATE INDEX IF NOT EXISTS idx_drop_files_org ON drop_files (org_id) WHERE status <> 'deleted';
CREATE INDEX IF NOT EXISTS idx_drop_files_node_cid ON drop_files (node_id, cid);

-- ── drop_pins ────────────────────────────────────────
-- Physical pin state on one node. Multiple files may reference the same CID on
-- the same node; the CID is only unpinned when the final reference is removed.
CREATE TABLE IF NOT EXISTS drop_pins (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id     uuid NOT NULL REFERENCES drop_files(id) ON DELETE CASCADE,
    node_id     text NOT NULL,
    cid         text NOT NULL,
    status      text NOT NULL DEFAULT 'pinning',
    last_error  text,
    pinned_at   timestamptz,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (file_id, node_id)
);
CREATE INDEX IF NOT EXISTS idx_drop_pins_node_cid ON drop_pins (node_id, cid);
CREATE INDEX IF NOT EXISTS idx_drop_pins_status ON drop_pins (status);

-- ── drop_crypto_profiles ─────────────────────────────
-- Encrypted client-side Drop vault backup + public metadata only. The gateway
-- never stores the recovery secret or the plaintext vault/keys.
CREATE TABLE IF NOT EXISTS drop_crypto_profiles (
    user_id         uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    version         integer NOT NULL DEFAULT 1,
    public_key      text,
    encrypted_vault text NOT NULL,
    kdf_metadata    jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
