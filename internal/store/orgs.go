package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const orgCols = `id, name, slug, plan, billing_status, verification_status,
	public_profile_enabled, owner_user_id, created_at, updated_at`

func scanOrg(sc interface{ Scan(...any) error }, _ bool) (*Org, error) {
	var o Org
	err := sc.Scan(
		&o.ID, &o.Name, &o.Slug, &o.Plan, &o.BillingStatus, &o.VerificationStatus,
		&o.PublicProfileEnabled, &o.OwnerUserID, &o.CreatedAt, &o.UpdatedAt,
	)
	return &o, err
}

// CreateOrgInput carries fields for org creation.
type CreateOrgInput struct {
	Name string
	Slug string
}

// CreateOrgForUser creates an org on the basic plan with profile and entitlements.
func (s *Store) CreateOrgForUser(ctx context.Context, userID string, in CreateOrgInput) (*Org, error) {
	return s.createOrg(ctx, userID, in, OrgPlanBasic)
}

// EnsurePersonalOrg guarantees the user owns at least one org: if they own none,
// it creates a personal workspace on the basic plan. Idempotent. Returns the org
// and whether it was just created. Every user gets a basic-plan org at first
// login; they may create more orgs or have an admin upgrade this one's plan.
func (s *Store) EnsurePersonalOrg(ctx context.Context, userID, walletAddress string) (*Org, bool, error) {
	var existingID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id::text FROM orgs WHERE owner_user_id = $1::uuid ORDER BY created_at ASC LIMIT 1`,
		userID).Scan(&existingID)
	if err == nil {
		org, gerr := s.GetOrg(ctx, existingID)
		return org, false, gerr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	org, err := s.createOrg(ctx, userID, CreateOrgInput{Name: personalOrgName(walletAddress)}, OrgPlanBasic)
	if err != nil {
		return nil, false, err
	}
	return org, true, nil
}

// UserOrgVPNPlan returns the highest paid org plan that entitles the user to VPN,
// or "" if none. A paid-plan org grants VPN to its seated members: the owner
// (always) and any member holding a paid seat (org_members.seat_tier <> 'free',
// assigned by an admin up to org_entitlements.paid_seats_included). Members
// without a seat fall back to their personal trial/NFT. The manual seat
// assignment is the single source of truth (CountPaidSeatsUsed == VPN-entitled
// members). Independent of the per-user subscription.
func (s *Store) UserOrgVPNPlan(ctx context.Context, userID string) (string, error) {
	var plan string
	err := s.db.QueryRowContext(ctx,
		`SELECT o.plan
		 FROM org_members m
		 JOIN orgs o ON o.id = m.org_id
		 WHERE m.user_id = $1::uuid AND m.status = 'active'
		   AND o.billing_status = $2
		   AND o.plan IN ('starter','pro','business','enterprise')
		   AND (m.role = 'owner' OR m.seat_tier <> 'free')
		 ORDER BY array_position(ARRAY['starter','pro','business','enterprise'], o.plan) DESC
		 LIMIT 1`, userID, OrgBillingActive).Scan(&plan)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return plan, nil
}

// personalOrgName derives a readable default name for a user's personal org.
func personalOrgName(wallet string) string {
	w := strings.TrimSpace(wallet)
	switch {
	case len(w) >= 10:
		return "Workspace " + w[:6] + "…" + w[len(w)-4:]
	case w != "":
		return "Workspace " + w
	default:
		return "Personal Workspace"
	}
}

// createOrg is the internal org creation flow.
func (s *Store) createOrg(ctx context.Context, ownerUserID string, in CreateOrgInput, plan string) (*Org, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	slug, err := s.resolveOrgSlug(ctx, in.Slug, name, "")
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
		`INSERT INTO orgs (name, slug, plan, billing_status, verification_status, owner_user_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+orgCols,
		name, slug, plan, OrgBillingActive, OrgVerificationUnverified, ownerUserID).
		Scan(&o.ID, &o.Name, &o.Slug, &o.Plan, &o.BillingStatus, &o.VerificationStatus,
			&o.PublicProfileEnabled, &o.OwnerUserID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO org_profiles (org_id, display_name) VALUES ($1, $2)`, o.ID, name); err != nil {
		return nil, err
	}

	tpl := PlanEntitlementTemplates()[plan]
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO org_entitlements (
			org_id, plan, paid_seats_included, managed_vpn_nodes_included,
			shield_instances_included, sentinel_licenses_included,
			public_node_access_tier, support_tier, audit_logs_enabled, advanced_analytics_enabled
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		o.ID, tpl.Plan, tpl.PaidSeatsIncluded, tpl.ManagedVPNNodesIncluded,
		tpl.ShieldInstancesIncluded, tpl.SentinelLicensesIncluded,
		tpl.PublicNodeAccessTier, tpl.SupportTier, tpl.AuditLogsEnabled, tpl.AdvancedAnalyticsEnabled); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO org_members (org_id, user_id, role, seat_tier, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		o.ID, ownerUserID, OrgRoleOwner, ownerSeatTierForPlan(plan), MemberStatusActive); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	o.Role = OrgRoleOwner
	return &o, nil
}

// SetOrgPlan updates an org's plan and refreshes entitlements.
func (s *Store) SetOrgPlan(ctx context.Context, orgID, plan string) (*Org, error) {
	plan, err := normalizeOrgPlan(plan)
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
		`UPDATE orgs SET plan=$2, updated_at=now() WHERE id=$1
		 RETURNING `+orgCols, orgID, plan).
		Scan(&o.ID, &o.Name, &o.Slug, &o.Plan, &o.BillingStatus, &o.VerificationStatus,
			&o.PublicProfileEnabled, &o.OwnerUserID, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	tpl := PlanEntitlementTemplates()[plan]
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO org_entitlements (
			org_id, plan, paid_seats_included, managed_vpn_nodes_included,
			shield_instances_included, sentinel_licenses_included,
			public_node_access_tier, support_tier, audit_logs_enabled, advanced_analytics_enabled
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (org_id) DO UPDATE SET
			plan = EXCLUDED.plan,
			paid_seats_included = EXCLUDED.paid_seats_included,
			managed_vpn_nodes_included = EXCLUDED.managed_vpn_nodes_included,
			shield_instances_included = EXCLUDED.shield_instances_included,
			sentinel_licenses_included = EXCLUDED.sentinel_licenses_included,
			public_node_access_tier = EXCLUDED.public_node_access_tier,
			support_tier = EXCLUDED.support_tier,
			audit_logs_enabled = EXCLUDED.audit_logs_enabled,
			advanced_analytics_enabled = EXCLUDED.advanced_analytics_enabled,
			updated_at = now()`,
		orgID, tpl.Plan, tpl.PaidSeatsIncluded, tpl.ManagedVPNNodesIncluded,
		tpl.ShieldInstancesIncluded, tpl.SentinelLicensesIncluded,
		tpl.PublicNodeAccessTier, tpl.SupportTier, tpl.AuditLogsEnabled, tpl.AdvancedAnalyticsEnabled); err != nil {
		return nil, err
	}

	// Owner occupies one seat on a paid plan (so "seats used" counts the owner and
	// starter = owner-only); basic resets the owner to a free seat.
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_members m SET seat_tier=$2, updated_at=now()
		 FROM orgs o
		 WHERE o.id = m.org_id AND m.org_id=$1 AND m.user_id=o.owner_user_id AND m.status='active'`,
		orgID, ownerSeatTierForPlan(plan)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &o, nil
}

// DeleteOrg permanently removes an org. Child rows (members, entitlements,
// profiles, nodes records, api keys, invites, firewall rules) cascade; runtime
// nodes are detached (nodes.org_id ON DELETE SET NULL) and keep running.
func (s *Store) DeleteOrg(ctx context.Context, orgID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM orgs WHERE id=$1::uuid`, orgID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AssignSeat assigns a paid seat tier to a member.
func (s *Store) AssignSeat(ctx context.Context, orgID, userID, seatTier string) error {
	seatTier, err := normalizeSeatTier(seatTier)
	if err != nil {
		return err
	}
	if seatTier == SeatTierFree {
		return fmt.Errorf("use RevokeSeat to remove a paid seat")
	}
	org, err := s.GetOrg(ctx, orgID)
	if err != nil {
		return err
	}
	if !SeatTierAllowedForPlan(org.Plan, seatTier) {
		return fmt.Errorf("seat tier %s is not included in plan %s", seatTier, org.Plan)
	}
	ent, err := s.GetOrgEntitlements(ctx, orgID)
	if err != nil {
		return err
	}
	used, err := s.CountPaidSeatsUsed(ctx, orgID)
	if err != nil {
		return err
	}
	var curTier string
	err = s.db.QueryRowContext(ctx,
		`SELECT seat_tier FROM org_members WHERE org_id=$1 AND user_id=$2 AND status='active'`,
		orgID, userID).Scan(&curTier)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if curTier == seatTier {
		return nil
	}
	if curTier == SeatTierFree && used >= ent.PaidSeatsIncluded {
		return fmt.Errorf("no paid seats remaining")
	}
	// A seat grants manager (admin) access alongside VPN entitlement; the owner's
	// role is never downgraded by this.
	_, err = s.db.ExecContext(ctx,
		`UPDATE org_members SET seat_tier=$3,
		   role = CASE WHEN role='owner' THEN role ELSE 'admin' END,
		   updated_at=now()
		 WHERE org_id=$1 AND user_id=$2 AND status='active'`,
		orgID, userID, seatTier)
	return err
}

// RevokeSeat removes a paid seat from a member.
func (s *Store) RevokeSeat(ctx context.Context, orgID, userID string) error {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2 AND status='active'`,
		orgID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if role == OrgRoleOwner {
		return fmt.Errorf("cannot revoke the owner's seat")
	}
	// Revoking a seat removes VPN entitlement and the manager role together.
	_, err = s.db.ExecContext(ctx,
		`UPDATE org_members SET seat_tier=$3, role='member', updated_at=now()
		 WHERE org_id=$1 AND user_id=$2 AND status='active'`,
		orgID, userID, SeatTierFree)
	return err
}

// GetOrg returns an org by id.
func (s *Store) GetOrg(ctx context.Context, id string) (*Org, error) {
	o, err := scanOrg(s.db.QueryRowContext(ctx, `SELECT `+orgCols+` FROM orgs WHERE id=$1`, id), false)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

// ListOrgsForUser returns the orgs the user belongs to, with their role.
func (s *Store) ListOrgsForUser(ctx context.Context, userID string) ([]Org, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.name, o.slug, o.plan, o.billing_status, o.verification_status,
		        o.public_profile_enabled, o.owner_user_id, o.created_at, o.updated_at, m.role, m.seat_tier
		 FROM orgs o JOIN org_members m ON m.org_id = o.id
		 WHERE m.user_id = $1 AND m.status = 'active' ORDER BY o.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Org
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.Plan, &o.BillingStatus, &o.VerificationStatus,
			&o.PublicProfileEnabled, &o.OwnerUserID, &o.CreatedAt, &o.UpdatedAt, &o.Role, &o.SeatTier); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// UpdateOrgInput carries patchable org fields.
type UpdateOrgInput struct {
	Name                 *string
	Slug                 *string
	BillingStatus        *string
	PublicProfileEnabled *bool
}

// UpdateOrg updates org fields. Nil pointers are left unchanged.
func (s *Store) UpdateOrg(ctx context.Context, orgID string, in UpdateOrgInput) (*Org, error) {
	cur, err := s.GetOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	name, slug, billing := cur.Name, cur.Slug, cur.BillingStatus
	publicProfile := cur.PublicProfileEnabled
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.Slug != nil {
		requested := strings.TrimSpace(*in.Slug)
		if requested == cur.Slug {
			slug = cur.Slug
		} else {
			slug, err = s.resolveOrgSlug(ctx, requested, name, orgID)
			if err != nil {
				return nil, err
			}
		}
	}
	if in.BillingStatus != nil {
		billing = strings.TrimSpace(*in.BillingStatus)
	}
	if in.PublicProfileEnabled != nil {
		publicProfile = *in.PublicProfileEnabled
	}
	o, err := scanOrg(s.db.QueryRowContext(ctx,
		`UPDATE orgs SET name=$2, slug=$3, billing_status=$4, public_profile_enabled=$5, updated_at=now()
		 WHERE id=$1
		 RETURNING `+orgCols,
		orgID, name, slug, billing, publicProfile), false)
	if err != nil {
		return nil, err
	}
	return o, nil
}

// MemberRole returns the caller's role in an org.
func (s *Store) MemberRole(ctx context.Context, orgID, userID string) (string, error) {
	role, _, err := s.MemberMembership(ctx, orgID, userID)
	return role, err
}

// MemberMembership returns the caller's role and seat tier in an org.
func (s *Store) MemberMembership(ctx context.Context, orgID, userID string) (role, seatTier string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT role, seat_tier FROM org_members WHERE org_id=$1 AND user_id=$2 AND status='active'`,
		orgID, userID).Scan(&role, &seatTier)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return role, seatTier, err
}

