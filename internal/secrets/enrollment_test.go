package secrets

import "testing"

func TestNewOrgEnrollmentSecret(t *testing.T) {
	s, err := NewOrgEnrollmentSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(s) < len(OrgEnrollmentPrefix)+16 {
		t.Fatalf("secret too short: %q", s)
	}
	if s[:len(OrgEnrollmentPrefix)] != OrgEnrollmentPrefix {
		t.Fatalf("prefix = %q", s)
	}
}

func TestNewNodeKey(t *testing.T) {
	k, err := NewNodeKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(k) < 20 {
		t.Fatalf("node key too short: %q", k)
	}
}