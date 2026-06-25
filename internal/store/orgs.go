package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/secrets"
)

const orgCols = `id, name, COALESCE(kind,'team'), verified,
	COALESCE(slug,''), COALESCE(description,''), COALESCE(website,''),
	owner_user_id, COALESCE(enrollment_secret,''), created_at, updated_at`

func scanOrg(sc interface{ Scan(...any) error }, includeSecret bool) (*Org, error) {
	var o Org
	var secret string
	dest := []any{&o.ID, &o.Name, &o.Kind, &o.Verified, &o.Slug, &o.Description, &o.Website,
		&o.OwnerUserID, &secret, &o.CreatedAt, &o.UpdatedAt}
	if err := sc.Scan(dest...); err != nil {
		return nil, err
	}
	if includeSecret {
		o.EnrollmentSecret = secret
	}
	return &o, nil
}

// CreateOrgInput carries fields for org creation.
type CreateOrgInput struct {
	Name        string
	Kind        string
	Slug        string
	Description string
	Website     string
	OwnerUserID string
}

// CreateOrg creates an org, records the caller as owner, and mints an enrollment secret.
func (s *Store) CreateOrg(ctx context.Context, in CreateOrgInput) (*Org, error) {
	kind := normalizeOrgKind(in.Kind)
	secret, err := secrets.NewOrgEnrollmentSecret()
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var o Org
	err = tx.QueryRowContext(ctx,
		`INSERT INTO orgs (name, kind, slug, description, website, owner_user_id, enrollment_secret)
		 VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), $6, $7)
		 RETURNING `+orgCols,
		in.Name, kind, in.Slug, in.Description, in.Website, in.OwnerUserID, secret).
		Scan(&o.ID, &o.Name, &o.Kind, &o.Verified, &o.Slug, &o.Description, &o.Website,
			&o.OwnerUserID, &o.EnrollmentSecret, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, 'owner')`,
		o.ID, in.OwnerUserID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	o.Role = OrgRoleOwner
	return &o, nil
}

// GetOrg returns an org by id (no enrollment secret).
func (s *Store) GetOrg(ctx context.Context, id string) (*Org, error) {
	o, err := scanOrg(s.db.QueryRowContext(ctx, `SELECT `+orgCols+` FROM orgs WHERE id=$1`, id), false)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

// GetOrgWithSecret returns an org including its enrollment secret (owner/admin views).
func (s *Store) GetOrgWithSecret(ctx context.Context, id string) (*Org, error) {
	o, err := scanOrg(s.db.QueryRowContext(ctx, `SELECT `+orgCols+` FROM orgs WHERE id=$1`, id), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

// LookupOrgByEnrollmentSecret resolves an org from a presented enrollment secret.
func (s *Store) LookupOrgByEnrollmentSecret(ctx context.Context, secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	var orgID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM orgs WHERE enrollment_secret = $1`, secret).Scan(&orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return orgID, err
}

// ListOrgsForUser returns the orgs the user belongs to, with their role.
func (s *Store) ListOrgsForUser(ctx context.Context, userID string) ([]Org, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.name, COALESCE(o.kind,'team'), o.verified,
		        COALESCE(o.slug,''), COALESCE(o.description,''), COALESCE(o.website,''),
		        o.owner_user_id, o.created_at, o.updated_at, m.role
		 FROM orgs o JOIN org_members m ON m.org_id = o.id
		 WHERE m.user_id = $1 ORDER BY o.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Org
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.Kind, &o.Verified, &o.Slug, &o.Description, &o.Website,
			&o.OwnerUserID, &o.CreatedAt, &o.UpdatedAt, &o.Role); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// UpdateOrg patches org metadata (not membership).
type UpdateOrgInput struct {
	Name        *string
	Kind        *string
	Slug        *string
	Description *string
	Website     *string
}

