package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// NodeMetricPoint is one downsampled time-series sample for a node.
type NodeMetricPoint struct {
	Bucket          time.Time `json:"bucket"`
	WGPeers         int       `json:"wg_peers"`
	WGPeersConnected int      `json:"wg_peers_connected"`
	ProxySessions   int       `json:"proxy_sessions"`
	RxBytes         int64     `json:"rx_bytes"`
	TxBytes         int64     `json:"tx_bytes"`
	CPUPct          float64   `json:"cpu_pct"`
	MemPct          float64   `json:"mem_pct"`
}

// RecordNodeMetrics upserts the per-minute rollup for a node from a heartbeat.
func (s *Store) RecordNodeMetrics(ctx context.Context, nodeID string, bucket time.Time,
	wgPeers, wgPeersConnected, proxySessions int, rx, tx int64, cpu, mem float64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO node_metrics (node_id, bucket, wg_peers, wg_peers_connected, proxy_sessions, rx_bytes, tx_bytes, cpu_pct, mem_pct)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (node_id, bucket) DO UPDATE SET
		   wg_peers = EXCLUDED.wg_peers, wg_peers_connected = EXCLUDED.wg_peers_connected,
		   proxy_sessions = EXCLUDED.proxy_sessions,
		   rx_bytes = EXCLUDED.rx_bytes, tx_bytes = EXCLUDED.tx_bytes,
		   cpu_pct = EXCLUDED.cpu_pct, mem_pct = EXCLUDED.mem_pct`,
		nodeID, bucket.UTC().Truncate(time.Minute), wgPeers, wgPeersConnected, proxySessions, rx, tx, cpu, mem)
	return err
}

// NodeMetrics returns a node's time series since `since`, downsampled to `step`.
func (s *Store) NodeMetrics(ctx context.Context, nodeID string, since time.Time, step time.Duration) ([]NodeMetricPoint, error) {
	stepSec := int64(step / time.Second)
	if stepSec < 60 {
		stepSec = 60
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT to_timestamp(floor(extract(epoch FROM bucket)/$3)*$3) AS b,
		        max(wg_peers), max(wg_peers_connected), max(proxy_sessions), max(rx_bytes), max(tx_bytes),
		        avg(cpu_pct), avg(mem_pct)
		 FROM node_metrics
		 WHERE node_id = $1 AND bucket >= $2
		 GROUP BY b ORDER BY b`, nodeID, since.UTC(), stepSec)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NodeMetricPoint{}
	for rows.Next() {
		var p NodeMetricPoint
		if err := rows.Scan(&p.Bucket, &p.WGPeers, &p.WGPeersConnected, &p.ProxySessions, &p.RxBytes, &p.TxBytes, &p.CPUPct, &p.MemPct); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PurgeOldNodeMetrics deletes buckets older than the retention window.
func (s *Store) PurgeOldNodeMetrics(ctx context.Context, retention time.Duration) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM node_metrics WHERE bucket < now() - $1::interval`, retention.String())
	return err
}

// OrgNodes returns nodes attached to orgs the user actively belongs to.
func (s *Store) OrgNodes(ctx context.Context, userID string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeCols+` FROM nodes
		 WHERE org_id IN (SELECT org_id FROM org_members WHERE user_id = $1 AND status = $2)
		 ORDER BY created_at DESC`, userID, MemberStatusActive)
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

// NodeOperatedBy reports whether the user is a member of the node's org.
func (s *Store) NodeOperatedBy(ctx context.Context, nodeID, userID string) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM nodes n
		   JOIN org_members m ON m.org_id = n.org_id
		   WHERE n.peer_id = $1 AND m.user_id = $2 AND m.status = $3)`, nodeID, userID, MemberStatusActive).Scan(&ok)
	return ok, err
}

// UserCanProvisionNode checks whether a user may provision on a node.
// Public nodes: any entitled user. Private nodes: org members only.
func (s *Store) UserCanProvisionNode(ctx context.Context, nodeID, userID string) (bool, error) {
	var access string
	var orgID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(access_mode,'public'), org_id::text FROM nodes WHERE peer_id=$1`, nodeID).
		Scan(&access, &orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if access != NodeAccessPrivate {
		return true, nil
	}
	if !orgID.Valid || orgID.String == "" {
		return false, nil
	}
	var member bool
	err = s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM org_members WHERE org_id=$1 AND user_id=$2 AND status=$3)`,
		orgID.String, userID, MemberStatusActive).Scan(&member)
	return member, err
}