// IsOrgPrivileged reports whether the role can manage org settings.
func IsOrgPrivileged(role string) bool {
	return role == OrgRoleOwner || role == OrgRoleAdmin
}

// CanManageOrgNodes reports whether the role may register and operate org nodes.
func CanManageOrgNodes(role string) bool {
	return role == OrgRoleOwner || role == OrgRoleAdmin || role == OrgRoleNodeOperator
}

// RoleRequiresPaidSeat reports whether assigning the role consumes a paid seat.
func RoleRequiresPaidSeat(role string) bool {
	return role == OrgRoleNodeOperator
}

// MemberHasPaidSeat reports whether the member holds a paid seat (owner always does).
func MemberHasPaidSeat(role, seatTier string) bool {
	return role == OrgRoleOwner || (seatTier != "" && seatTier != SeatTierFree)
}

// UserHasActiveOrgMembership reports whether the user belongs to any active workspace.
func (s *Store) UserHasActiveOrgMembership(ctx context.Context, userID string) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM org_members WHERE user_id=$1 AND status=$2)`,
		userID, MemberStatusActive).Scan(&ok)
	return ok, err
}

// UserHasOrgSeat reports whether the user owns, or holds a paid seat in, the org
// (the "manager" gate for sensitive org resources like node admin credentials).
func (s *Store) UserHasOrgSeat(ctx context.Context, orgID, userID string) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM org_members
			WHERE org_id = $1 AND user_id = $2 AND status = 'active'
			  AND (role = 'owner' OR seat_tier <> 'free')
		)`, orgID, userID).Scan(&ok)
	return ok, err
}

