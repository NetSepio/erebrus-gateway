-- Nodes no longer advertise a public IPFS gateway base; direct browser
-- retrieval via node gateway URLs is removed. Content is gateway-proxied only.
ALTER TABLE node_drop_status
    DROP COLUMN IF EXISTS public_gateway_url;
