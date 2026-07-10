package store

import (
	"context"
	"encoding/json"
	"strings"
)

// FirewallSyncRule is one rule in a node-bound policy sync payload.
type FirewallSyncRule struct {
	RuleType string `json:"rule_type"`
	Target   string `json:"target"`
	Action   string `json:"action"`
	Value    string `json:"value,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// FirewallSyncPayload is sent to nodes via sync_firewall WS command.
type FirewallSyncPayload struct {
	OrgID        string             `json:"org_id"`
	NodeID       string             `json:"node_id"`
	ServiceKind  string             `json:"service_kind"` // shield | sentinel
	Rules        []FirewallSyncRule `json:"rules"`
	Upstreams    []string           `json:"upstreams"`
	Licensed     bool               `json:"licensed"`
	ShieldAdmin  string             `json:"shield_admin_url,omitempty"`
}

// BuildFirewallSyncPayload maps gateway firewall rules to a node sync payload.
func BuildFirewallSyncPayload(orgID, nodeID string, fw *FirewallService, rules []FirewallRule) FirewallSyncPayload {
	out := FirewallSyncPayload{
		OrgID:       orgID,
		NodeID:      nodeID,
		ServiceKind: fw.ServiceKind,
		Licensed:    fw.Service.ServiceStatus != ServiceStatusUnlicensed,
	}
	if fw.ServiceKind == "shield" && fw.Service.AccessURL != "" {
		out.ShieldAdmin = fw.Service.AccessURL
	}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		sr := FirewallSyncRule{RuleType: r.RuleType, Target: r.Target, Action: r.Action, Enabled: true}
		switch r.RuleType {
		case FirewallRuleDNSRewrite:
			sr.Action = "rewrite"
			sr.Value = strings.TrimSpace(r.Action)
		case FirewallRuleUpstreamResolverConfig:
			out.Upstreams = append(out.Upstreams, strings.TrimSpace(r.Target))
			continue
		case FirewallRuleDomainBlock, FirewallRuleWildcardDomainBlock:
			sr.Action = "block"
		case FirewallRuleDomainAllow:
			sr.Action = "allow"
		default:
			continue
		}
		out.Rules = append(out.Rules, sr)
	}
	return out
}

// MarshalFirewallSyncPayload JSON-encodes a sync payload for WS command args.
func MarshalFirewallSyncPayload(p FirewallSyncPayload) (json.RawMessage, error) {
	return json.Marshal(p)
}

// ServiceHealthFromNode maps node-reported service keys to gateway service_status.
func ServiceHealthFromNode(reported string) string {
	switch strings.ToLower(strings.TrimSpace(reported)) {
	case "active":
		return ServiceStatusActive
	case "degraded":
		return ServiceStatusDegraded
	case "unreachable", "error":
		return ServiceStatusError
	case "unlicensed":
		return ServiceStatusUnlicensed
	default:
		return ""
	}
}

// UpdateNodeServicesFromReport patches org_node_services status from heartbeat/hello maps.
func (s *Store) UpdateNodeServicesFromReport(ctx context.Context, peerID string, services map[string]string) error {
	if len(services) == 0 {
		return nil
	}
	orgID, err := s.OrgIDForNode(ctx, peerID)
	if err != nil {
		return err
	}
	type mapping struct {
		key         string
		serviceType string
	}
	mappings := []mapping{
		{"vpn", ServiceTypeVPN},
		{"community_firewall", ServiceTypeCommunityFirewall},
		{"erebrus_firewall", ServiceTypeErebrusFirewall},
		{"shield", ServiceTypeCommunityFirewall},
		{"sentinel", ServiceTypeErebrusFirewall},
	}
	for _, m := range mappings {
		raw, ok := services[m.key]
		if !ok {
			continue
		}
		status := ServiceHealthFromNode(raw)
		if status == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE org_node_services SET service_status=$4, updated_at=now()
			 WHERE org_id=$1 AND node_id=$2 AND service_type=$3`,
			orgID, peerID, m.serviceType, status); err != nil {
			return err
		}
	}
	return nil
}

// OrgIDForNode returns the org_id linked to a runtime peer_id.
func (s *Store) OrgIDForNode(ctx context.Context, peerID string) (string, error) {
	var orgID string
	err := s.db.QueryRowContext(ctx,
		`SELECT org_id::text FROM org_nodes WHERE node_id=$1 LIMIT 1`, peerID).Scan(&orgID)
	if err != nil {
		return "", err
	}
	return orgID, nil
}

// UpdateOrgNodeDeploymentProfile sets deployment_profile when a node reports it in hello.
func (s *Store) UpdateOrgNodeDeploymentProfile(ctx context.Context, peerID, profile string) error {
	profile = NormalizeDeploymentProfile(profile)
	if profile == DeploymentProfileStandard {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE org_nodes SET deployment_profile=$2, updated_at=now() WHERE node_id=$1`,
		peerID, profile)
	return err
}