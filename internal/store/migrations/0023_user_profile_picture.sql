-- 0023: user avatar — bare IPFS CID (the webapp uploads to IPFS and PATCHes
-- only the hash; the gateway never stores image bytes). Additive + idempotent.

ALTER TABLE users ADD COLUMN IF NOT EXISTS profile_picture text;
