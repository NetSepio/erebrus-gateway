package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ProvisioningConfig carries managed-node defaults from gateway env.
type ProvisioningConfig struct {
	Enabled       bool
	DefaultRegion string
}

// ProvisionIncludedPlanResources creates or reserves plan-included nodes and services.
func (s *Store) ProvisionIncludedPlanResources(ctx context.Context, orgID, plan string, cfg ProvisioningConfig) error {
	plan, err := normalizeOrgPlan(plan)
	if err != nil {
		return err
	}
	switch plan {
	case OrgPlanPro:
		return s.provisionProPlan(ctx, orgID, cfg)
	case OrgPlanBusiness:
		return s.provisionBusinessPlan(ctx, orgID, cfg)
	default:
		return nil
	}
}

func (s *Store) provisionProPlan(ctx context.Context, orgID string, cfg ProvisioningConfig) error {
	nodes, err := s.countManagedNodesByProfile(ctx, orgID, DeploymentProfileShield)
	if err != nil {
		return err
	}
	if nodes == 0 {
		nodeID, err := s.reserveManagedNode(ctx, orgID, DeploymentProfileShield, cfg)
		if err != nil {
			return err
		}
		if err := s.attachVPNService(ctx, orgID, nodeID); err != nil {
			return err
		}
		if err := s.attachShieldService(ctx, orgID, nodeID); err != nil {
			return err
		}
	} else {
		existing, err := s.listManagedNodesByProfile(ctx, orgID, DeploymentProfileShield)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			nodeID := existing[0].NodeID
			if err := s.attachVPNService(ctx, orgID, nodeID); err != nil {
				return err
			}
			if err := s.attachShieldService(ctx, orgID, nodeID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) provisionBusinessPlan(ctx context.Context, orgID string, cfg ProvisioningConfig) error {
	if err := s.ensureSentinelLicenses(ctx, orgID); err != nil {
		return err
	}
	existing, err := s.listManagedNodesByProfile(ctx, orgID, DeploymentProfileSentinel)
	if err != nil {
		return err
	}
	need := 3 - len(existing)
	for i := 0; i < need; i++ {
		nodeID, err := s.reserveManagedNode(ctx, orgID, DeploymentProfileSentinel, cfg)
		if err != nil {
			return err
		}
		if err := s.attachVPNService(ctx, orgID, nodeID); err != nil {
			return err
		}
		if err := s.attachSentinelService(ctx, orgID, nodeID); err != nil {
			return err
		}
	}
	for _, n := range existing {
		if err := s.attachVPNService(ctx, orgID, n.NodeID); err != nil {
			return err
		}
		if err := s.attachSentinelService(ctx, orgID, n.NodeID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) reserveManagedNode(ctx context.Context, orgID, profile string, cfg ProvisioningConfig) (string, error) {
	status := OrgNodeStatusPending
	if cfg.Enabled {
		status = OrgNodeStatusProvisioning
	}
	region := cfg.DefaultRegion
	if region == "" {
		region = "unknown"
	}
	nodeID := "managed-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	_, err := s.UpsertOrgNode(ctx, OrgNodeInput{
		OrgID: orgID, NodeID: nodeID, NodeName: managedNodeName(profile),
		DeploymentProfile: profile, NodeType: OrgNodeTypeManaged,
		Visibility: OrgNodeVisibilityPrivateOrg, ManagedBy: OrgNodeManagedByErebrus,
		Region: region, Status: status,
	})
	return nodeID, err
}

func managedNodeName(profile string) string {
	switch profile {
	case DeploymentProfileShield:
		return "Erebrus Shield (managed)"
	case DeploymentProfileSentinel:
		return "Erebrus Sentinel (managed)"
	default:
		return "Erebrus VPN (managed)"
	}
}

func (s *Store) attachVPNService(ctx context.Context, orgID, nodeID string) error {
	_, err := s.AttachServiceToNode(ctx, AttachServiceInput{
		OrgID: orgID, NodeID: nodeID,
		ServiceType: ServiceTypeVPN, ServiceName: ServiceNameErebrus,
		ServiceProvider: ServiceProviderWireguard, Visibility: ServiceVisibilityOrgOnly,
	})
	return err
}

func (s *Store) attachShieldService(ctx context.Context, orgID, nodeID string) error {
	_, err := s.AttachServiceToNode(ctx, AttachServiceInput{
		OrgID: orgID, NodeID: nodeID,
		ServiceType: ServiceTypeCommunityFirewall, ServiceName: ServiceNameErebrusShield,
		ServiceProvider: ServiceProviderAdGuardHome, Visibility: ServiceVisibilityVPNOnly,
	})
	return err
}

func (s *Store) attachSentinelService(ctx context.Context, orgID, nodeID string) error {
	lic, err := s.AttachSentinelLicense(ctx, orgID, nodeID)
	if err != nil {
		return fmt.Errorf("attach sentinel license: %w", err)
	}
	_, err = s.AttachServiceToNode(ctx, AttachServiceInput{
		OrgID: orgID, NodeID: nodeID,
		ServiceType: ServiceTypeErebrusFirewall, ServiceName: ServiceNameErebrusSentinel,
		ServiceProvider: ServiceProviderUnboundErebrus, Visibility: ServiceVisibilityVPNOnly,
		LicenseID: lic.ID,
	})
	return err
}

func (s *Store) ensureSentinelLicenses(ctx context.Context, orgID string) error {
	ent, err := s.GetOrgEntitlements(ctx, orgID)
	if err != nil {
		return err
	}
	var have int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM sentinel_licenses WHERE org_id=$1`, orgID).Scan(&have); err != nil {
		return err
	}
	need := ent.SentinelLicensesIncluded - have
	if need > 0 {
		return s.CreateSentinelLicenses(ctx, orgID, need, SentinelLicenseIncluded)
	}
	return nil
}

func (s *Store) countManagedNodesByProfile(ctx context.Context, orgID, profile string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM org_nodes WHERE org_id=$1 AND managed_by=$2 AND deployment_profile=$3`,
		orgID, OrgNodeManagedByErebrus, profile).Scan(&n)
	return n, err
}

func (s *Store) listManagedNodesByProfile(ctx context.Context, orgID, profile string) ([]OrgNode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+orgNodeCols+` FROM org_nodes
		 WHERE org_id=$1 AND managed_by=$2 AND deployment_profile=$3 ORDER BY created_at`,
		orgID, OrgNodeManagedByErebrus, profile)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgNode
	for rows.Next() {
		n, err := scanOrgNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// SetOrgPlanAndProvision updates plan, entitlements, and included resources.
func (s *Store) SetOrgPlanAndProvision(ctx context.Context, orgID, plan string, cfg ProvisioningConfig) (*Org, error) {
	org, err := s.SetOrgPlan(ctx, orgID, plan)
	if err != nil {
		return nil, err
	}
	if err := s.ProvisionIncludedPlanResources(ctx, orgID, plan, cfg); err != nil {
		return nil, err
	}
	return org, nil
}

// ReconcileUnlicensedSentinel marks Sentinel services unlicensed when no license is available.
func (s *Store) ReconcileUnlicensedSentinel(ctx context.Context, orgID, nodeID string) error {
	avail, err := s.CountAvailableSentinelLicenses(ctx, orgID)
	if err != nil {
		return err
	}
	if avail > 0 {
		return nil
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE org_node_services SET service_status=$4, updated_at=now()
		 WHERE org_id=$1 AND node_id=$2 AND service_type=$3 AND service_status <> $5`,
		orgID, nodeID, ServiceTypeErebrusFirewall, ServiceStatusUnlicensed, ServiceStatusDisabled)
	return err
}