-- 0006: XP earn-vs-claim ledger + idempotent XP awards (PROD-STREAMLINE-PLAN §5b).
-- Additive + idempotent. xp_events, the users xp cols, and AwardXP already exist
-- from 0005; this adds claim accounting + a dedup key for once/per-period awards.

-- dedup_key makes "once"/"per-day"/"per-month" XP awards idempotent
-- (e.g. email_verified:<uid>, nft_held:<uid>:<yyyymm>, uptime:<node>:<yyyymmdd>).
ALTER TABLE xp_events ADD COLUMN IF NOT EXISTS dedup_key text;
CREATE UNIQUE INDEX IF NOT EXISTS idx_xp_events_dedup ON xp_events (dedup_key) WHERE dedup_key IS NOT NULL;

-- Claim ledger: lifetime earned XP (xp_events) never decreases and drives tier;
-- rewards are claimed from the claimable balance (earned - claimed), each logged
-- here with IP + device for the activity/audit trail (§6.5).
CREATE TABLE IF NOT EXISTS xp_claims (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid   NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       text   NOT NULL,                 -- e.g. free_days
    xp_spent   bigint NOT NULL DEFAULT 0,
    reward     text   NOT NULL,                 -- human-readable granted reward
    ip         text,
    device     text,
    meta       jsonb  NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_xp_claims_user ON xp_claims (user_id, created_at DESC);

-- Backfill users.tier from the default thresholds (runtime recomputes on the
-- next award if XP_TIER_THRESHOLDS is customized).
UPDATE users SET tier = CASE
    WHEN xp_earned >= 10000 THEN 4
    WHEN xp_earned >= 2000  THEN 3
    WHEN xp_earned >= 500   THEN 2
    WHEN xp_earned >= 100   THEN 1
    ELSE 0 END
WHERE tier = 0;
