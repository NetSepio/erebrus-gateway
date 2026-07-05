package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// UserOrgInvite is a pending workspace invitation for the authenticated user.
type UserOrgInvite struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	OrgName   string    `json:"org_name"`
	OrgSlug   string    `json:"org_slug,omitempty"`
	Role      string    `json:"role"`
	SeatTier  string    `json:"seat_tier,omitempty"`
	Source    string    `json:"source"` // membership | email
	CreatedAt time.Time `json:"created_at"`
}

// ListUserOrgInvites returns pending membership and email invites for a user.
func (s *Store) ListUserOrgInvites(ctx context.Context, userID, email string) ([]UserOrgInvite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id::text, o.id::text, o.name, COALESCE(o.slug,''), m.role, COALESCE(m.seat_tier,''), m.created_at
		 FROM org_members m
		 JOIN orgs o ON o.id = m.org_id
		 WHERE m.user_id = $1 AND m.status = $2
		 ORDER BY m.created_at DESC`,
		userID, MemberStatusInvited)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]struct{}{}
	var out []UserOrgInvite
	for rows.Next() {
		var inv UserOrgInvite
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.OrgName, &inv.OrgSlug, &inv.Role, &inv.SeatTier, &inv.CreatedAt); err != nil {
			return nil, err
		}
		inv.Source = "membership"
		seen[inv.OrgID] = struct{}{}
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return out, nil
	}

	emailRows, err := s.db.QueryContext(ctx,
		`SELECT i.id::text, o.id::text, o.name, COALESCE(o.slug,''), i.role, COALESCE(i.seat_tier,''), i.created_at
		 FROM org_invites i
		 JOIN orgs o ON o.id = i.org_id
		 WHERE lower(i.email) = lower($1) AND i.status = $2
		 ORDER BY i.created_at DESC`,
		email, OrgInviteStatusPending)
	if err != nil {
		return nil, err
	}
	defer emailRows.Close()

	for emailRows.Next() {
		var inv UserOrgInvite
		if err := emailRows.Scan(&inv.ID, &inv.OrgID, &inv.OrgName, &inv.OrgSlug, &inv.Role, &inv.SeatTier, &inv.CreatedAt); err != nil {
			return nil, err
		}
		if _, dup := seen[inv.OrgID]; dup {
			continue
		}
		inv.Source = "email"
		out = append(out, inv)
	}
	return out, emailRows.Err()
}

// AcceptUserOrgInvite activates a pending membership or email invite for one org.
func (s *Store) AcceptUserOrgInvite(ctx context.Context, userID, orgID, email string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var (
		role     string
		seatTier string
		found    bool
	)
	err = tx.QueryRowContext(ctx,
		`SELECT role, seat_tier FROM org_members
		 WHERE org_id = $1 AND user_id = $2 AND status = $3`,
		orgID, userID, MemberStatusInvited).
		Scan(&role, &seatTier)
	if err == nil {
		found = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if !found {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			return ErrNotFound
		}
		err = tx.QueryRowContext(ctx,
			`SELECT role, seat_tier FROM org_invites
			 WHERE org_id = $1 AND lower(email) = lower($2) AND status = $3`,
			orgID, email, OrgInviteStatusPending).
			Scan(&role, &seatTier)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
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
		return err
	}

	if email != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE org_invites SET status = $4, updated_at = now()
			 WHERE org_id = $1 AND lower(email) = lower($2) AND status = $3`,
			orgID, email, OrgInviteStatusPending, OrgInviteStatusAccepted); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeclineUserOrgInvite rejects a pending membership or email invite for one org.
func (s *Store) DeclineUserOrgInvite(ctx context.Context, userID, orgID, email string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx,
		`UPDATE org_members SET status = $4, updated_at = now()
		 WHERE org_id = $1 AND user_id = $2 AND status = $3`,
		orgID, userID, MemberStatusInvited, MemberStatusRemoved)
	if err != nil {
		return err
	}
	membershipRows, _ := res.RowsAffected()

	email = strings.ToLower(strings.TrimSpace(email))
	var inviteRows int64
	if email != "" {
		res, err = tx.ExecContext(ctx,
			`UPDATE org_invites SET status = $4, updated_at = now()
			 WHERE org_id = $1 AND lower(email) = lower($2) AND status = $3`,
			orgID, email, OrgInviteStatusPending, OrgInviteStatusRevoked)
		if err != nil {
			return err
		}
		inviteRows, _ = res.RowsAffected()
	}

	if membershipRows == 0 && inviteRows == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}