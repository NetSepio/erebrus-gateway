package store

import "testing"

func TestPlanEntitlementTemplates(t *testing.T) {
	t.Parallel()
	templates := PlanEntitlementTemplates()

	pro := templates[OrgPlanPro]
	if pro.PaidSeatsIncluded != 5 {
		t.Fatalf("pro paid seats = %d, want 5", pro.PaidSeatsIncluded)
	}
	if pro.ManagedVPNNodesIncluded != 1 {
		t.Fatalf("pro managed nodes = %d, want 1", pro.ManagedVPNNodesIncluded)
	}
	if pro.ShieldInstancesIncluded != 1 {
		t.Fatalf("pro shield instances = %d, want 1", pro.ShieldInstancesIncluded)
	}

	business := templates[OrgPlanBusiness]
	if business.PaidSeatsIncluded != 25 {
		t.Fatalf("business paid seats = %d, want 25", business.PaidSeatsIncluded)
	}
	if business.SentinelLicensesIncluded != 3 {
		t.Fatalf("business sentinel licenses = %d, want 3", business.SentinelLicensesIncluded)
	}
	if !business.AuditLogsEnabled || !business.AdvancedAnalyticsEnabled {
		t.Fatal("business plan should enable audit logs and advanced analytics")
	}
}

func TestNormalizeOrgPlan(t *testing.T) {
	t.Parallel()
	if _, err := normalizeOrgPlan("invalid"); err == nil {
		t.Fatal("expected error for invalid plan")
	}
	for _, plan := range []string{OrgPlanBasic, OrgPlanStarter, OrgPlanPro, OrgPlanBusiness, OrgPlanEnterprise} {
		got, err := normalizeOrgPlan(plan)
		if err != nil || got != plan {
			t.Fatalf("normalizeOrgPlan(%q) = %q, %v", plan, got, err)
		}
	}
}