// EnsureManagerRoleSeat ensures a manager (node_operator) role has a paid seat:
// reuses an existing paid seat or assigns one when capacity remains.
func (s *Store) EnsureManagerRoleSeat(ctx context.Context, orgID, role, seatTier string) (string, error) {
	if !RoleRequiresPaidSeat(role) {
		return seatTier, nil
	}
	seatTier, err := normalizeSeatTier(seatTier)
	if err != nil {
		seatTier = SeatTierFree
	}
	if MemberHasPaidSeat("", seatTier) {
		return seatTier, nil
	}
	org, err := s.GetOrg(ctx, orgID)
	if err != nil {
		return "", err
	}
	ent, err := s.GetOrgEntitlements(ctx, orgID)
	if err != nil {
		return "", err
	}
	used, err := s.CountPaidSeatsUsed(ctx, orgID)
	if err != nil {
		return "", err
	}
	if used >= ent.PaidSeatsIncluded {
		return "", fmt.Errorf("manager role requires a paid seat — none remaining")
	}
	planTier, ok := planSeatTierForOrg(org.Plan)
	if !ok {
		return "", fmt.Errorf("manager role requires a paid seat — upgrade the workspace plan")
	}
	return planTier, nil
}

func planSeatTierForOrg(plan string) (string, bool) {
	switch plan {
	case OrgPlanStarter:
		return SeatTierStarter, true
	case OrgPlanPro:
		return SeatTierPro, true
	case OrgPlanBusiness:
		return SeatTierBusiness, true
	case OrgPlanEnterprise:
		return SeatTierEnterprise, true
	default:
		return "", false
	}
}

