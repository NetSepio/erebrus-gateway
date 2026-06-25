-- 0008: account activity / audit log (§6.5). Every meaningful authenticated
-- mutation (login, email/social verify, VPN provision/delete, org + API-key
-- changes, subscription/trial/NFT, rank claims, admin actions, profile edits) is
-- recorded with IP + device so a user can spot anything they didn't do. Never
-- traffic content — this is account activity, not browsing.
CREATE TABLE IF NOT EXISTS activity_log (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid REFERENCES users(id) ON DELETE CASCADE,
    action     text NOT NULL,                 -- e.g. vpn.client.provision
    target     text,                          -- referenced id, never a secret value
    ip         text,
    user_agent text,
    device     text,
    app        text,                          -- web | ios | android | desktop
    meta       jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_activity_user ON activity_log (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_created ON activity_log (created_at DESC);
