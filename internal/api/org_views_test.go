package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOrgSummaryVerifiedFalseEmitted(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(orgSummary{Name: "clawbrick", Kind: "team", Verified: false})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"verified":false`) {
		t.Fatalf("want verified:false in JSON, got %s", b)
	}
}