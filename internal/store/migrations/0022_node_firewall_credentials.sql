-- AdGuard Home (Shield) admin credentials for a node. The password is encrypted
-- at rest (AES-256-GCM, key derived from the gateway MNEMONIC) — the column is
-- opaque ciphertext, never plaintext. Reported by the node, revealed only to an
-- org's paid seats.
CREATE TABLE IF NOT EXISTS node_firewall_credentials (
    node_id       text PRIMARY KEY REFERENCES nodes(peer_id) ON DELETE CASCADE,
    admin_user    text NOT NULL DEFAULT 'admin',
    admin_secret  bytea NOT NULL,            -- nonce||ciphertext of the password
    admin_url     text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
