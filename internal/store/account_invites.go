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
	ID             string    `json:"id"`
	OrgID          string    `json:"org_id"`
	OrgName        string    `json:"org_name"`
	OrgSlug        string    `json:"org_slug,omitempty"`
	OrgDisplayName string    `json:"org_display_name,omitempty"`
	OrgPlan        string    `json:"org_plan,omitempty"`
	OrgDescription string    `json:"org_description,omitempty"`
	OrgLogoURL     string    `json:"org_logo_url,omitempty"`
	MemberCount    int       `json:"member_count"`
	NodeCount      int       `json:"node_count"`
	Role           string    `json:"role"`
	SeatTier       string    `json:"seat_tier,omitempty"`
	Source         string    `json:"source"`                   // membership | email
	InviteChannel  string    `json:"invite_channel,omitempty"` // wallet | email
	InvitedByID    string    `json:"invited_by_id,omitempty"`
	InvitedByName  string    `json:"invited_by_name,omitempty"`
	InvitedByEmail string    `json:"invited_by_email,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
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
		inv.InviteChannel = "wallet"
		seen[inv.OrgID] = struct{}{}
		if err := s.enrichUserOrgInvite(ctx, &inv, email); err != nil {
			return nil, err
		}
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
		`SELECT i.id::text, o.id::text, o.name, COALESCE(o.slug,''), i.role, COALESCE(i.seat_tier,''),
		        COALESCE(i.invited_by::text,''), i.created_at
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
		var invitedBy string
		if err := emailRows.Scan(&inv.ID, &inv.OrgID, &inv.OrgName, &inv.OrgSlug, &inv.Role, &inv.SeatTier, &invitedBy, &inv.CreatedAt); err != nil {
			return nil, err
		}
		if _, dup := seen[inv.OrgID]; dup {
			continue
		}
		inv.Source = "email"
		inv.InviteChannel = "email"
		if err := s.enrichUserOrgInvite(ctx, &inv, email); err != nil {
			return nil, err
		}
		if invitedBy != "" {
			inv.InvitedByID = invitedBy
			s.applyInviter(ctx, &inv, invitedBy)
		}
		out = append(out, inv)
	}
	return out, emailRows.Err()
}

// GetUserOrgInvite returns one pending invite for the user within an org.
func (s *Store) GetUserOrgInvite(ctx context.Context, userID, orgID, email string) (*UserOrgInvite, error) {
	invites, err := s.ListUserOrgInvites(ctx, userID, email)
	if err != nil {
		return nil, err
	}
	for i := range invites {
		if invites[i].OrgID == orgID {
			return &invites[i], nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) enrichUserOrgInvite(ctx context.Context, inv *UserOrgInvite, userEmail string) error {
	org, err := s.GetOrg(ctx, inv.OrgID)
	if err != nil {
		return err
	}
	inv.OrgPlan = org.Plan
	if profile, err := s.GetOrgProfile(ctx, inv.OrgID); err == nil {
		inv.OrgDisplayName = strings.TrimSpace(profile.DisplayName)
		inv.OrgDescription = strings.TrimSpace(profile.Description)
		inv.OrgLogoURL = strings.TrimSpace(profile.LogoURL)
	}
	if inv.OrgDisplayName == "" {
		inv.OrgDisplayName = org.Name
	}

	_ = s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM org_members WHERE org_id=$1 AND status IN ('active','invited')`, inv.OrgID).
		Scan(&inv.MemberCount)
	_ = s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM nodes WHERE org_id=$1::uuid`, inv.OrgID).
		Scan(&inv.NodeCount)

	if inv.InvitedByName == "" {
		if inv.Source == "email" && userEmail != "" {
			var invitedBy string
			_ = s.db.QueryRowContext(ctx,
				`SELECT COALESCE(invited_by::text,'') FROM org_invites
				 WHERE org_id=$1 AND lower(email)=lower($2) AND status=$3`,
				inv.OrgID, userEmail, OrgInviteStatusPending).Scan(&invitedBy)
			if invitedBy != "" {
				inv.InvitedByID = invitedBy
				s.applyInviter(ctx, inv, invitedBy)
			}
		}
		if inv.InvitedByName == "" && org.OwnerUserID != "" {
			inv.InvitedByID = org.OwnerUserID
			s.applyInviter(ctx, inv, org.OwnerUserID)
		}
	}
	return nil
}

func (s *Store) applyInviter(ctx context.Context, inv *UserOrgInvite, userID string) {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return
	}
	if n := strings.TrimSpace(u.Name); n != "" {
		inv.InvitedByName = n
	} else if u.EmailVerified && strings.TrimSpace(u.Email) != "" {
		inv.InvitedByName = u.Email
	} else if w := strings.TrimSpace(u.WalletAddress); w != "" {
		if len(w) > 10 {
			inv.InvitedByName = w[:6] + "…" + w[len(w)-4:]
		} else {
			inv.InvitedByName = w
		}
	}
	if u.EmailVerified {
		inv.InvitedByEmail = strings.TrimSpace(u.Email)
	}
}

// InviteNotificationContext carries data for invite outcome emails.
type InviteNotificationContext struct {
	OrgID          string
	OrgName        string
	OrgDisplayName string
	OrgSlug        string
	Role           string
	SeatTier       string
	InviteeName    string
	InviteeEmail   string
	InviterID      string
	InviterName    string
	InviterEmail   string
	OwnerID        string
	OwnerEmail     string
}

// InviteNotificationContextForUser loads email context for an org invite action.
func (s *Store) InviteNotificationContextForUser(ctx context.Context, userID, orgID, email string) (*InviteNotificationContext, error) {
	inv, err := s.GetUserOrgInvite(ctx, userID, orgID, email)
	if err != nil {
		return nil, err
	}
	org, err := s.GetOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := &InviteNotificationContext{
		OrgID:          orgID,
		OrgName:        org.Name,
		OrgDisplayName: inv.OrgDisplayName,
		OrgSlug:        org.Slug,
		Role:           inv.Role,
		SeatTier:       inv.SeatTier,
		InviteeEmail:   strings.TrimSpace(email),
		OwnerID:        org.OwnerUserID,
	}
	if out.OrgDisplayName == "" {
		out.OrgDisplayName = org.Name
	}
	if n := strings.TrimSpace(u.Name); n != "" {
		out.InviteeName = n
	} else if out.InviteeEmail != "" {
		out.InviteeName = out.InviteeEmail
	} else if w := strings.TrimSpace(u.WalletAddress); w != "" {
		out.InviteeName = w
	}

	out.InviterName = inv.InvitedByName
	out.InviterEmail = inv.InvitedByEmail
	out.InviterID = inv.InvitedByID
	if out.InviterID != "" {
		if inviter, err := s.GetUser(ctx, out.InviterID); err == nil && inviter.EmailVerified {
			out.InviterEmail = strings.TrimSpace(inviter.Email)
			if out.InviterName == "" {
				out.InviterName = strings.TrimSpace(inviter.Name)
			}
		}
	}
	if org.OwnerUserID != "" {
		if owner, err := s.GetUser(ctx, org.OwnerUserID); err == nil && owner.EmailVerified {
			out.OwnerEmail = strings.TrimSpace(owner.Email)
		}
	}
	return out, nil
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
