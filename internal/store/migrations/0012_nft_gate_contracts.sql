-- Gating NFT collections by chain + address (more rows can be added over time).

CREATE TABLE IF NOT EXISTS nft_gate_contracts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chain      TEXT NOT NULL,
    address    TEXT NOT NULL,
    label      TEXT,
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT nft_gate_contracts_chain_address_unique UNIQUE (chain, address)
);

CREATE INDEX IF NOT EXISTS idx_nft_gate_contracts_enabled ON nft_gate_contracts (enabled) WHERE enabled = true;

INSERT INTO nft_gate_contracts (chain, address, label) VALUES
    ('solana', '5XSXoWkcmynUSiwoi7XByRDiV9eomTgZQywgWrpYzKZ8', 'IslandDAO collection'),
    ('solana', '7dsnMhMyj9tFMbEgSmHwLGUSgP2fmdttTzwkGvGv3LbD', 'Erebrus Free Trial NFT')
ON CONFLICT (chain, address) DO NOTHING;