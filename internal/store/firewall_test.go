package store

import "testing"

func TestValidateFirewallRuleType(t *testing.T) {
	t.Parallel()
	if err := validateFirewallRuleType(FirewallRuleDomainAllow); err != nil {
		t.Fatalf("expected valid rule type: %v", err)
	}
	if err := validateFirewallRuleType("invalid"); err == nil {
		t.Fatal("expected error for invalid rule type")
	}
}