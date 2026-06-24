-- 0007: social verification + perks + tier-gated node pools (§5d/§5e).
-- Additive + idempotent.

-- Tier-gated premium pool: a node's min_tier hides it from lower-tier callers in
-- discovery + provisioning. Admin-controlled (anti-gaming — not self-asserted by
-- the node). Default 0 = open pool (everyone).
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS min_tier int NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_nodes_min_tier ON nodes (min_tier);

-- Verified social accounts. Store only the provider id + handle, never tokens.
-- One provider account maps to one user (UNIQUE).
CREATE TABLE IF NOT EXISTS social_accounts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    text NOT NULL,                 -- x | telegram | email
    provider_id text NOT NULL,                 -- stable provider account id
    handle      text,                          -- @handle / username
    verified_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_id)
);
CREATE INDEX IF NOT EXISTS idx_social_accounts_user ON social_accounts (user_id);

-- Perks registry (admin catalog) + grants.
CREATE TABLE IF NOT EXISTS perks (
    id         text PRIMARY KEY,               -- slug
    name       text NOT NULL,
    type       text NOT NULL,                  -- nft | xp | free_days | node_pool
    min_tier   int  NOT NULL DEFAULT 0,
    meta       jsonb NOT NULL DEFAULT '{}',
    is_active  boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS user_perks (
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    perk_id    text NOT NULL REFERENCES perks(id) ON DELETE CASCADE,
    granted_at timestamptz NOT NULL DEFAULT now(),
    meta       jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY (user_id, perk_id)
);
