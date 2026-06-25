-- Platform settings: product tunables (XP, trials, rate limits, auth text).
-- Seeded with v2 defaults; admins can change via PATCH /api/v2/admin/settings.

CREATE TABLE IF NOT EXISTS platform_settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    description TEXT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO platform_settings (key, value, description) VALUES
    ('trial_period', '168h', 'General free trial length (one per user)'),
    ('nft_gate_period', '720h', 'NFT holder entitlement window (re-verify)'),
    ('nft_gate_plan_id', 'pro', 'Plan granted to NFT holders'),
    ('node_metrics_retention', '720h', 'Per-node metrics rollup retention'),
    ('xp_referrer_points', '100', 'XP for referrer when referee starts first trial'),
    ('xp_referee_points', '25', 'XP for referee on first trial start'),
    ('xp_email_verified', '25', 'XP once on email verification'),
    ('xp_nft_held', '50', 'XP per month while holding gating NFT'),
    ('xp_uptime_day', '20', 'XP per healthy owned node per UTC day (cap 5/owner)'),
    ('xp_tier_thresholds', '0,100,500,2000,10000', 'Tier cutoffs (ascending, comma-separated)'),
    ('xp_social_verified', '75', 'XP once per social provider (X/Telegram)'),
    ('xp_free_days_cost', '500', 'Claimable XP cost for free-days reward'),
    ('xp_free_days_grant', '7', 'Free entitlement days granted per rank claim'),
    ('rate_limit_auth_per_min', '30', 'Per-IP auth requests per minute (<=0 disables)'),
    ('rate_limit_register_per_min', '10', 'Per-IP node register requests per minute'),
    ('paseto_expiration', '24h', 'Wallet session token lifetime'),
    ('paseto_signed_by', 'Erebrus', 'PASETO issuer footer'),
    ('auth_eula', 'I accept the Erebrus Terms of Service https://erebrus.io/terms.', 'Wallet auth EULA prefix'),
    ('magic_link_expiration', '15m', 'Email OTP lifetime'),
    ('x_api_base_url', 'https://api.twitter.com', 'X (Twitter) API base for OAuth verify')
ON CONFLICT (key) DO NOTHING;