// InviteMember adds a member invitation (active membership with optional seat).
func (s *Store) InviteMember(ctx context.Context, orgID, userID, role, seatTier string) (*Member, error) {
	role = normalizeOrgRole(role)
	seatTier, err := normalizeSeatTier(seatTier)
	if err != nil {
		seatTier = SeatTierFree
	}
	seatTier, err = s.EnsureManagerRoleSeat(ctx, orgID, role, seatTier)
	if err != nil {
		return nil, err
	}
	var m Member
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO org_members (org_id, user_id, role, seat_tier, status)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (org_id, user_id) DO UPDATE SET
			role = EXCLUDED.role, seat_tier = EXCLUDED.seat_tier,
			status = CASE WHEN org_members.status = 'removed' THEN EXCLUDED.status ELSE org_members.status END,
			updated_at = now()
		 RETURNING id, user_id, role, seat_tier, status, created_at, updated_at`,
		orgID, userID, role, seatTier, MemberStatusInvited).
		Scan(&m.ID, &m.UserID, &m.Role, &m.SeatTier, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetMemberByID returns a member by membership id.
func (s *Store) GetMemberByID(ctx context.Context, orgID, memberID string) (*Member, error) {
	var m Member
	err := s.db.QueryRowContext(ctx,
		`SELECT m.id, m.user_id, COALESCE(u.wallet_address,''), m.role, m.seat_tier, m.status,
		        m.created_at, m.updated_at
		 FROM org_members m JOIN users u ON u.id = m.user_id
		 WHERE m.org_id=$1 AND m.id=$2`, orgID, memberID).
		Scan(&m.ID, &m.UserID, &m.WalletAddress, &m.Role, &m.SeatTier, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &m, err
}

// PatchMember updates a member's role or seat tier by membership id.
func (s *Store) PatchMember(ctx context.Context, orgID, memberID, role, seatTier string) (*Member, error) {
	cur, err := s.GetMemberByID(ctx, orgID, memberID)
	if err != nil {
		return nil, err
	}
	if cur.Role == OrgRoleOwner {
		return nil, fmt.Errorf("cannot change owner role here")
	}
	if role != "" {
		cur.Role = normalizeOrgRole(role)
	}
	if seatTier != "" {
		cur.SeatTier, err = normalizeSeatTier(seatTier)
		if err != nil {
			return nil, err
		}
	}
	cur.SeatTier, err = s.EnsureManagerRoleSeat(ctx, orgID, cur.Role, cur.SeatTier)
	if err != nil {
		return nil, err
	}
	return s.scanMemberRow(s.db.QueryRowContext(ctx,
		`UPDATE org_members SET role=$3, seat_tier=$4, updated_at=now()
		 WHERE org_id=$1 AND id=$2 AND role <> 'owner'
		 RETURNING id, user_id, role, seat_tier, status, created_at, updated_at`,
		orgID, memberID, cur.Role, cur.SeatTier))
}

func (s *Store) scanMemberRow(sc interface{ Scan(...any) error }) (*Member, error) {
	var m Member
	err := sc.Scan(&m.ID, &m.UserID, &m.Role, &m.SeatTier, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	return &m, err
}

// RemoveMemberByID removes a member by membership id.
func (s *Store) RemoveMemberByID(ctx context.Context, orgID, memberID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE org_members SET status=$3, updated_at=now()
		 WHERE org_id=$1 AND id=$2 AND role <> 'owner' AND status='active'`,
		orgID, memberID, MemberStatusRemoved)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AddMember adds or updates a member's role.
