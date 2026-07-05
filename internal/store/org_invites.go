package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const (
	OrgInviteStatusPending  = "pending"
	OrgInviteStatusAccepted = "accepted"
	OrgInviteStatusRevoked  = "revoked"
)

// OrgInvite is a pending email invitation to join an org.
type OrgInvite struct {
	ID        string `json:"id"`
	OrgID     string `json:"org_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	SeatTier  string `json:"seat_tier"`
	InvitedBy string `json:"invited_by,omitempty"`
	Status    string `json:"status"`
}

// CreateOrgInvite records a pending invite for an email address.
func (s *Store) CreateOrgInvite(ctx context.Context, orgID, email, role, seatTier, invitedBy string) (*OrgInvite, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	role = normalizeOrgRole(role)
	seatTier, err := normalizeSeatTier(seatTier)
	if err != nil {
		seatTier = SeatTierFree
	}
	var inv OrgInvite
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO org_invites (org_id, email, role, seat_tier, invited_by, status)
		 VALUES ($1, $2, $3, $4, NULLIF($5,'')::uuid, $6)
		 ON CONFLICT (org_id, email) DO UPDATE SET
			role = EXCLUDED.role,
			seat_tier = EXCLUDED.seat_tier,
			invited_by = COALESCE(EXCLUDED.invited_by, org_invites.invited_by),
			status = CASE WHEN org_invites.status = $7 THEN EXCLUDED.status ELSE org_invites.status END,
			updated_at = now()
		 RETURNING id, org_id, email, role, seat_tier, COALESCE(invited_by::text,''), status`,
		orgID, email, role, seatTier, invitedBy, OrgInviteStatusPending, OrgInviteStatusRevoked).
		Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.SeatTier, &inv.InvitedBy, &inv.Status)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// AcceptOrgInvitesForEmail activates pending invites matching a verified email.
func (s *Store) AcceptOrgInvitesForEmail(ctx context.Context, userID, email string) (int, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx,
		`SELECT org_id, role, seat_tier FROM org_invites
		 WHERE lower(email) = lower($1) AND status = $2`, email, OrgInviteStatusPending)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var accepted int
	for rows.Next() {
		var orgID, role, seatTier string
		if err := rows.Scan(&orgID, &role, &seatTier); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO org_members (org_id, user_id, role, seat_tier, status)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (org_id, user_id) DO UPDATE SET
				role = EXCLUDED.role,
				seat_tier = EXCLUDED.seat_tier,
				status = 'active',
				updated_at = now()`,
			orgID, userID, role, seatTier, MemberStatusActive); err != nil {
			return 0, err
		}
		accepted++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if accepted > 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE org_invites SET status = $3, updated_at = now()
			 WHERE lower(email) = lower($1) AND status = $2`,
			email, OrgInviteStatusPending, OrgInviteStatusAccepted); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return accepted, nil
}

// ActivateInvitedMemberships promotes invited org_members rows to active for a user.
func (s *Store) ActivateInvitedMemberships(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE org_members SET status = $2, updated_at = now()
		 WHERE user_id = $1 AND status = $3`,
		userID, MemberStatusActive, MemberStatusInvited)
	return err
}

// PendingOrgInviteCount returns how many pending email invites exist for an address.
func (s *Store) PendingOrgInviteCount(ctx context.Context, email string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM org_invites WHERE lower(email) = lower($1) AND status = $2`,
		email, OrgInviteStatusPending).Scan(&n)
	return n, err
}

// OrgNameForInvite returns the org display name for invite emails.
func (s *Store) OrgNameForInvite(ctx context.Context, orgID string) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx,
		`SELECT o.name FROM orgs o WHERE o.id = $1`, orgID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

// ListPendingOrgInvites returns pending email invitations for an org.
func (s *Store) ListPendingOrgInvites(ctx context.Context, orgID string) ([]OrgInvite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, email, role, seat_tier, COALESCE(invited_by::text,''), status
		 FROM org_invites
		 WHERE org_id = $1 AND status = $2
		 ORDER BY created_at`,
		orgID, OrgInviteStatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgInvite
	for rows.Next() {
		var inv OrgInvite
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.SeatTier, &inv.InvitedBy, &inv.Status); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}