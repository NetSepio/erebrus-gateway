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
	COALESCE(kubo_version,''), repo_size_bytes, storage_max_bytes, num_objects, reserved_bytes, last_reported_at`

func scanNodeDropStatus(sc interface{ Scan(...any) error }) (*NodeDropStatus, error) {
	n := &NodeDropStatus{}
	if err := sc.Scan(&n.NodeID, &n.Enabled, &n.AcceptsPublicUploads, &n.WebUIAvailable, &n.State,
		&n.KuboVersion, &n.RepoSizeBytes, &n.StorageMaxBytes, &n.NumObjects, &n.ReservedBytes, &n.LastReportedAt); err != nil {
		return nil, err
	}
	return n, nil
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
