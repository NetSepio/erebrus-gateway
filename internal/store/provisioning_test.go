package store

import "testing"

func TestManagedNodeName(t *testing.T) {
	t.Parallel()
	if got := managedNodeName(DeploymentProfileSentinel); got == "" {
		t.Fatal("expected non-empty managed node name")
	}
}