-- Userless one-time codes for EMAIL LOGIN (distinct from email_otps, which links
-- an email to an already-authenticated account). The user is created/resolved
-- only after the code is verified, so an unverified email never squats a row in
-- users. One pending code per email (PK email); re-requests upsert.
CREATE TABLE IF NOT EXISTS email_login_otps (
    email       text PRIMARY KEY,
    code_hash   text NOT NULL,
    attempts    int NOT NULL DEFAULT 0,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_login_otps_expires ON email_login_otps (expires_at);
