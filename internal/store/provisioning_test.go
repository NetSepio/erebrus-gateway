package store

import (
	"context"
	"testing"
)

func TestManagedNodeName(t *testing.T) {
	t.Parallel()
	if got := managedNodeName(DeploymentProfileSentinel); got == "" {
		t.Fatal("expected non-empty managed node name")
	}
}

func TestProvisionIncludedPlanResourcesSkipsWhenDisabled(t *testing.T) {
	t.Parallel()
	s := &Store{}
	if err := s.ProvisionIncludedPlanResources(context.Background(), "org-id", OrgPlanPro, ProvisioningConfig{Enabled: false}); err != nil {
		t.Fatalf("disabled provisioning: %v", err)
	}
}