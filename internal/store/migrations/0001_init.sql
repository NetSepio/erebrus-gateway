-- Erebrus gateway v2 initial schema.
-- UUID PKs are generated in Go (google/uuid); gen_random_uuid() (PG13+) is used
-- as a defensive default. Money is numeric(18,6) USDC, scanned as string in Go.

-- ── users ───────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_address text UNIQUE,
    chain          text,                       -- evm | sol | apt | sui
    role           text NOT NULL DEFAULT 'user', -- user | admin
    email          text UNIQUE,
    name           text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

-- ── auth challenges (wallet-signature login) ─────────
CREATE TABLE IF NOT EXISTS flow_ids (
    flow_id        text PRIMARY KEY,
    wallet_address text NOT NULL,
    chain          text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_flow_ids_expires ON flow_ids (expires_at);

-- ── nodes ────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS nodes (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    peer_id        text UNIQUE NOT NULL,
    did            text NOT NULL,
    wallet_address text,
    name           text,
    region         text,
    ip             text,                  -- raw IP: operational use only, never published
    ip_hash        text,                  -- sha3-256 hex, the only publishable IP form
    spec           jsonb NOT NULL DEFAULT '{}',
    capabilities   jsonb NOT NULL DEFAULT '{}',
    endpoints      jsonb NOT NULL DEFAULT '{}',
    protocols      text[] NOT NULL DEFAULT '{}',
    api_base_url   text,                  -- gateway-reachable node API base, e.g. https://1.2.3.4:9080
    api_token      text,                  -- bearer for gateway→node /api/v2 calls (provided at registration)
    status         text NOT NULL DEFAULT 'offline', -- online | offline | draining
    load           jsonb NOT NULL DEFAULT '{}',
    speedtest      jsonb NOT NULL DEFAULT '{}',
    rx_bytes       bigint NOT NULL DEFAULT 0, -- cumulative interface counters
    tx_bytes       bigint NOT NULL DEFAULT 0,
    version        text,
    last_heartbeat timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_nodes_status_region ON nodes (status, region);

-- ── plans ────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS plans (
    id          text PRIMARY KEY,            -- e.g. 'free', 'pro', 'team'
    name        text NOT NULL,
    price_usdc  numeric(18,6) NOT NULL DEFAULT 0,
    period_days int NOT NULL DEFAULT 30,
    max_clients int NOT NULL DEFAULT 1,
    is_active   boolean NOT NULL DEFAULT true,
    sort_order  int NOT NULL DEFAULT 0
);

-- ── organizations & API keys ─────────────────────────
CREATE TABLE IF NOT EXISTS orgs (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name        text,
    prefix      text NOT NULL,           -- non-secret display prefix
    key_hash    text NOT NULL,           -- sha256 of the full key
    created_at  timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at  timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys (key_hash);

-- ── subscriptions ────────────────────────────────────
CREATE TABLE IF NOT EXISTS subscriptions (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            uuid REFERENCES users(id) ON DELETE CASCADE,
    org_id             uuid REFERENCES orgs(id) ON DELETE CASCADE,
    plan_id            text NOT NULL REFERENCES plans(id),
    source             text NOT NULL,        -- crypto | nft | trial | admin
    status             text NOT NULL,        -- active | expired | canceled
    current_period_end timestamptz,
    payment_id         uuid,
    created_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT subs_owner CHECK (user_id IS NOT NULL OR org_id IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_subs_user ON subscriptions (user_id, status);
-- one trial per user, enforced at the DB
CREATE UNIQUE INDEX IF NOT EXISTS idx_subs_one_trial
    ON subscriptions (user_id) WHERE source = 'trial';

-- ── VPN clients ──────────────────────────────────────
CREATE TABLE IF NOT EXISTS vpn_clients (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id         uuid REFERENCES orgs(id) ON DELETE SET NULL,
    node_id        uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    name           text NOT NULL,
    wg_public_key  text NOT NULL,
    wg_allowed_ip  text,
    status         text NOT NULL DEFAULT 'pending', -- pending | active | deleting
    rx_bytes       bigint NOT NULL DEFAULT 0,        -- cumulative metered traffic
    tx_bytes       bigint NOT NULL DEFAULT 0,
    last_handshake timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_vpn_clients_user ON vpn_clients (user_id);
CREATE INDEX IF NOT EXISTS idx_vpn_clients_node ON vpn_clients (node_id);

-- ── usage rollups (daily, per client) ────────────────
CREATE TABLE IF NOT EXISTS usage_daily (
    client_id uuid NOT NULL REFERENCES vpn_clients(id) ON DELETE CASCADE,
    day       date NOT NULL,
    rx_bytes  bigint NOT NULL DEFAULT 0,
    tx_bytes  bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (client_id, day)
);

-- ── crypto payments (USDC Solana + Base) ─────────────
CREATE TABLE IF NOT EXISTS crypto_payments (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid REFERENCES users(id) ON DELETE SET NULL,
    org_id           uuid REFERENCES orgs(id) ON DELETE SET NULL,
    plan_id          text NOT NULL REFERENCES plans(id),
    chain            text NOT NULL CHECK (chain IN ('solana','base')),
    expected_amount  numeric(18,6) NOT NULL,
    token            text NOT NULL,
    treasury_address text NOT NULL,
    reference        text,
    tx_hash          text UNIQUE,
    payer_address    text,
    status           text NOT NULL CHECK (status IN
                       ('pending','awaiting_finality','confirmed','mismatched','expired')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    confirmed_at     timestamptz
);
CREATE INDEX IF NOT EXISTS idx_payments_status ON crypto_payments (status);

-- ── seed plans ───────────────────────────────────────
INSERT INTO plans (id, name, price_usdc, period_days, max_clients, sort_order) VALUES
    ('free', 'Free',  0,     30,  1,  0),
    ('pro',  'Pro',   9.99,  30,  5,  1),
    ('team', 'Team',  49.99, 30,  25, 2)
ON CONFLICT (id) DO NOTHING;
