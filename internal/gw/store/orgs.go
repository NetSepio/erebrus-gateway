package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CreateOrg creates an org and records the caller as its owner.
func (s *Store) CreateOrg(ctx context.Context, name, ownerUserID string) (*Org, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var o Org
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO orgs (name, owner_user_id) VALUES ($1, $2)
		 RETURNING id, name, owner_user_id, created_at`,
		name, ownerUserID).Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, 'owner')`,
		o.ID, ownerUserID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	o.Role = "owner"
	return &o, nil
}

// GetOrg returns an org by id.
func (s *Store) GetOrg(ctx context.Context, id string) (*Org, error) {
	var o Org
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, owner_user_id, created_at FROM orgs WHERE id=$1`, id).
		Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &o, err
}

// ListOrgsForUser returns the orgs the user belongs to, with their role.
func (s *Store) ListOrgsForUser(ctx context.Context, userID string) ([]Org, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.name, o.owner_user_id, m.role, o.created_at
		 FROM orgs o JOIN org_members m ON m.org_id = o.id
		 WHERE m.user_id = $1 ORDER BY o.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Org
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.Role, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// MemberRole returns the caller's role in an org, or ErrNotFound if not a member.
func (s *Store) MemberRole(ctx context.Context, orgID, userID string) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return role, err
}

// AddMember adds (or updates the role of) a member.
func (s *Store) AddMember(ctx context.Context, orgID, userID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, $3)
		 ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		orgID, userID, role)
	return err
}

// ListMembers returns an org's members with wallet addresses.
func (s *Store) ListMembers(ctx context.Context, orgID string) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.user_id, COALESCE(u.wallet_address,''), m.role, m.added_at
		 FROM org_members m JOIN users u ON u.id = m.user_id
		 WHERE m.org_id = $1 ORDER BY m.added_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.WalletAddress, &m.Role, &m.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// OrgClients returns VPN clients provisioned under an org.
func (s *Store) OrgClients(ctx context.Context, orgID string) ([]*Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+clientCols+` FROM vpn_clients WHERE org_id=$1 ORDER BY created_at DESC`, orgID)
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

// OrgBandwidth sums an org's metered traffic since `since`.
func (s *Store) OrgBandwidth(ctx context.Context, orgID string, since time.Time) (rx, tx int64, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(d.rx_bytes),0), COALESCE(SUM(d.tx_bytes),0)
		 FROM usage_daily d JOIN vpn_clients c ON c.id = d.client_id
		 WHERE c.org_id = $1 AND d.day >= $2::date`, orgID, since).Scan(&rx, &tx)
	return
}

// OrgAPICalls sums an org's API calls since `since`.
func (s *Store) OrgAPICalls(ctx context.Context, orgID string, since time.Time) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(api_calls),0) FROM api_usage_daily WHERE org_id=$1 AND day >= $2::date`,
		orgID, since).Scan(&n)
	return n, err
}

// IncrAPICall records one API call against an org for today.
func (s *Store) IncrAPICall(ctx context.Context, orgID string) {
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO api_usage_daily (org_id, day, api_calls) VALUES ($1, current_date, 1)
		 ON CONFLICT (org_id, day) DO UPDATE SET api_calls = api_usage_daily.api_calls + 1`, orgID)
}

// CountOrgs returns the total org count (admin stats).
func (s *Store) CountOrgs(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM orgs`).Scan(&n)
	return n, err
}

// ListOrgs returns a page of all orgs (admin).
func (s *Store) ListOrgs(ctx context.Context, limit, offset int) ([]Org, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, owner_user_id, created_at FROM orgs ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Org
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
