package store

import (
	"strings"
	"testing"
)

// TestMigrationsEmbedded guards the migration runner's contract without a DB:
// the embedded FS must contain the expected files, in sorted order, non-empty.
// migrate() applies them in lexical order, so a missing or misnamed file (or a
// dropped //go:embed) would silently skip schema — catch it here.
func TestMigrationsEmbedded(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}

	want := []string{"0001_init.sql", "0002_orgs_entitlement.sql", "0003_email_auth.sql", "0004_node_metrics.sql", "0005_referrals_xp.sql", "0006_xp_claims.sql", "0007_social_perks.sql", "0008_activity_log.sql"}
	if len(names) != len(want) {
		t.Fatalf("migration files = %v, want %v", names, want)
	}
	for i, w := range want {
		if names[i] != w {
			t.Fatalf("migration[%d] = %q, want %q (order matters)", i, names[i], w)
		}
		body, err := migrationsFS.ReadFile("migrations/" + names[i])
		if err != nil || len(strings.TrimSpace(string(body))) == 0 {
			t.Fatalf("migration %q empty or unreadable: %v", names[i], err)
		}
	}

	// S2 schema must actually be in 0003.
	body, _ := migrationsFS.ReadFile("migrations/0003_email_auth.sql")
	for _, token := range []string{"email_verified", "email_otps", "code_hash"} {
		if !strings.Contains(string(body), token) {
			t.Fatalf("0003_email_auth.sql missing %q", token)
		}
	}
}
