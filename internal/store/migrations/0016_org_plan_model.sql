-- Stage 1: Org-based plan model — orgs, profiles, members (seat tiers), entitlements.
-- Backward compatibility not required; destructive org schema changes.

-- ── org_profiles ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS org_profiles (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL UNIQUE REFERENCES orgs(id) ON DELETE CASCADE,
    legal_name      text,
    display_name    text,
    description     text,
    logo_url        text,
    website_url     text,
    public_email    text,
    billing_email   text,
    support_email   text,
    country         text,
    timezone        text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

INSERT INTO org_profiles (org_id, display_name, description, website_url)
SELECT id, name, description, website
FROM orgs
ON CONFLICT (org_id) DO NOTHING;

-- ── orgs: plan + billing fields ──────────────────────
UPDATE orgs
SET slug = lower(regexp_replace(regexp_replace(COALESCE(name, 'org'), '[^a-zA-Z0-9]+', '-', 'g'), '(^-|-$)', '', 'g'))
           || '-' || left(replace(id::text, '-', ''), 8)
WHERE slug IS NULL OR slug = '';

ALTER TABLE orgs ADD COLUMN IF NOT EXISTS plan text NOT NULL DEFAULT 'basic';
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS billing_status text NOT NULL DEFAULT 'active';
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'unverified';
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS public_profile_enabled boolean NOT NULL DEFAULT false;

UPDATE orgs SET verification_status = 'verified' WHERE verified = true;

ALTER TABLE orgs ALTER COLUMN slug SET NOT NULL;

DROP INDEX IF EXISTS idx_orgs_slug;
CREATE UNIQUE INDEX IF NOT EXISTS idx_orgs_slug ON orgs (slug);

ALTER TABLE orgs DROP COLUMN IF EXISTS kind;
ALTER TABLE orgs DROP COLUMN IF EXISTS verified;
ALTER TABLE orgs DROP COLUMN IF EXISTS description;
ALTER TABLE orgs DROP COLUMN IF EXISTS website;
ALTER TABLE orgs DROP COLUMN IF EXISTS enrollment_secret;

-- ── org_members rebuild ──────────────────────────────
CREATE TABLE IF NOT EXISTS org_members_new (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        text NOT NULL,
    seat_tier   text NOT NULL DEFAULT 'free',
    status      text NOT NULL DEFAULT 'active',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, user_id)
);

INSERT INTO org_members_new (org_id, user_id, role, seat_tier, status, created_at, updated_at)
SELECT org_id, user_id, role, 'free', 'active', added_at, added_at
FROM org_members;

DROP TABLE org_members;
ALTER TABLE org_members_new RENAME TO org_members;

CREATE INDEX IF NOT EXISTS idx_org_members_user ON org_members (user_id);
CREATE INDEX IF NOT EXISTS idx_org_members_org ON org_members (org_id);

-- ── org_entitlements ─────────────────────────────────
CREATE TABLE IF NOT EXISTS org_entitlements (
    id                           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                       uuid NOT NULL UNIQUE REFERENCES orgs(id) ON DELETE CASCADE,
    plan                         text NOT NULL,
    paid_seats_included          integer NOT NULL DEFAULT 0,
    managed_vpn_nodes_included   integer NOT NULL DEFAULT 0,
    shield_instances_included    integer NOT NULL DEFAULT 0,
    sentinel_licenses_included   integer NOT NULL DEFAULT 0,
    public_node_access_tier      text,
    api_quota_monthly            integer,
    bandwidth_policy             text,
    support_tier                 text,
    audit_logs_enabled           boolean NOT NULL DEFAULT false,
    advanced_analytics_enabled   boolean NOT NULL DEFAULT false,
    created_at                   timestamptz NOT NULL DEFAULT now(),
    updated_at                   timestamptz NOT NULL DEFAULT now()
);

INSERT INTO org_entitlements (
    org_id, plan, paid_seats_included, managed_vpn_nodes_included,
    shield_instances_included, sentinel_licenses_included,
    public_node_access_tier, support_tier, audit_logs_enabled, advanced_analytics_enabled
)
SELECT id, plan, 0, 0, 0, 0, 'free', 'community', false, false
FROM orgs
ON CONFLICT (org_id) DO NOTHING;