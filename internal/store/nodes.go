package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/lib/pq"
)

// NodeRegistration carries the fields a node submits when registering.
type NodeRegistration struct {
	PeerID     string
	DID        string
	Wallet     string
	Chain      string
	OrgID      string
	Name       string
	Region     string
	Zone       string
	APIBaseURL string
	NodeKey    string
	AccessMode         string
	DeploymentProfile  string // standard | shield | sentinel
}

// RegisterNode inserts (or updates) the node row keyed by peer_id and returns
// the node id. Called from the HTTPS enrollment flow.
func (s *Store) RegisterNode(ctx context.Context, r NodeRegistration) (string, error) {
	access := r.AccessMode
	if access != NodeAccessPrivate {
		access = NodeAccessPublic
	}
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO nodes (peer_id, did, wallet_address, chain, org_id, name, region, zone, api_base_url, node_key, access_mode)
		 VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,'')::uuid, NULLIF($6,''), NULLIF($7,''), NULLIF($8,''), NULLIF($9,''), NULLIF($10,''), $11)
		 ON CONFLICT (peer_id) DO UPDATE SET
		   did = EXCLUDED.did,
		   wallet_address = COALESCE(NULLIF(EXCLUDED.wallet_address,''), nodes.wallet_address),
		   chain = COALESCE(NULLIF(EXCLUDED.chain,''), nodes.chain),
		   org_id = EXCLUDED.org_id,
		   name = COALESCE(NULLIF(EXCLUDED.name,''), nodes.name),
		   region = COALESCE(NULLIF(EXCLUDED.region,''), nodes.region),
		   zone = COALESCE(NULLIF(EXCLUDED.zone,''), nodes.zone),
		   api_base_url = COALESCE(NULLIF(EXCLUDED.api_base_url,''), nodes.api_base_url),
		   node_key = COALESCE(NULLIF(EXCLUDED.node_key,''), nodes.node_key),
		   access_mode = EXCLUDED.access_mode,
		   updated_at = now()
		 RETURNING id`,
		r.PeerID, r.DID, r.Wallet, r.Chain, r.OrgID, r.Name, r.Region, r.Zone, r.APIBaseURL, r.NodeKey, access).Scan(&id)
	return id, err
}

// ResolvePeerID verifies a node exists and returns its peer_id.
func (s *Store) ResolvePeerID(ctx context.Context, peerID string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT peer_id FROM nodes WHERE peer_id = $1`, peerID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// NodeInternalID maps a node's peer_id to its internal UUID row id.
func (s *Store) NodeInternalID(ctx context.Context, peerID string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id::text FROM nodes WHERE peer_id = $1`, peerID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// NodeKeyMatches reports whether the presented key matches the stored node_key.
func (s *Store) NodeKeyMatches(ctx context.Context, peerID, nodeKey string) (bool, error) {
	peerID = strings.TrimSpace(peerID)
	nodeKey = strings.TrimSpace(nodeKey)
	if peerID == "" || nodeKey == "" {
		return false, nil
	}
	var stored string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(node_key,'') FROM nodes WHERE peer_id = $1`, peerID).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(nodeKey)) == 1, nil
}

