package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PlanEntitlementTemplate is the entitlement row seeded when an org is assigned a plan.
type PlanEntitlementTemplate struct {
	Plan                       string
	PaidSeatsIncluded          int
	ManagedVPNNodesIncluded    int
	ShieldInstancesIncluded    int
	SentinelLicensesIncluded   int
	PublicNodeAccessTier       string
	SupportTier                string
	AuditLogsEnabled           bool
	AdvancedAnalyticsEnabled   bool
}

// PlanEntitlementTemplates returns the canonical plan → entitlement mapping.
func PlanEntitlementTemplates() map[string]PlanEntitlementTemplate {
	return map[string]PlanEntitlementTemplate{
		OrgPlanBasic: {
			Plan: OrgPlanBasic, PublicNodeAccessTier: "free", SupportTier: "community",
		},
		OrgPlanStarter: {
			Plan: OrgPlanStarter, PaidSeatsIncluded: 1, PublicNodeAccessTier: "starter",
			SupportTier: "standard",
		},
		OrgPlanPro: {
			Plan: OrgPlanPro, PaidSeatsIncluded: 5, ManagedVPNNodesIncluded: 1,
			ShieldInstancesIncluded: 1, PublicNodeAccessTier: "pro", SupportTier: "standard",
		},
		OrgPlanBusiness: {
			Plan: OrgPlanBusiness, PaidSeatsIncluded: 25, ManagedVPNNodesIncluded: 3,
			SentinelLicensesIncluded: 3, PublicNodeAccessTier: "business", SupportTier: "priority",
			AuditLogsEnabled: true, AdvancedAnalyticsEnabled: true,
		},
		OrgPlanEnterprise: {
			Plan: OrgPlanEnterprise, SupportTier: "enterprise",
			AuditLogsEnabled: true, AdvancedAnalyticsEnabled: true,
		},
	}
}

func normalizeOrgPlan(plan string) (string, error) {
	switch plan {
	case OrgPlanBasic, OrgPlanStarter, OrgPlanPro, OrgPlanBusiness, OrgPlanEnterprise:
		return plan, nil
	default:
		return "", fmt.Errorf("invalid plan: %s", plan)
	}
}

func normalizeSeatTier(tier string) (string, error) {
	switch tier {
	case SeatTierFree, SeatTierStarter, SeatTierPro, SeatTierBusiness, SeatTierEnterprise:
		return tier, nil
	default:
		return "", fmt.Errorf("invalid seat tier: %s", tier)
	}
}

func seatTierRank(tier string) int {
	switch tier {
	case SeatTierFree:
		return 0
	case SeatTierStarter:
		return 1
	case SeatTierPro:
		return 2
	case SeatTierBusiness:
		return 3
	case SeatTierEnterprise:
		return 4
	default:
		return -1
	}
}

func maxSeatTierForPlan(plan string) string {
	switch plan {
	case OrgPlanStarter:
		return SeatTierStarter
	case OrgPlanPro:
		return SeatTierPro
	case OrgPlanBusiness:
		return SeatTierBusiness
	case OrgPlanEnterprise:
		return SeatTierEnterprise
	default:
		return SeatTierFree
	}
}

// SeatTierAllowedForPlan reports whether a seat tier is valid for an org plan.
func SeatTierAllowedForPlan(plan, seatTier string) bool {
	if seatTier == SeatTierFree {
		return true
	}
	return seatTierRank(seatTier) > 0 && seatTierRank(seatTier) <= seatTierRank(maxSeatTierForPlan(plan))
}

// CountShieldInstancesUsed returns active Shield services across the org.
func (s *Store) CountShieldInstancesUsed(ctx context.Context, orgID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM org_node_services
		 WHERE org_id=$1 AND service_type=$2 AND service_status <> $3`,
		orgID, ServiceTypeCommunityFirewall, ServiceStatusDisabled).Scan(&n)
	return n, err
}

const entitlementCols = `id, org_id, plan, paid_seats_included, managed_vpn_nodes_included,
	shield_instances_included, sentinel_licenses_included,
	COALESCE(public_node_access_tier,''), api_quota_monthly, COALESCE(bandwidth_policy,''),
	COALESCE(support_tier,''), audit_logs_enabled, advanced_analytics_enabled,
	created_at, updated_at`

func scanEntitlement(sc interface{ Scan(...any) error }) (*OrgEntitlement, error) {
	var e OrgEntitlement
	var apiQuota sql.NullInt64
	err := sc.Scan(
		&e.ID, &e.OrgID, &e.Plan, &e.PaidSeatsIncluded, &e.ManagedVPNNodesIncluded,
		&e.ShieldInstancesIncluded, &e.SentinelLicensesIncluded,
		&e.PublicNodeAccessTier, &apiQuota, &e.BandwidthPolicy, &e.SupportTier,
		&e.AuditLogsEnabled, &e.AdvancedAnalyticsEnabled, &e.CreatedAt, &e.UpdatedAt,
	)
	if apiQuota.Valid {
		v := int(apiQuota.Int64)
		e.APIQuotaMonthly = &v
	}
	return &e, err
}

// UpsertOrgEntitlements seeds or updates entitlement limits for an org plan.
func (s *Store) UpsertOrgEntitlements(ctx context.Context, orgID, plan string) (*OrgEntitlement, error) {
	plan, err := normalizeOrgPlan(plan)
	if err != nil {
		return nil, err
	}
	tpl, ok := PlanEntitlementTemplates()[plan]
	if !ok {
		return nil, fmt.Errorf("no entitlement template for plan %s", plan)
	}
	return scanEntitlement(s.db.QueryRowContext(ctx,
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
			updated_at = now()
		RETURNING `+entitlementCols,
		orgID, tpl.Plan, tpl.PaidSeatsIncluded, tpl.ManagedVPNNodesIncluded,
		tpl.ShieldInstancesIncluded, tpl.SentinelLicensesIncluded,
		tpl.PublicNodeAccessTier, tpl.SupportTier, tpl.AuditLogsEnabled, tpl.AdvancedAnalyticsEnabled,
	))
}

// GetOrgEntitlements returns the entitlement row for an org.
func (s *Store) GetOrgEntitlements(ctx context.Context, orgID string) (*OrgEntitlement, error) {
	e, err := scanEntitlement(s.db.QueryRowContext(ctx,
		`SELECT `+entitlementCols+` FROM org_entitlements WHERE org_id=$1`, orgID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

// CountPaidSeatsUsed returns members with a non-free seat tier.
func (s *Store) CountPaidSeatsUsed(ctx context.Context, orgID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM org_members
		 WHERE org_id=$1 AND status='active' AND seat_tier <> 'free'`, orgID).Scan(&n)
	return n, err
}