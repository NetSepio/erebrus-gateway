package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// NormalizeDropState maps a node-reported Drop state onto a known value,
// defaulting unknown/empty input to disabled.
func NormalizeDropState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case DropStateStarting:
		return DropStateStarting
	case DropStateActive:
		return DropStateActive
	case DropStateDegraded:
		return DropStateDegraded
	case DropStateFull:
		return DropStateFull
	case DropStateUnreachable:
		return DropStateUnreachable
	default:
		return DropStateDisabled
	}
}

// ApplyDropCapability records the Drop capability a node advertised in its hello
// frame. Capacity/health fields are left untouched (they come from heartbeats).
func (s *Store) ApplyDropCapability(ctx context.Context, nodeID string, enabled, acceptsPublic, webui bool) error {
	initialState := DropStateDisabled
	if enabled {
		initialState = DropStateStarting
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO node_drop_status (node_id, enabled, accepts_public_uploads, webui_available, state, last_reported_at)
		 VALUES ($1,$2,$3,$4,$5, now())
		 ON CONFLICT (node_id) DO UPDATE SET
		     enabled = EXCLUDED.enabled,
		     accepts_public_uploads = EXCLUDED.accepts_public_uploads,
		     webui_available = EXCLUDED.webui_available,
		     state = CASE WHEN EXCLUDED.enabled = false THEN $5 ELSE node_drop_status.state END,
		     last_reported_at = now(), updated_at = now()`,
		nodeID, enabled, acceptsPublic, webui, initialState)
	return err
}

// ApplyDropStatus records the Drop runtime health/capacity a node reported in a
// heartbeat. Capability flags (accepts_public_uploads, webui_available) are left
// untouched (they come from hello). A heartbeat carrying Drop implies enabled.
func (s *Store) ApplyDropStatus(ctx context.Context, nodeID, state, kuboVersion string, repoSize, storageMax, numObjects int64) error {
	state = NormalizeDropState(state)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO node_drop_status (node_id, enabled, state, kubo_version, repo_size_bytes, storage_max_bytes, num_objects, last_reported_at)
		 VALUES ($1, true, $2, NULLIF($3,''), $4, $5, $6, now())
		 ON CONFLICT (node_id) DO UPDATE SET
		     enabled = true,
		     state = EXCLUDED.state,
		     kubo_version = EXCLUDED.kubo_version,
		     repo_size_bytes = EXCLUDED.repo_size_bytes,
		     storage_max_bytes = EXCLUDED.storage_max_bytes,
		     num_objects = EXCLUDED.num_objects,
		     last_reported_at = now(), updated_at = now()`,
		nodeID, state, kuboVersion, repoSize, storageMax, numObjects)
	return err
}

const nodeDropStatusCols = `node_id, enabled, accepts_public_uploads, webui_available, state,
	COALESCE(kubo_version,''), repo_size_bytes, storage_max_bytes, num_objects, reserved_bytes,
	last_reported_at`

func scanNodeDropStatus(sc interface{ Scan(...any) error }) (*NodeDropStatus, error) {
	n := &NodeDropStatus{}
	if err := sc.Scan(&n.NodeID, &n.Enabled, &n.AcceptsPublicUploads, &n.WebUIAvailable, &n.State,
		&n.KuboVersion, &n.RepoSizeBytes, &n.StorageMaxBytes, &n.NumObjects, &n.ReservedBytes,
		&n.LastReportedAt); err != nil {
		return nil, err
	}
	return n, nil
}

