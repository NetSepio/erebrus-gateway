-- Organization membership, org-level API-call metering, and entitlement guards.

-- ── org membership ───────────────────────────────────
CREATE TABLE IF NOT EXISTS org_members (
    org_id    uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role      text NOT NULL DEFAULT 'member', -- owner | member
    added_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_org_members_user ON org_members (user_id);

-- ── org API-call usage (daily) ───────────────────────
CREATE TABLE IF NOT EXISTS api_usage_daily (
    org_id    uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    day       date NOT NULL,
    api_calls bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (org_id, day)
);

-- ── entitlement: at most one NFT-sourced subscription per user ──
CREATE UNIQUE INDEX IF NOT EXISTS idx_subs_one_nft
    ON subscriptions (user_id) WHERE source = 'nft';
