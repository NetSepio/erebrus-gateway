package store

import (
	"strings"
	"testing"
)

func TestOwnerSeatTierForPlan(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		OrgPlanBasic:      SeatTierFree,
		OrgPlanStarter:    SeatTierStarter,
		OrgPlanPro:        SeatTierPro,
		OrgPlanBusiness:   SeatTierBusiness,
		OrgPlanEnterprise: SeatTierEnterprise,
		"unknown":         SeatTierFree,
	}
	for plan, want := range cases {
		if got := ownerSeatTierForPlan(plan); got != want {
			t.Fatalf("ownerSeatTierForPlan(%q) = %q, want %q", plan, got, want)
		}
	}
}

func TestPersonalOrgName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		wallet string
		want   string
	}{
		{"9SXo8wiAdsDBQPUKk4LFN73T4DcueYhPDRN3p6wTsgaR", "Workspace 9SXo8w…sgaR"},
		{"0xAbC123", "Workspace 0xAbC123"},
		{"", "Personal Workspace"},
		{"   ", "Personal Workspace"},
	}
	for _, tc := range cases {
		got := personalOrgName(tc.wallet)
		if got != tc.want {
			t.Fatalf("personalOrgName(%q) = %q, want %q", tc.wallet, got, tc.want)
		}
		if strings.TrimSpace(got) == "" {
			t.Fatalf("personalOrgName(%q) returned blank", tc.wallet)
		}
	}
}
