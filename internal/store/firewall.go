package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Firewall rule types (Sentinel v1).
const (
	FirewallRuleDomainAllow            = "domain_allow"
	FirewallRuleDomainBlock            = "domain_block"
	FirewallRuleWildcardDomainBlock    = "wildcard_domain_block"
	FirewallRuleDNSRewrite             = "dns_rewrite"
	FirewallRuleUpstreamResolverConfig = "upstream_resolver_config"
)

// FirewallService is the abstracted firewall capability on a node.
type FirewallService struct {
	Service     OrgNodeService `json:"service"`
	ServiceKind string         `json:"service_kind"` // shield | sentinel
}

const firewallRuleCols = `id, org_id, node_id, firewall_service_id, rule_type, target, action, scope,
	enabled, created_by, created_at, updated_at`

// FirewallRule is a gateway-managed Sentinel policy rule.
type FirewallRule struct {
	ID                string    `json:"id"`
	OrgID             string    `json:"org_id"`
	NodeID            string    `json:"node_id"`
	FirewallServiceID string    `json:"firewall_service_id"`
	RuleType          string    `json:"rule_type"`
	Target            string    `json:"target"`
	Action            string    `json:"action"`
	Scope             string    `json:"scope"`
	Enabled           bool      `json:"enabled"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func scanFirewallRule(sc interface{ Scan(...any) error }) (*FirewallRule, error) {
	var r FirewallRule
	err := sc.Scan(
		&r.ID, &r.OrgID, &r.NodeID, &r.FirewallServiceID, &r.RuleType, &r.Target, &r.Action, &r.Scope,
		&r.Enabled, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	return &r, err
}

// GetFirewallService returns the node's firewall service (Shield or Sentinel).
func (s *Store) GetFirewallService(ctx context.Context, orgID, nodeID string) (*FirewallService, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+orgNodeServiceCols+` FROM org_node_services
		 WHERE org_id=$1 AND node_id=$2
		   AND service_type IN ($3, $4)
		   AND service_status <> $5
		 ORDER BY created_at LIMIT 1`,
		orgID, nodeID, ServiceTypeCommunityFirewall, ServiceTypeErebrusFirewall, ServiceStatusDisabled)
	svc, err := scanOrgNodeService(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	kind := "shield"
	if svc.ServiceType == ServiceTypeErebrusFirewall {
		kind = "sentinel"
	}
	return &FirewallService{Service: *svc, ServiceKind: kind}, nil
}

// UpdateFirewallServiceStatus patches firewall service lifecycle fields.
func (s *Store) UpdateFirewallServiceStatus(ctx context.Context, orgID, nodeID, status string, configRef, accessURL *string) (*OrgNodeService, error) {
	fw, err := s.GetFirewallService(ctx, orgID, nodeID)
	if err != nil {
		return nil, err
	}
	cfg, url := fw.Service.ConfigRef, fw.Service.AccessURL
	if configRef != nil {
		cfg = strings.TrimSpace(*configRef)
	}
	if accessURL != nil {
		url = strings.TrimSpace(*accessURL)
	}
	return scanOrgNodeService(s.db.QueryRowContext(ctx,
		`UPDATE org_node_services SET service_status=$4,
		 config_ref=COALESCE(NULLIF($5,''), config_ref),
		 access_url=COALESCE(NULLIF($6,''), access_url),
		 updated_at=now()
		 WHERE id=$1 AND org_id=$2 AND node_id=$3
		 RETURNING `+orgNodeServiceCols,
		fw.Service.ID, orgID, nodeID, status, cfg, url))
}

// CreateFirewallRuleInput carries a new Sentinel rule.
type CreateFirewallRuleInput struct {
	OrgID             string
	NodeID            string
	FirewallServiceID string
	RuleType          string
	Target            string
	Action            string
	Scope             string
	Enabled           bool
	CreatedBy         string
}

// CreateFirewallRule inserts a Sentinel policy rule.
func (s *Store) CreateFirewallRule(ctx context.Context, in CreateFirewallRuleInput) (*FirewallRule, error) {
	if err := validateFirewallRuleType(in.RuleType); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Target) == "" {
		return nil, fmt.Errorf("target is required")
	}
	return scanFirewallRule(s.db.QueryRowContext(ctx,
		`INSERT INTO firewall_rules (
			org_id, node_id, firewall_service_id, rule_type, target, action, scope, enabled, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING `+firewallRuleCols,
		in.OrgID, in.NodeID, in.FirewallServiceID, in.RuleType, in.Target, in.Action, in.Scope, in.Enabled, in.CreatedBy))
}

// ListFirewallRules returns rules for a node's firewall service.
func (s *Store) ListFirewallRules(ctx context.Context, orgID, nodeID, serviceID string) ([]FirewallRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+firewallRuleCols+` FROM firewall_rules
		 WHERE org_id=$1 AND node_id=$2 AND firewall_service_id=$3
		 ORDER BY created_at`,
		orgID, nodeID, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FirewallRule
	for rows.Next() {
		r, err := scanFirewallRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// UpdateFirewallRuleInput carries patchable rule fields.
type UpdateFirewallRuleInput struct {
	Target  *string
	Action  *string
	Scope   *string
	Enabled *bool
}

// UpdateFirewallRule patches a firewall rule scoped to a node.
func (s *Store) UpdateFirewallRule(ctx context.Context, orgID, nodeID, ruleID string, in UpdateFirewallRuleInput) (*FirewallRule, error) {
	cur, err := s.getFirewallRule(ctx, orgID, nodeID, ruleID)
	if err != nil {
		return nil, err
	}
	target, action, scope, enabled := cur.Target, cur.Action, cur.Scope, cur.Enabled
	if in.Target != nil {
		target = strings.TrimSpace(*in.Target)
	}
	if in.Action != nil {
		action = strings.TrimSpace(*in.Action)
	}
	if in.Scope != nil {
		scope = strings.TrimSpace(*in.Scope)
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return scanFirewallRule(s.db.QueryRowContext(ctx,
		`UPDATE firewall_rules SET target=$4, action=$5, scope=$6, enabled=$7, updated_at=now()
		 WHERE id=$1 AND org_id=$2 AND node_id=$3
		 RETURNING `+firewallRuleCols,
		ruleID, orgID, nodeID, target, action, scope, enabled))
}

// DeleteFirewallRule removes a rule scoped to a node.
func (s *Store) DeleteFirewallRule(ctx context.Context, orgID, nodeID, ruleID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM firewall_rules WHERE id=$1 AND org_id=$2 AND node_id=$3`, ruleID, orgID, nodeID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) getFirewallRule(ctx context.Context, orgID, nodeID, ruleID string) (*FirewallRule, error) {
	r, err := scanFirewallRule(s.db.QueryRowContext(ctx,
		`SELECT `+firewallRuleCols+` FROM firewall_rules WHERE id=$1 AND org_id=$2 AND node_id=$3`, ruleID, orgID, nodeID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func validateFirewallRuleType(ruleType string) error {
	switch ruleType {
	case FirewallRuleDomainAllow, FirewallRuleDomainBlock, FirewallRuleWildcardDomainBlock,
		FirewallRuleDNSRewrite, FirewallRuleUpstreamResolverConfig:
		return nil
	default:
		return fmt.Errorf("invalid rule_type: %s", ruleType)
	}
}