// UpdateOrg updates org fields. Nil pointers are left unchanged.
func (s *Store) UpdateOrg(ctx context.Context, orgID string, in UpdateOrgInput) (*Org, error) {
	cur, err := s.GetOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	name, kind, slug, desc, site := cur.Name, cur.Kind, cur.Slug, cur.Description, cur.Website
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.Kind != nil {
		kind = normalizeOrgKind(*in.Kind)
	}
	if in.Slug != nil {
		slug = strings.TrimSpace(*in.Slug)
	}
	if in.Description != nil {
		desc = strings.TrimSpace(*in.Description)
	}
	if in.Website != nil {
		site = strings.TrimSpace(*in.Website)
	}
	o, err := scanOrg(s.db.QueryRowContext(ctx,
		`UPDATE orgs SET name=$2, kind=$3, slug=NULLIF($4,''), description=NULLIF($5,''),
		 website=NULLIF($6,''), updated_at=now()
		 WHERE id=$1
		 RETURNING `+orgCols,
		orgID, name, kind, slug, desc, site), false)
	if err != nil {
		return nil, err
	}
	return o, nil
}

// MemberRole returns the caller's role in an org.
func (s *Store) MemberRole(ctx context.Context, orgID, userID string) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return role, err
}

// IsOrgPrivileged reports whether the role can see org_id and enrollment_secret.
func IsOrgPrivileged(role string) bool {
	return role == OrgRoleOwner || role == OrgRoleAdmin
}

// AddMember adds or updates a member's role.
func (s *Store) AddMember(ctx context.Context, orgID, userID, role string) error {
	role = normalizeOrgRole(role)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, $3)
		 ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		orgID, userID, role)
	return err
}

// RemoveMember removes a user from an org (not the owner).
func (s *Store) RemoveMember(ctx context.Context, orgID, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM org_members WHERE org_id=$1 AND user_id=$2 AND role <> 'owner'`,
		orgID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TransferOrgOwnership moves ownership to another admin/owner member.
func (s *Store) TransferOrgOwnership(ctx context.Context, orgID, fromUserID, toUserID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var fromRole string
	err = tx.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, fromUserID).Scan(&fromRole)
	if err != nil || fromRole != OrgRoleOwner {
		return ErrNotFound
	}
	var toRole string
	err = tx.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, toUserID).Scan(&toRole)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if toRole != OrgRoleAdmin && toRole != OrgRoleOwner {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_members SET role='admin' WHERE org_id=$1 AND user_id=$2`, orgID, fromUserID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_members SET role='owner' WHERE org_id=$1 AND user_id=$2`, orgID, toUserID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE orgs SET owner_user_id=$2, updated_at=now() WHERE id=$1`, orgID, toUserID); err != nil {
		return err
	}
	return tx.Commit()
}

// ListMembers returns an org's members.
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
func (s *Store) OrgClients(ctx context.Context, orgID string) ([]Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+clientCols+` FROM vpn_clients WHERE org_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// OrgBandwidth returns rx/tx bytes for an org's clients since `since`.
func (s *Store) OrgBandwidth(ctx context.Context, orgID string, since time.Time) (rx, tx int64, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(d.rx_bytes),0), COALESCE(SUM(d.tx_bytes),0)
		 FROM vpn_usage_daily d JOIN vpn_clients c ON c.id = d.client_id
		 WHERE c.org_id = $1 AND d.day >= $2::date`, orgID, since).Scan(&rx, &tx)
	return
}

// OrgAPICalls returns API call count for an org since `since`.
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

// ListOrgs returns orgs for admin listing.
func (s *Store) ListOrgs(ctx context.Context, limit, offset int) ([]Org, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+orgCols+` FROM orgs ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Org
	for rows.Next() {
		o, err := scanOrg(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

// SetOrgVerified sets the platform verification flag (admin).
func (s *Store) SetOrgVerified(ctx context.Context, orgID string, verified bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE orgs SET verified=$2, updated_at=now() WHERE id=$1`, orgID, verified)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func normalizeOrgKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case OrgKindCompany, OrgKindIndividual, OrgKindFamily, OrgKindTeam:
		return strings.ToLower(strings.TrimSpace(kind))
	default:
		return OrgKindTeam
	}
}

func normalizeOrgRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case OrgRoleOwner:
		return OrgRoleOwner
	case OrgRoleAdmin:
		return OrgRoleAdmin
	default:
		return OrgRoleMember
	}
}