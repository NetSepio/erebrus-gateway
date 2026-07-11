-- Bind node registration tokens to an optional peer_id and cap TTL.
ALTER TABLE node_registration_tokens ADD COLUMN IF NOT EXISTS peer_id text;
CREATE INDEX IF NOT EXISTS idx_node_registration_tokens_peer ON node_registration_tokens (org_id, peer_id) WHERE peer_id IS NOT NULL;
