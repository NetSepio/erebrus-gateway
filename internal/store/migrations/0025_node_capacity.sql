-- 0025: node capacity and admission control

-- Add connected peer count to the per-minute heartbeat rollup.
ALTER TABLE node_metrics ADD COLUMN IF NOT EXISTS wg_peers_connected int NOT NULL DEFAULT 0;

-- Node admission gate thresholds. Admin-adjustable via platform_settings.
INSERT INTO platform_settings (key, value, description) VALUES
    ('node_cpu_max', '80', 'Reject new VPN clients when CPU % exceeds this value'),
    ('node_cpu_soft', '60', 'CPU soft threshold; combined with node_peer_connected_soft to reject'),
    ('node_peer_ratio_max', '0.9', 'Reject new clients when connected/registered peers ratio >= this'),
    ('node_peer_connected_soft', '80', 'Connected peer count that, combined with node_cpu_soft, rejects new clients')
ON CONFLICT (key) DO NOTHING;
