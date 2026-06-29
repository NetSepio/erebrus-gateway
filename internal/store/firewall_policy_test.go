package store

import "testing"

func TestBuildFirewallSyncPayload(t *testing.T) {
	fw := &FirewallService{
		ServiceKind: "sentinel",
		Service: OrgNodeService{ServiceStatus: ServiceStatusActive},
	}
	rules := []FirewallRule{
		{RuleType: FirewallRuleDomainBlock, Target: "ads.example.com", Enabled: true},
		{RuleType: FirewallRuleDNSRewrite, Target: "home.lan", Action: "10.0.0.5", Enabled: true},
		{RuleType: FirewallRuleUpstreamResolverConfig, Target: "1.1.1.1", Enabled: true},
	}
	p := BuildFirewallSyncPayload("org", "node", fw, rules)
	if len(p.Rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(p.Rules))
	}
	if len(p.Upstreams) != 1 || p.Upstreams[0] != "1.1.1.1" {
		t.Fatalf("upstreams = %v", p.Upstreams)
	}
	if !p.Licensed {
		t.Fatal("expected licensed")
	}
}

func TestServiceHealthFromNode(t *testing.T) {
	if got := ServiceHealthFromNode("active"); got != ServiceStatusActive {
		t.Fatalf("got %q", got)
	}
	if got := ServiceHealthFromNode("unlicensed"); got != ServiceStatusUnlicensed {
		t.Fatalf("got %q", got)
	}
}