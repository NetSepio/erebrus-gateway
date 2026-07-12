-- User account deletion requests.

CREATE TABLE IF NOT EXISTS deletion_requests (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid REFERENCES users(id) ON DELETE SET NULL,
    wallet_address text,
    email         text,
    name          text,
    status        text NOT NULL DEFAULT 'pending', -- pending | fulfilled
    requested_at  timestamptz NOT NULL DEFAULT now(),
    fulfilled_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_deletion_requests_status ON deletion_requests (status);
CREATE INDEX IF NOT EXISTS idx_deletion_requests_user_id ON deletion_requests (user_id);
