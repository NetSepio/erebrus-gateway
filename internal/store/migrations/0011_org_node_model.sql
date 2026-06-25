-- Org workspace model + node enrollment auth (v2.1).
-- - Orgs carry a retrievable enrollment_secret (unlike one-time API keys).
-- - Nodes bind to org_id only (drop owner_user_id).
-- - api_token renamed to node_key (per-node bearer for gateway→node calls).
-- - access_mode is public | private only (shared removed).

-- ── org workspace fields ─────────────────────────────
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 'team';
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS verified boolean NOT NULL DEFAULT false;
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS slug text;
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS description text;
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS website text;
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS enrollment_secret text;
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

CREATE UNIQUE INDEX IF NOT EXISTS idx_orgs_slug ON orgs (slug) WHERE slug IS NOT NULL;

-- Backfill enrollment_secret for orgs created before this migration.
UPDATE orgs
SET enrollment_secret = 'ere_org_' || replace(gen_random_uuid()::text, '-', '')
WHERE enrollment_secret IS NULL OR enrollment_secret = '';

ALTER TABLE orgs ALTER COLUMN enrollment_secret SET NOT NULL;

-- org_members.role: owner | admin | member (validated in application code)

-- ── nodes: org-scoped, per-node key ──────────────────
UPDATE nodes SET access_mode = 'public' WHERE access_mode = 'shared' OR access_mode IS NULL OR access_mode = '';

ALTER TABLE nodes DROP COLUMN IF EXISTS owner_user_id;
DROP INDEX IF EXISTS idx_nodes_owner;

ALTER TABLE nodes RENAME COLUMN api_token TO node_key;