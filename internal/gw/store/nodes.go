package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

// NodeRegistration carries the fields a node submits when registering.
type NodeRegistration struct {
	PeerID     string
	DID        string
	Wallet     string
	Name       string
	Region     string
	APIBaseURL string // gateway-reachable node API base
	APIToken   string // bearer for gateway→node calls
}

// RegisterNode inserts (or updates) the node row keyed by peer_id and returns
// the node id. Called from the HTTPS registration flow.
func (s *Store) RegisterNode(ctx context.Context, r NodeRegistration) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO nodes (peer_id, did, wallet_address, name, region, api_base_url, api_token)
		 VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''))
		 ON CONFLICT (peer_id) DO UPDATE SET
		   did = EXCLUDED.did,
		   wallet_address = COALESCE(EXCLUDED.wallet_address, nodes.wallet_address),
		   name = COALESCE(EXCLUDED.name, nodes.name),
		   region = COALESCE(EXCLUDED.region, nodes.region),
		   api_base_url = COALESCE(EXCLUDED.api_base_url, nodes.api_base_url),
		   api_token = COALESCE(EXCLUDED.api_token, nodes.api_token),
		   updated_at = now()
		 RETURNING id`,
		r.PeerID, r.DID, r.Wallet, r.Name, r.Region, r.APIBaseURL, r.APIToken).Scan(&id)
	return id, err
}

// NodeAPI returns the gateway-reachable API base URL, bearer token and status
// for a node, used when proxying provisioning calls. When api_base_url was never
// set at registration, it is derived from the node's last-reported IP.
func (s *Store) NodeAPI(ctx context.Context, nodeID string) (baseURL, token, status string, err error) {
	var ip string
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(api_base_url,''), COALESCE(api_token,''), status, COALESCE(ip,'')
		 FROM nodes WHERE id = $1`, nodeID).
		Scan(&baseURL, &token, &status, &ip)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", err
	}
	if baseURL == "" && ip != "" {
		baseURL = "http://" + ip + ":9080"
	}
	return
}

// HelloUpdate carries the dynamic fields a node sends in its hello frame.
type HelloUpdate struct {
	PeerID       string
	IP           string
	IPHash       string
	Version      string
	Region       string
	Spec         []byte // json
	Capabilities []byte // json
	Endpoints    []byte // json
	Protocols    []string
}

// ApplyHello records a node's hello: connection endpoints, spec, capabilities,
// and marks it online.
func (s *Store) ApplyHello(ctx context.Context, h HelloUpdate) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET
		   ip = NULLIF($2,''), ip_hash = NULLIF($3,''), version = NULLIF($4,''),
		   region = COALESCE(NULLIF($5,''), region),
		   spec = $6::jsonb, capabilities = $7::jsonb, endpoints = $8::jsonb,
		   protocols = $9, status = 'online', last_heartbeat = now(), updated_at = now(),
		   api_base_url = CASE
		     WHEN COALESCE(api_base_url,'') = '' AND NULLIF($2,'') IS NOT NULL
		     THEN 'http://' || $2 || ':9080'
		     ELSE api_base_url
		   END
		 WHERE peer_id = $1`,
		h.PeerID, h.IP, h.IPHash, h.Version, h.Region,
		string(h.Spec), string(h.Capabilities), string(h.Endpoints), pq.Array(h.Protocols))
	return err
}

// ApplyHeartbeat records a heartbeat snapshot and bumps last_heartbeat.
func (s *Store) ApplyHeartbeat(ctx context.Context, peerID, status string, load, speedtest []byte, rx, tx int64, version string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET
		   status = $2, load = $3::jsonb, speedtest = $4::jsonb,
		   rx_bytes = $5, tx_bytes = $6, version = COALESCE(NULLIF($7,''), version),
		   last_heartbeat = now(), updated_at = now()
		 WHERE peer_id = $1`,
		peerID, status, string(load), string(speedtest), rx, tx, version)
	return err
}

// SetNodeStatus forces a node's status (e.g. offline on disconnect/timeout).
func (s *Store) SetNodeStatus(ctx context.Context, peerID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET status = $2, updated_at = now() WHERE peer_id = $1`, peerID, status)
	return err
}

// MarkStaleNodesOffline flips nodes with no heartbeat within the window to
// offline. Returns the number flipped.
func (s *Store) MarkStaleNodesOffline(ctx context.Context, within time.Duration) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET status = 'offline', updated_at = now()
		 WHERE status <> 'offline' AND (last_heartbeat IS NULL OR last_heartbeat < now() - $1::interval)`,
		within.String())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

const nodeCols = `id, peer_id, did, COALESCE(wallet_address,''), COALESCE(name,''),
	COALESCE(region,''), COALESCE(ip,''), COALESCE(ip_hash,''), spec, capabilities,
	endpoints, protocols, status, load, speedtest, rx_bytes, tx_bytes,
	COALESCE(version,''), last_heartbeat, created_at`

func scanNode(sc interface{ Scan(...any) error }) (*Node, error) {
	var n Node
	if err := sc.Scan(&n.ID, &n.PeerID, &n.DID, &n.WalletAddress, &n.Name, &n.Region,
		&n.IP, &n.IPHash, &n.Spec, &n.Capabilities, &n.Endpoints, pq.Array(&n.Protocols),
		&n.Status, &n.Load, &n.Speedtest, &n.RxBytes, &n.TxBytes, &n.Version,
		&n.LastHeartbeat, &n.CreatedAt); err != nil {
		return nil, err
	}
	return &n, nil
}

// GetNode returns a node by id.
func (s *Store) GetNode(ctx context.Context, id string) (*Node, error) {
	n, err := scanNode(s.db.QueryRowContext(ctx, `SELECT `+nodeCols+` FROM nodes WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return n, err
}

// ListNodes returns nodes filtered by status (and optionally region) for the
// public discovery endpoint and admin views.
func (s *Store) ListNodes(ctx context.Context, status, region string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeCols+` FROM nodes
		 WHERE ($1 = '' OR status = $1) AND ($2 = '' OR region = $2)
		 ORDER BY region, name`, status, region)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CountNodesByStatus returns counts grouped by status (admin stats).
func (s *Store) CountNodesByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, count(*) FROM nodes GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}
