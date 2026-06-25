-- 0005: referrals + the XP ledger foundation (PROD-STREAMLINE-PLAN §5a/§5b).
-- Referrals land here; tiers/claims/leaderboard build on xp_events in S6.
-- Additive + idempotent.

ALTER TABLE users ADD COLUMN IF NOT EXISTS referral_code text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS referred_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS xp_earned bigint NOT NULL DEFAULT 0;  -- cached sum of xp_events (lifetime)
ALTER TABLE users ADD COLUMN IF NOT EXISTS xp_claimed bigint NOT NULL DEFAULT 0; -- cached sum of claims (S6)
ALTER TABLE users ADD COLUMN IF NOT EXISTS tier int NOT NULL DEFAULT 0;          -- derived from xp_earned (S6)
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_referral_code ON users (referral_code) WHERE referral_code IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_referred_by ON users (referred_by_user_id);

-- Append-only XP ledger: lifetime earned XP drives rank/tier and never decreases.
-- meta carries context (e.g. the referee/referrer id for a referral_qualified event).
CREATE TABLE IF NOT EXISTS xp_events (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       text   NOT NULL,                 -- referral_qualified | social_verified | ... (S6)
    points     bigint NOT NULL DEFAULT 0,
    meta       jsonb  NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_xp_events_user ON xp_events (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xp_events_kind ON xp_events (kind);
