-- Latest WireGuard handshake across any client on this node (from usage_report).
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS last_peer_handshake timestamptz;

UPDATE nodes n
   SET last_peer_handshake = sub.max_hs
  FROM (
        SELECT node_id, MAX(last_handshake) AS max_hs
          FROM vpn_clients
         WHERE last_handshake IS NOT NULL
         GROUP BY node_id
       ) sub
 WHERE n.id = sub.node_id;