func (s *Store) AddMember(ctx context.Context, orgID, userID, role string) error {
	role = normalizeOrgRole(role)
	var curTier string
	_ = s.db.QueryRowContext(ctx,
		`SELECT seat_tier FROM org_members WHERE org_id=$1 AND user_id=$2 AND status='active'`,
		orgID, userID).Scan(&curTier)
	seatTier := curTier
	if seatTier == "" {
		seatTier = SeatTierFree
	}
	var err error
	seatTier, err = s.EnsureManagerRoleSeat(ctx, orgID, role, seatTier)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO org_members (org_id, user_id, role, seat_tier, status)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (org_id, user_id) DO UPDATE SET
			role = EXCLUDED.role, seat_tier = EXCLUDED.seat_tier, status = 'active', updated_at = now()`,
		orgID, userID, role, seatTier, MemberStatusActive)
	return err
}

// RemoveMember removes a user from an org (not the owner).
func (s *Store) RemoveMember(ctx context.Context, orgID, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE org_members SET status=$3, updated_at=now()
		 WHERE org_id=$1 AND user_id=$2 AND role <> 'owner' AND status='active'`,
		orgID, userID, MemberStatusRemoved)
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
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2 AND status='active'`,
		orgID, fromUserID).Scan(&fromRole)
	if err != nil || fromRole != OrgRoleOwner {
		return ErrNotFound
	}
	var toRole string
	err = tx.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2 AND status='active'`,
		orgID, toUserID).Scan(&toRole)
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
		`UPDATE org_members SET role='admin', updated_at=now() WHERE org_id=$1 AND user_id=$2`,
		orgID, fromUserID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_members SET role='owner', updated_at=now() WHERE org_id=$1 AND user_id=$2`,
		orgID, toUserID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE orgs SET owner_user_id=$2, updated_at=now() WHERE id=$1`, orgID, toUserID); err != nil {
		return err
	}
	return tx.Commit()
}

