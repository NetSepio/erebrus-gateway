package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// FindClientByUserNodeWGKey returns the newest non-deleting client for a user on
// a node with the same WireGuard public key (reconnect idempotency).
func (s *Store) FindClientByUserNodeWGKey(ctx context.Context, userID, nodeID, wgPub string) (*Client, error) {
	c, err := scanClient(s.db.QueryRowContext(ctx,
		`SELECT `+clientCols+` FROM vpn_clients
		 WHERE user_id = $1 AND node_id = $2 AND wg_public_key = $3 AND status <> 'deleting'
		 ORDER BY created_at DESC LIMIT 1`,
		userID, nodeID, wgPub))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// DeletePendingClientsByUserNodeWGKey removes stale pending rows for the same
// device identity so reconnects do not accumulate ghost clients.
func (s *Store) DeletePendingClientsByUserNodeWGKey(ctx context.Context, userID, nodeID, wgPub, keepID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM vpn_clients
		 WHERE user_id = $1 AND node_id = $2 AND wg_public_key = $3
		   AND status = 'pending' AND id <> $4`,
		userID, nodeID, wgPub, keepID)
	return err
}

// CreateClient inserts a pending VPN client and returns its generated id.
func (s *Store) CreateClient(ctx context.Context, userID, orgID, nodeID, name, wgPub string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO vpn_clients (user_id, org_id, node_id, name, wg_public_key, status)
		 VALUES ($1, NULLIF($2,'')::uuid, $3, $4, $5, 'pending')
		 RETURNING id`,
		userID, orgID, nodeID, name, wgPub).Scan(&id)
	return id, err
}

// SetClientActive marks a client active and records its assigned tunnel IP.
func (s *Store) SetClientActive(ctx context.Context, id, allowedIP string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE vpn_clients SET status='active', wg_allowed_ip=NULLIF($2,''), updated_at=now() WHERE id=$1`,
		id, allowedIP)
	return err
}

const clientCols = `id, user_id, COALESCE(org_id::text,''), node_id, name, wg_public_key,
	COALESCE(wg_allowed_ip,''), status, rx_bytes, tx_bytes, last_handshake, created_at`

func scanClient(sc interface{ Scan(...any) error }) (*Client, error) {
	var c Client
	if err := sc.Scan(&c.ID, &c.UserID, &c.OrgID, &c.NodeID, &c.Name, &c.WGPublicKey,
		&c.WGAllowedIP, &c.Status, &c.RxBytes, &c.TxBytes, &c.LastHandshake, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

// GetClient returns a client by id.
func (s *Store) GetClient(ctx context.Context, id string) (*Client, error) {
	c, err := scanClient(s.db.QueryRowContext(ctx, `SELECT `+clientCols+` FROM vpn_clients WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// ListClientsByUser returns a user's clients.
func (s *Store) ListClientsByUser(ctx context.Context, userID string) ([]*Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+clientCols+` FROM vpn_clients WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CountActiveClientsByUser counts non-deleting clients (plan-limit enforcement).
func (s *Store) CountActiveClientsByUser(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM vpn_clients WHERE user_id=$1 AND status <> 'deleting'`, userID).Scan(&n)
	return n, err
}

// DeleteClient removes a client row.
func (s *Store) DeleteClient(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM vpn_clients WHERE id=$1`, id)
	return err
}

// AddUsage applies a usage_report delta to a client: bumps cumulative counters,
// upserts the daily rollup, and records the last handshake. Unknown client ids
// are ignored (the node may report a peer the gateway has since deleted).
func (s *Store) AddUsage(ctx context.Context, clientID string, rxDelta, txDelta, lastHandshake int64) error {
	if rxDelta < 0 {
		rxDelta = 0
	}
	if txDelta < 0 {
		txDelta = 0
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var hs any
	if lastHandshake > 0 {
		hs = time.Unix(lastHandshake, 0).UTC()
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE vpn_clients
		 SET rx_bytes = rx_bytes + $2, tx_bytes = tx_bytes + $3,
		     last_handshake = COALESCE($4, last_handshake), updated_at = now()
		 WHERE id = $1`,
		clientID, rxDelta, txDelta, hs)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return tx.Commit() // unknown client: no-op
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO usage_daily (client_id, day, rx_bytes, tx_bytes)
		 VALUES ($1, current_date, $2, $3)
		 ON CONFLICT (client_id, day)
		 DO UPDATE SET rx_bytes = usage_daily.rx_bytes + EXCLUDED.rx_bytes,
		               tx_bytes = usage_daily.tx_bytes + EXCLUDED.tx_bytes`,
		clientID, rxDelta, txDelta); err != nil {
		return err
	}
	return tx.Commit()
}

// UserBandwidth sums a user's metered traffic since `since` (admin/usage views).
func (s *Store) UserBandwidth(ctx context.Context, userID string, since time.Time) (rx, tx int64, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(d.rx_bytes),0), COALESCE(SUM(d.tx_bytes),0)
		 FROM usage_daily d JOIN vpn_clients c ON c.id = d.client_id
		 WHERE c.user_id = $1 AND d.day >= $2::date`,
		userID, since).Scan(&rx, &tx)
	return
}

// TotalBandwidth sums all metered traffic since `since` (admin stats).
func (s *Store) TotalBandwidth(ctx context.Context, since time.Time) (rx, tx int64, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(rx_bytes),0), COALESCE(SUM(tx_bytes),0)
		 FROM usage_daily WHERE day >= $1::date`, since).Scan(&rx, &tx)
	return
}
