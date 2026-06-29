package api

import (
	"context"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

// orgSummary is the org projection returned alongside nodes and org resources.
// org_id is omitted unless the caller is org owner/admin or platform admin.
type orgSummary struct {
	ID                   string `json:"id,omitempty"`
	Name                 string `json:"name"`
	Slug                 string `json:"slug,omitempty"`
	Plan                 string `json:"plan,omitempty"`
	VerificationStatus   string `json:"verification_status"`
	PublicProfileEnabled bool   `json:"public_profile_enabled"`
	DisplayName          string `json:"display_name,omitempty"`
	Description          string `json:"description,omitempty"`
	WebsiteURL           string `json:"website_url,omitempty"`
}

func (s *Server) orgSummaryFor(ctx context.Context, orgID, callerRole string, privileged bool) *orgSummary {
	if orgID == "" {
		return nil
	}
	org, err := s.store.GetOrg(ctx, orgID)
	if err != nil {
		return nil
	}
	out := &orgSummary{
		Name: org.Name, Slug: org.Slug, Plan: org.Plan,
		VerificationStatus: org.VerificationStatus, PublicProfileEnabled: org.PublicProfileEnabled,
	}
	if profile, err := s.store.GetOrgProfile(ctx, orgID); err == nil {
		out.DisplayName = profile.DisplayName
		out.Description = profile.Description
		out.WebsiteURL = profile.WebsiteURL
	}
	if privileged || store.IsOrgPrivileged(callerRole) {
		out.ID = org.ID
	}
	return out
}

func orgResponse(org *store.Org, privileged bool) gin.H {
	if org == nil {
		return gin.H{}
	}
	out := gin.H{
		"name":                   org.Name,
		"slug":                   org.Slug,
		"plan":                   org.Plan,
		"billing_status":         org.BillingStatus,
		"verification_status":    org.VerificationStatus,
		"public_profile_enabled": org.PublicProfileEnabled,
		"created_at":             org.CreatedAt,
		"updated_at":             org.UpdatedAt,
	}
	if privileged {
		out["id"] = org.ID
		out["owner_user_id"] = org.OwnerUserID
	}
	if org.Role != "" {
		out["role"] = org.Role
	}
	return out
}

func orgProfileResponse(p *store.OrgProfile) gin.H {
	if p == nil {
		return gin.H{}
	}
	out := gin.H{
		"org_id":     p.OrgID,
		"created_at": p.CreatedAt,
		"updated_at": p.UpdatedAt,
	}
	setIfNonEmpty(out, "legal_name", p.LegalName)
	setIfNonEmpty(out, "display_name", p.DisplayName)
	setIfNonEmpty(out, "description", p.Description)
	setIfNonEmpty(out, "logo_url", p.LogoURL)
	setIfNonEmpty(out, "website_url", p.WebsiteURL)
	setIfNonEmpty(out, "public_email", p.PublicEmail)
	setIfNonEmpty(out, "billing_email", p.BillingEmail)
	setIfNonEmpty(out, "support_email", p.SupportEmail)
	setIfNonEmpty(out, "country", p.Country)
	setIfNonEmpty(out, "timezone", p.Timezone)
	return out
}

func entitlementResponse(e *store.OrgEntitlement) gin.H {
	if e == nil {
		return gin.H{}
	}
	out := gin.H{
		"org_id":                       e.OrgID,
		"plan":                         e.Plan,
		"paid_seats_included":          e.PaidSeatsIncluded,
		"managed_vpn_nodes_included":   e.ManagedVPNNodesIncluded,
		"shield_instances_included":    e.ShieldInstancesIncluded,
		"sentinel_licenses_included":   e.SentinelLicensesIncluded,
		"audit_logs_enabled":           e.AuditLogsEnabled,
		"advanced_analytics_enabled":   e.AdvancedAnalyticsEnabled,
		"created_at":                   e.CreatedAt,
		"updated_at":                   e.UpdatedAt,
	}
	setIfNonEmpty(out, "public_node_access_tier", e.PublicNodeAccessTier)
	setIfNonEmpty(out, "bandwidth_policy", e.BandwidthPolicy)
	setIfNonEmpty(out, "support_tier", e.SupportTier)
	if e.APIQuotaMonthly != nil {
		out["api_quota_monthly"] = *e.APIQuotaMonthly
	}
	return out
}

func setIfNonEmpty(m gin.H, key, val string) {
	if val != "" {
		m[key] = val
	}
}