// ListMembers returns an org's active and invited members.
func (s *Store) ListMembers(ctx context.Context, orgID string) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.user_id, COALESCE(u.wallet_address,''), COALESCE(u.email,''), COALESCE(u.name,''),
		        m.role, m.seat_tier, m.status, m.created_at, m.updated_at
		 FROM org_members m JOIN users u ON u.id = m.user_id
		 WHERE m.org_id = $1 AND m.status IN ('active', 'invited') ORDER BY m.created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.UserID, &m.WalletAddress, &m.Email, &m.Name, &m.Role, &m.SeatTier, &m.Status,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
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

// SetOrgVerificationStatus sets the platform verification status (admin).
func (s *Store) SetOrgVerificationStatus(ctx context.Context, orgID, status string) error {
	status = strings.TrimSpace(status)
	if status != OrgVerificationVerified && status != OrgVerificationUnverified && status != OrgVerificationRejected {
		return fmt.Errorf("invalid verification status")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE orgs SET verification_status=$2, updated_at=now() WHERE id=$1`, orgID, status)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

var slugSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Store) resolveOrgSlug(ctx context.Context, requested, name, excludeOrgID string) (string, error) {
	base := strings.TrimSpace(requested)
	if base == "" {
		base = slugSanitizer.ReplaceAllString(strings.ToLower(name), "-")
		base = strings.Trim(base, "-")
	}
	if base == "" {
		base = "org"
	}
	base = strings.Trim(slugSanitizer.ReplaceAllString(strings.ToLower(base), "-"), "-")
	if base == "" {
		base = "org"
	}
	slug := base
	for i := 0; i < 5; i++ {
		var exists bool
		var err error
		if excludeOrgID != "" {
			err = s.db.QueryRowContext(ctx,
				`SELECT EXISTS(SELECT 1 FROM orgs WHERE slug=$1 AND id<>$2)`, slug, excludeOrgID).Scan(&exists)
		} else {
			err = s.db.QueryRowContext(ctx,
				`SELECT EXISTS(SELECT 1 FROM orgs WHERE slug=$1)`, slug).Scan(&exists)
		}
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		if excludeOrgID != "" {
			return "", fmt.Errorf("slug already taken")
		}
		slug = base + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	}
	return "", fmt.Errorf("could not allocate unique slug")
}

func normalizeOrgRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case OrgRoleOwner:
		return OrgRoleOwner
	case OrgRoleAdmin:
		return OrgRoleAdmin
	case OrgRoleNodeOperator:
		return OrgRoleNodeOperator
	case OrgRoleViewer:
		return OrgRoleViewer
	default:
		return OrgRoleMember
	}
}