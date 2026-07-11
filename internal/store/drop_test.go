package store

import "testing"

func TestNormalizeDropTier(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"free":       DropTierFree,
		"starter":    DropTierStarter,
		"pro":        DropTierPro,
		"business":   DropTierBusiness,
		"enterprise": DropTierEnterprise,
		"":           DropTierFree,
		"bogus":      DropTierFree,
	}
	for in, want := range cases {
		if got := NormalizeDropTier(in); got != want {
			t.Fatalf("NormalizeDropTier(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDropTierRankOrdering(t *testing.T) {
	t.Parallel()
	order := []string{DropTierFree, DropTierStarter, DropTierPro, DropTierBusiness, DropTierEnterprise}
	for i := 1; i < len(order); i++ {
		if dropTierRank(order[i-1]) >= dropTierRank(order[i]) {
			t.Fatalf("rank(%s) should be < rank(%s)", order[i-1], order[i])
		}
	}
	if dropTierRank("nope") != -1 {
		t.Fatal("unknown tier should rank -1")
	}
}

func TestDefaultDropQuotaBytes(t *testing.T) {
	t.Parallel()
	cases := map[string]int64{
		DropTierFree:       500_000_000,
		DropTierStarter:    1_000_000_000,
		DropTierPro:        5_000_000_000,
		DropTierBusiness:   10_000_000_000,
		DropTierEnterprise: 10_000_000_000,
		"unknown":          500_000_000,
	}
	for tier, want := range cases {
		if got := DefaultDropQuotaBytes(tier); got != want {
			t.Fatalf("DefaultDropQuotaBytes(%q) = %d, want %d", tier, got, want)
		}
	}
}

func TestDropSupportedOnAllProfiles(t *testing.T) {
	t.Parallel()
	for _, profile := range []string{DeploymentProfileStandard, DeploymentProfileShield, DeploymentProfileSentinel} {
		if !DeploymentProfileAllowsService(profile, ServiceTypeDrop) {
			t.Fatalf("profile %q should allow Drop", profile)
		}
	}
	if DeploymentProfileAllowsService("nonexistent", ServiceTypeDrop) {
		t.Fatal("unknown profile must not allow Drop")
	}
}