// NodeAPI returns the gateway-reachable API base URL, node key and status
// for a node, used when proxying provisioning calls.
func (s *Store) NodeAPI(ctx context.Context, ref string) (baseURL, nodeKey, status string, err error) {
	var ip string
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(api_base_url,''), COALESCE(node_key,''), status, COALESCE(ip,'')
		 FROM nodes WHERE peer_id = $1`, ref).
		Scan(&baseURL, &nodeKey, &status, &ip)
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
	Zone         string
	AccessMode   string // public | private (empty keeps the prior value)
	Spec         []byte // json
	Capabilities []byte // json
	Endpoints    []byte // json
	Protocols    []string
}

// ApplyHello records a node's hello: connection endpoints, spec, capabilities,
// access mode, and marks it online.
func (s *Store) ApplyHello(ctx context.Context, h HelloUpdate) error {
	access := h.AccessMode
	if access != "" && access != NodeAccessPublic && access != NodeAccessPrivate {
		access = ""
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET
		   ip = NULLIF($2,''), ip_hash = NULLIF($3,''), version = NULLIF($4,''),
		   region = COALESCE(NULLIF($5,''), region),
		   zone = COALESCE(NULLIF($6,''), zone),
		   access_mode = COALESCE(NULLIF($7,''), access_mode),
		   spec = $8::jsonb, capabilities = $9::jsonb, endpoints = $10::jsonb,
		   protocols = $11, status = 'online', last_heartbeat = now(), updated_at = now(),
		   api_base_url = CASE
		     WHEN COALESCE(api_base_url,'') = '' AND NULLIF($2,'') IS NOT NULL
		     THEN 'http://' || $2 || ':9080'
		     ELSE api_base_url
		   END
		 WHERE peer_id = $1`,
		h.PeerID, h.IP, h.IPHash, h.Version, h.Region, h.Zone, access,
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

// SetNodeMinTier sets a node's premium-pool gate (admin). Returns ErrNotFound
// when the node does not exist.
func (s *Store) SetNodeMinTier(ctx context.Context, ref string, minTier int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET min_tier = $2, updated_at = now() WHERE peer_id = $1`, ref, minTier)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetNodeOrg updates a node's org attachment (operator/admin).
func (s *Store) SetNodeOrg(ctx context.Context, ref, orgID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET org_id = NULLIF($2,'')::uuid, updated_at = now() WHERE peer_id = $1`,
		ref, orgID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetNodeAccessMode sets public/private visibility.
func (s *Store) SetNodeAccessMode(ctx context.Context, ref, mode string) error {
	if mode != NodeAccessPublic && mode != NodeAccessPrivate {
		return errors.New("invalid access_mode")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET access_mode = $2, updated_at = now() WHERE peer_id = $1`, ref, mode)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetNodeStatus forces a node's status (e.g. offline on disconnect/timeout).
func (s *Store) SetNodeStatus(ctx context.Context, peerID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET status = $2, updated_at = now() WHERE peer_id = $1`, peerID, status)
	if err != nil {
		return err
	}
	_ = s.SyncOrgNodeStatusFromRuntime(ctx, peerID, status)
	return nil
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
	_ = s.SyncAllOrgNodeStatusesFromRuntime(ctx)
	return res.RowsAffected()
}

const nodeCols = `id, peer_id, did, COALESCE(wallet_address,''), COALESCE(chain,''),
	COALESCE(org_id::text,''), COALESCE(access_mode,'public'),
	COALESCE(min_tier,0), COALESCE(name,''), COALESCE(region,''), COALESCE(zone,''), COALESCE(ip,''), COALESCE(ip_hash,''),
	spec, capabilities, endpoints, protocols, status, load, speedtest, rx_bytes, tx_bytes,
	COALESCE(version,''), COALESCE(deployment_profile,'standard'), last_heartbeat, last_peer_handshake, created_at`

func scanNode(sc interface{ Scan(...any) error }) (*Node, error) {
	var n Node
	if err := sc.Scan(&n.ID, &n.PeerID, &n.DID, &n.WalletAddress, &n.Chain,
		&n.OrgID, &n.AccessMode, &n.MinTier, &n.Name, &n.Region, &n.Zone,
		&n.IP, &n.IPHash, &n.Spec, &n.Capabilities, &n.Endpoints, pq.Array(&n.Protocols),
		&n.Status, &n.Load, &n.Speedtest, &n.RxBytes, &n.TxBytes, &n.Version, &n.DeploymentProfile,
		&n.LastHeartbeat, &n.LastPeerHandshake, &n.CreatedAt); err != nil {
		return nil, err
	}
	return &n, nil
}

// GetNodePeerID resolves a node reference to peer_id.
func (s *Store) GetNodePeerID(ctx context.Context, ref string) (string, error) {
	return s.ResolvePeerID(ctx, ref)
}

// ApplyNodeHeartbeat records a REST heartbeat for a runtime node.
func (s *Store) ApplyNodeHeartbeat(ctx context.Context, peerID, status string, load, speedtest []byte, rx, tx int64, version string) error {
	if err := s.ApplyHeartbeat(ctx, peerID, status, load, speedtest, rx, tx, version); err != nil {
		return err
	}
	now := time.Now()
	_ = s.TouchOrgNodeHeartbeat(ctx, peerID, now)
	return nil
}

// GetNode returns a node by peer_id.
func (s *Store) GetNode(ctx context.Context, peerID string) (*Node, error) {
	n, err := scanNode(s.db.QueryRowContext(ctx, `SELECT `+nodeCols+` FROM nodes WHERE peer_id = $1`, peerID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return n, err
}

// ListNodes returns nodes filtered by status (and optionally region/zone) for admin views.
func (s *Store) ListNodes(ctx context.Context, status, region, zone string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeCols+` FROM nodes
		 WHERE ($1 = '' OR status = $1) AND ($2 = '' OR region = $2) AND ($3 = '' OR zone = $3)
		 ORDER BY region, zone, name`, status, region, zone)
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

// ListDiscoverableNodes is the PUBLIC directory: public nodes only, tier-gated.
func (s *Store) ListDiscoverableNodes(ctx context.Context, status, region, zone string, callerTier int) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeCols+` FROM nodes
		 WHERE access_mode = 'public' AND min_tier <= $4
		   AND ($1 = '' OR status = $1) AND ($2 = '' OR region = $2) AND ($3 = '' OR zone = $3)
		 ORDER BY region, zone, name`, status, region, zone, callerTier)
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

// NodesByOrg returns all nodes attached to an org.
func (s *Store) NodesByOrg(ctx context.Context, orgID string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeCols+` FROM nodes WHERE org_id = $1::uuid ORDER BY created_at DESC`, orgID)
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