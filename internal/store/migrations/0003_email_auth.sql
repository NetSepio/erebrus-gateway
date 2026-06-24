-- 0003: optional email linking to a wallet account (Resend OTP).
-- Email is NEVER required to use the VPN; it links to the wallet account for
-- perks/ranking + recovery (PROD-STREAMLINE-PLAN §2). users.email already exists
-- and is UNIQUE (0001); we add only the verified flag + the pending-code table.

-- Verified flag on the user (email column already exists from 0001).
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified boolean NOT NULL DEFAULT false;

-- Pending email-verification one-time codes. The code is stored hashed
-- (sha256 hex), never in cleartext; rows are short-lived (MAGIC_LINK_EXPIRATION,
-- default 15m) and deleted on success. attempts caps brute force at the API layer.
CREATE TABLE IF NOT EXISTS email_otps (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email      text NOT NULL,                 -- stored lowercased
    code_hash  text NOT NULL,                 -- sha256(code) hex
    attempts   int  NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_email_otps_user_email ON email_otps (user_id, email);
CREATE INDEX IF NOT EXISTS idx_email_otps_expires ON email_otps (expires_at);
