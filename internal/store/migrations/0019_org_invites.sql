-- Org email invites: pending until invitee verifies email (or logs in for wallet invites).

CREATE TABLE IF NOT EXISTS org_invites (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email       text NOT NULL,
    role        text NOT NULL,
    seat_tier   text NOT NULL DEFAULT 'free',
    invited_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    status      text NOT NULL DEFAULT 'pending',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, email)
);

CREATE INDEX IF NOT EXISTS idx_org_invites_pending_email
    ON org_invites (lower(email))
    WHERE status = 'pending';