// DropPinnedNodes returns the ids of healthy nodes that currently hold cid
// pinned, preferred node first. Used to source retrieval (node→gateway→caller)
// from any node holding the object, so a single node going down does not break
// reads.
func (s *Store) DropPinnedNodes(ctx context.Context, cid, preferNodeID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT d.node_id
		   FROM drop_pins p
		   JOIN node_drop_status d ON d.node_id = p.node_id
		  WHERE p.cid = $1 AND p.status = 'pinned'
		    AND d.enabled = true AND d.state IN ('active','degraded')
		  ORDER BY (d.node_id = $2) DESC, d.node_id`,
		cid, preferNodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GetNodeDropStatus returns the latest Drop status for a node, or ErrNotFound.
func (s *Store) GetNodeDropStatus(ctx context.Context, nodeID string) (*NodeDropStatus, error) {
	n, err := scanNodeDropStatus(s.db.QueryRowContext(ctx,
		`SELECT `+nodeDropStatusCols+` FROM node_drop_status WHERE node_id = $1`, nodeID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return n, err
}

// Coarse capacity buckets exposed to callers (never exact bytes).
const (
	DropCapacityUnknown   = "unknown"
	DropCapacityAvailable = "available"
	DropCapacityLimited   = "limited"
	DropCapacityFull      = "full"
)

// DropNodeInfo is the safe, coarse view of a Drop-capable node returned by
// discovery. It intentionally omits Kubo RPC URLs, node credentials, and exact
// capacity — only eligibility and a coarse capacity bucket are exposed.
type DropNodeInfo struct {
	NodeID               string `json:"node_id"` // peer id
	OrgID                string `json:"org_id,omitempty"`
	Name                 string `json:"name"`
	Region               string `json:"region"`
	AccessMode           string `json:"access_mode"`
	DeploymentProfile    string `json:"deployment_profile"`
	Online               bool   `json:"online"`
	AcceptingUploads     bool   `json:"accepting_uploads"`
	State                string `json:"state"`
	AcceptsPublicUploads bool   `json:"accepts_public_uploads"`
	WebUIAvailable       bool   `json:"webui_available"`
	Capacity             string `json:"capacity"` // coarse bucket
}

// coarseCapacity buckets remaining capacity without revealing exact bytes.
func coarseCapacity(storageMax, repoSize, reserved int64) string {
	if storageMax <= 0 {
		return DropCapacityUnknown
	}
	avail := storageMax - repoSize - reserved
	if avail <= 0 {
		return DropCapacityFull
	}
	if avail*10 < storageMax { // < 10% remaining
		return DropCapacityLimited
	}
	return DropCapacityAvailable
}

// ListDropNodes returns Drop-capable nodes for discovery. scope "public" returns
// online public nodes accepting public uploads; scope "private_org" returns the
// given org's nodes with Drop enabled. Only coarse, non-sensitive fields are
// returned.
func (s *Store) ListDropNodes(ctx context.Context, scope, orgID string) ([]*DropNodeInfo, error) {
	var q string
	var args []any
	base := `SELECT n.peer_id, COALESCE(n.org_id::text,''), COALESCE(n.name,''), COALESCE(n.region,''),
	        COALESCE(n.access_mode,'public'), COALESCE(n.deployment_profile,'standard'),
	        n.status = 'online', d.state, d.accepts_public_uploads, d.webui_available,
	        d.storage_max_bytes, d.repo_size_bytes, d.reserved_bytes
	 FROM node_drop_status d JOIN nodes n ON n.peer_id = d.node_id
	 WHERE d.enabled = true AND d.state IN ('active','degraded','full') `
	switch scope {
	case DropScopePrivateOrg:
		q = base + `AND n.org_id = $1::uuid ORDER BY n.region, n.name`
		args = []any{orgID}
	default:
		q = base + `AND n.access_mode = 'public' AND n.status = 'online' AND d.accepts_public_uploads = true
		            ORDER BY n.region, n.name`
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DropNodeInfo
	for rows.Next() {
		n := &DropNodeInfo{}
		var storageMax, repoSize, reserved int64
		if err := rows.Scan(&n.NodeID, &n.OrgID, &n.Name, &n.Region, &n.AccessMode, &n.DeploymentProfile,
			&n.Online, &n.State, &n.AcceptsPublicUploads, &n.WebUIAvailable, &storageMax, &repoSize, &reserved); err != nil {
			return nil, err
		}
		n.Capacity = coarseCapacity(storageMax, repoSize, reserved)
		n.AcceptingUploads = n.Online && n.State == DropStateActive && n.Capacity != DropCapacityFull &&
			(scope == DropScopePrivateOrg || n.AcceptsPublicUploads)
		out = append(out, n)
	}
	return out, rows.Err()
}

// AvailableDropCapacityBytes returns the node's remaining Drop capacity
// (storage_max - repo_size - reserved), clamped at zero. Zero storage_max means
// the node has not advertised a bound, so capacity is treated as unknown (-1).
func (n *NodeDropStatus) AvailableDropCapacityBytes() int64 {
	if n.StorageMaxBytes <= 0 {
		return -1
	}
	avail := n.StorageMaxBytes - n.RepoSizeBytes - n.ReservedBytes
	if avail < 0 {
		return 0
	}
	return avail
}
