-- Stage 4: Sentinel firewall policy rules (gateway-managed).

CREATE TABLE IF NOT EXISTS firewall_rules (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    node_id             text NOT NULL,
    firewall_service_id uuid NOT NULL REFERENCES org_node_services(id) ON DELETE CASCADE,
    rule_type           text NOT NULL,
    target              text NOT NULL,
    action              text NOT NULL,
    scope               text NOT NULL,
    enabled             boolean NOT NULL DEFAULT true,
    created_by          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_firewall_rules_node ON firewall_rules (org_id, node_id);
CREATE INDEX IF NOT EXISTS idx_firewall_rules_service ON firewall_rules (firewall_service_id);