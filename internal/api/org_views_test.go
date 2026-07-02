package api

import (
	"encoding/json"
	"strings"
	"testing"
)

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