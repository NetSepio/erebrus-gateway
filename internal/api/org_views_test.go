package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/NetSepio/gateway/internal/store"
)

func TestOrgResponseIncludesIDForMembers(t *testing.T) {
	t.Parallel()
	out := orgResponse(&store.Org{ID: "org-1", Name: "Acme", Role: store.OrgRoleNodeOperator}, false)
	if out["id"] != "org-1" {
		t.Fatalf("member org response id = %v, want org-1", out["id"])
	}
	if _, ok := out["owner_user_id"]; ok {
		t.Fatal("owner_user_id should be omitted for non-privileged member")
	}
}

func TestOrgResponseOmitsIDWithoutMembership(t *testing.T) {
	t.Parallel()
	out := orgResponse(&store.Org{ID: "org-1", Name: "Acme"}, false)
	if _, ok := out["id"]; ok {
		t.Fatalf("public org projection should omit id, got %v", out["id"])
	}
}

func TestOrgSummaryVerificationStatusEmitted(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(orgSummary{Name: "clawbrick", VerificationStatus: "unverified"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"verification_status":"unverified"`) {
		t.Fatalf("want verification_status in JSON, got %s", b)
	}
}