package store

import "testing"

func TestCoarseCapacity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		max, repo, reserved int64
		want                string
	}{
		{0, 0, 0, DropCapacityUnknown},        // no advertised bound
		{1000, 1000, 0, DropCapacityFull},     // exhausted
		{1000, 950, 0, DropCapacityLimited},   // <10% left
		{1000, 100, 0, DropCapacityAvailable}, // plenty
		{1000, 500, 450, DropCapacityLimited}, // reservations count
		{1000, 900, 200, DropCapacityFull},    // over-committed clamps to full
	}
	for _, c := range cases {
		if got := coarseCapacity(c.max, c.repo, c.reserved); got != c.want {
			t.Errorf("coarseCapacity(%d,%d,%d)=%q want %q", c.max, c.repo, c.reserved, got, c.want)
		}
	}
}

func TestNormalizePublicGatewayURL(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                             "",
		"   ":                          "",
		"https://drop-sg1.erebrus.io":  "https://drop-sg1.erebrus.io",
		"https://drop-sg1.erebrus.io/": "https://drop-sg1.erebrus.io",
		"https://Node-SG-1.Erebrus.IO/ipfs/x?a=b#c": "https://node-sg-1.erebrus.io",
		"  https://drop.erebrus.io  ":               "https://drop.erebrus.io",
		"http://203.0.113.10:8080":                  "http://203.0.113.10:8080",
		"HTTPS://Drop.Erebrus.io":                   "https://drop.erebrus.io",
		"http://node:5001":                          "", // Kubo RPC port rejected
		"https://user:pass@drop.io":                 "", // credentials rejected
		"ftp://drop.erebrus.io":                     "", // non-http(s) rejected
		"https://":                                  "", // missing host
		"not a url":                                 "", // no scheme/host
		"/ipfs/x":                                   "", // relative
	}
	for in, want := range cases {
		if got := NormalizePublicGatewayURL(in); got != want {
			t.Errorf("NormalizePublicGatewayURL(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizeDropState(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"active":      DropStateActive,
		"DEGRADED":    DropStateDegraded,
		"full":        DropStateFull,
		"unreachable": DropStateUnreachable,
		"starting":    DropStateStarting,
		"":            DropStateDisabled,
		"nonsense":    DropStateDisabled,
	}
	for in, want := range cases {
		if got := NormalizeDropState(in); got != want {
			t.Errorf("NormalizeDropState(%q)=%q want %q", in, got, want)
		}
	}
}

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
