package store

import "testing"

func TestRuntimeToOrgNodeStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		runtime string
		want    string
	}{
		{"online", OrgNodeStatusActive},
		{"draining", OrgNodeStatusDegraded},
		{"offline", OrgNodeStatusDegraded},
		{"", OrgNodeStatusDegraded},
	}
	for _, tc := range cases {
		if got := RuntimeToOrgNodeStatus(tc.runtime); got != tc.want {
			t.Fatalf("runtime=%q got %q want %q", tc.runtime, got, tc.want)
		}
	}
}

func TestNormalizeDeploymentProfile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"standard", DeploymentProfileStandard},
		{"shield", DeploymentProfileShield},
		{"sentinel", DeploymentProfileSentinel},
		{"", DeploymentProfileStandard},
		{"unknown", DeploymentProfileStandard},
	}
	for _, tc := range cases {
		if got := NormalizeDeploymentProfile(tc.in); got != tc.want {
			t.Fatalf("in=%q got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestDeploymentProfileAllowsService(t *testing.T) {
	t.Parallel()
	cases := []struct {
		profile string
		svc     string
		want    bool
	}{
		{DeploymentProfileStandard, ServiceTypeVPN, true},
		{DeploymentProfileStandard, ServiceTypeCommunityFirewall, false},
		{DeploymentProfileShield, ServiceTypeCommunityFirewall, true},
		{DeploymentProfileSentinel, ServiceTypeErebrusFirewall, true},
		{DeploymentProfileSentinel, ServiceTypeCommunityFirewall, false},
	}
	for _, tc := range cases {
		got := DeploymentProfileAllowsService(tc.profile, tc.svc)
		if got != tc.want {
			t.Fatalf("profile=%s svc=%s got %v want %v", tc.profile, tc.svc, got, tc.want)
		}
	}
}
