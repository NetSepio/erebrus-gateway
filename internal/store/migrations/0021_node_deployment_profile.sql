-- Node type (Standard=erebrus / Shield / Sentinel) on the runtime node row, so
-- operator/discovery views can expose it for client-side filtering. Mirrors
-- org_nodes.deployment_profile written at enrollment.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS deployment_profile text NOT NULL DEFAULT 'erebrus';
