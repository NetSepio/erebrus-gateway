-- Node public IPFS gateway base for direct object retrieval (http(s), e.g.
-- https://node-host:8080 or a reverse-proxied host). Advertised additively on
-- the Drop hello capability. The Kubo RPC (5001) is never stored or exposed.
ALTER TABLE node_drop_status
    ADD COLUMN IF NOT EXISTS public_gateway_url text;
