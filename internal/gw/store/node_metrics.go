package store

import (
	"context"
	"time"
)

// NodeMetricPoint is one downsampled time-series sample for a node.
type NodeMetricPoint struct {
	Bucket        time.Time `json:"bucket"`
	WGPeers       int       `json:"wg_peers"`
	ProxySessions int       `json:"proxy_sessions"`
	RxBytes       int64     `json:"rx_bytes"`
	TxBytes       int64     `json:"tx_bytes"`
	CPUPct        float64   `json:"cpu_pct"`
	MemPct        float64   `json:"mem_pct"`
}

// RecordNodeMetrics upserts the per-minute rollup for a node from a heartbeat.
// Last write in a bucket wins (heartbeats arrive ~2x/minute).
func (s *Store) RecordNodeMetrics(ctx context.Context, nodeID string, bucket time.Time,
	wgPeers, proxySessions int, rx, tx int64, cpu, mem float64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO node_metrics (node_id, bucket, wg_peers, proxy_sessions, rx_bytes, tx_bytes, cpu_pct, mem_pct)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (node_id, bucket) DO UPDATE SET
		   wg_peers = EXCLUDED.wg_peers, proxy_sessions = EXCLUDED.proxy_sessions,
		   rx_bytes = EXCLUDED.rx_bytes, tx_bytes = EXCLUDED.tx_bytes,
		   cpu_pct = EXCLUDED.cpu_pct, mem_pct = EXCLUDED.mem_pct`,
		nodeID, bucket.UTC().Truncate(time.Minute), wgPeers, proxySessions, rx, tx, cpu, mem)
	return err
}

// NodeMetrics returns a node's time series since `since`, downsampled to `step`
// buckets. Gauges/counters use max() within a step; cpu/mem use avg(). The
// epoch-floor bucketing works on any supported Postgres (no date_bin needed).
func (s *Store) NodeMetrics(ctx context.Context, nodeID string, since time.Time, step time.Duration) ([]NodeMetricPoint, error) {
	stepSec := int64(step / time.Second)
	if stepSec < 60 {
		stepSec = 60
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT to_timestamp(floor(extract(epoch FROM bucket)/$3)*$3) AS b,
		        max(wg_peers), max(proxy_sessions), max(rx_bytes), max(tx_bytes),
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
		if err := rows.Scan(&p.Bucket, &p.WGPeers, &p.ProxySessions, &p.RxBytes, &p.TxBytes, &p.CPUPct, &p.MemPct); err != nil {
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

// OwnedNodes returns the nodes a user operates: those they own directly, plus
// nodes attached to an org they belong to.
func (s *Store) OwnedNodes(ctx context.Context, userID string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeCols+` FROM nodes
		 WHERE owner_user_id = $1
		    OR org_id IN (SELECT org_id FROM org_members WHERE user_id = $1)
		 ORDER BY created_at DESC`, userID)
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

// NodeOperatedBy reports whether the user owns the node or shares its org — the
// authorization check for operator metrics.
func (s *Store) NodeOperatedBy(ctx context.Context, nodeID, userID string) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM nodes
		   WHERE id = $1 AND (
		     owner_user_id = $2
		     OR org_id IN (SELECT org_id FROM org_members WHERE user_id = $2)
		   ))`, nodeID, userID).Scan(&ok)
	return ok, err
}
