-- Stage 2: Org nodes, services, registration tokens, and Sentinel licenses.

CREATE TABLE IF NOT EXISTS org_nodes (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    node_id             text NOT NULL UNIQUE,
    node_name           text,
    deployment_profile  text NOT NULL,
    node_type           text NOT NULL,
    visibility          text NOT NULL,
    managed_by          text NOT NULL,
    region              text,
    zone                text,
    status              text NOT NULL,
    api_public_url      text,
    last_seen_at        timestamptz,
    created_by          uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_org_nodes_org ON org_nodes (org_id);

CREATE TABLE IF NOT EXISTS org_node_services (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    node_id           text NOT NULL REFERENCES org_nodes(node_id) ON DELETE CASCADE,
    service_type      text NOT NULL,
    service_name      text NOT NULL,
    service_provider  text NOT NULL,
    service_status    text NOT NULL,
    visibility        text NOT NULL,
    config_ref        text,
    access_url        text,
    license_id        uuid,
    created_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (node_id, service_type)
);
CREATE INDEX IF NOT EXISTS idx_org_node_services_org ON org_node_services (org_id);

CREATE TABLE IF NOT EXISTS node_registration_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    token_hash  text NOT NULL,
    scopes      text[] NOT NULL,
    expires_at  timestamptz NOT NULL,
    created_by  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    used_at     timestamptz,
    revoked_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_node_registration_tokens_hash
    ON node_registration_tokens (token_hash);

CREATE TABLE IF NOT EXISTS sentinel_licenses (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    node_id     text REFERENCES org_nodes(node_id) ON DELETE SET NULL,
    status      text NOT NULL,
    source      text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sentinel_licenses_org ON sentinel_licenses (org_id);

-- Backfill org_nodes from existing runtime nodes attached to orgs.
INSERT INTO org_nodes (
    org_id, node_id, node_name, deployment_profile, node_type, visibility,
    managed_by, region, zone, status, api_public_url, last_seen_at
)
SELECT
    n.org_id,
    n.peer_id,
    n.name,
    'standard',
    CASE WHEN COALESCE(n.access_mode, 'public') = 'private' THEN 'private' ELSE 'public' END,
    CASE WHEN COALESCE(n.access_mode, 'public') = 'private' THEN 'private_org' ELSE 'public_network' END,
    'org',
    n.region,
    n.zone,
    CASE n.status WHEN 'online' THEN 'active' WHEN 'draining' THEN 'degraded' ELSE 'disabled' END,
    n.api_base_url,
    n.last_heartbeat
FROM nodes n
WHERE n.org_id IS NOT NULL
ON CONFLICT (node_id) DO NOTHING;

-- Default VPN service for backfilled nodes.
INSERT INTO org_node_services (
    org_id, node_id, service_type, service_name, service_provider,
    service_status, visibility
)
SELECT org_id, node_id, 'vpn', 'erebrus', 'wireguard', 'active', 'org_only'
FROM org_nodes
ON CONFLICT (node_id, service_type) DO NOTHING;