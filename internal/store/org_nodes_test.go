package store

import "testing"

func TestDeploymentProfileAllowsService(t *testing.T) {
	t.Parallel()
	cases := []struct {
		profile string
		svc     string
		want    bool
	}{
		{DeploymentProfileErebrus, ServiceTypeVPN, true},
		{DeploymentProfileErebrus, ServiceTypeCommunityFirewall, false},
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