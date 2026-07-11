package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/secrets"
)

const orgNodeCols = `id, org_id, node_id, COALESCE(node_name,''), deployment_profile, node_type,
	visibility, managed_by, COALESCE(region,''), COALESCE(zone,''), status,
	COALESCE(api_public_url,''), last_seen_at, COALESCE(created_by::text,''),
	created_at, updated_at`

const orgNodeColsJoined = `on2.id, on2.org_id, on2.node_id, COALESCE(on2.node_name,''), on2.deployment_profile, on2.node_type,
	on2.visibility, on2.managed_by, COALESCE(on2.region,''), COALESCE(on2.zone,''), on2.status,
	COALESCE(on2.api_public_url,''), on2.last_seen_at, COALESCE(on2.created_by::text,''),
	on2.created_at, on2.updated_at`

const orgNodeRuntimeCols = `, COALESCE(n.status,''), COALESCE(n.access_mode,''), n.last_heartbeat`

func scanOrgNode(sc interface{ Scan(...any) error }) (*OrgNode, error) {
	var n OrgNode
	err := sc.Scan(
		&n.ID, &n.OrgID, &n.NodeID, &n.NodeName, &n.DeploymentProfile, &n.NodeType,
		&n.Visibility, &n.ManagedBy, &n.Region, &n.Zone, &n.Status,
		&n.APIPublicURL, &n.LastSeenAt, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt,
	)
	return &n, err
}

func scanOrgNodeWithRuntime(sc interface{ Scan(...any) error }) (*OrgNode, error) {
	var n OrgNode
	var runtimeStatus, accessMode sql.NullString
	var lastHeartbeat sql.NullTime
	err := sc.Scan(
		&n.ID, &n.OrgID, &n.NodeID, &n.NodeName, &n.DeploymentProfile, &n.NodeType,
		&n.Visibility, &n.ManagedBy, &n.Region, &n.Zone, &n.Status,
		&n.APIPublicURL, &n.LastSeenAt, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt,
		&runtimeStatus, &accessMode, &lastHeartbeat,
	)
	if err != nil {
		return nil, err
	}
	if runtimeStatus.Valid {
		n.RuntimeStatus = runtimeStatus.String
	}
	if accessMode.Valid {
		n.AccessMode = accessMode.String
	}
	if lastHeartbeat.Valid {
		t := lastHeartbeat.Time
		n.LastHeartbeat = &t
	}
	return &n, err
}

// OrgNodeInput carries fields for creating or updating an org node record.
type OrgNodeInput struct {
	OrgID             string
	NodeID            string
	NodeName          string
	DeploymentProfile string
	NodeType          string
	Visibility        string
	ManagedBy         string
	Region            string
	Zone              string
	Status            string
	APIPublicURL      string
	CreatedBy         string
}

// UpsertOrgNode creates or updates the org control-plane node record.
func (s *Store) UpsertOrgNode(ctx context.Context, in OrgNodeInput) (*OrgNode, error) {
	in.DeploymentProfile = defaultIfEmpty(in.DeploymentProfile, DeploymentProfileStandard)
	in.NodeType = defaultIfEmpty(in.NodeType, OrgNodeTypeBYOC)
	in.Visibility = defaultIfEmpty(in.Visibility, OrgNodeVisibilityPrivateOrg)
	in.ManagedBy = defaultIfEmpty(in.ManagedBy, OrgNodeManagedByOrg)
	in.Status = defaultIfEmpty(in.Status, OrgNodeStatusActive)
	return scanOrgNode(s.db.QueryRowContext(ctx,
		`INSERT INTO org_nodes (
			org_id, node_id, node_name, deployment_profile, node_type, visibility,
			managed_by, region, zone, status, api_public_url, created_by
		) VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,NULLIF($11,''),NULLIF($12,'')::uuid)
		ON CONFLICT (node_id) DO UPDATE SET
			org_id = EXCLUDED.org_id,
			node_name = COALESCE(NULLIF(EXCLUDED.node_name,''), org_nodes.node_name),
			deployment_profile = EXCLUDED.deployment_profile,
			node_type = EXCLUDED.node_type,
			visibility = EXCLUDED.visibility,
			managed_by = EXCLUDED.managed_by,
			region = COALESCE(NULLIF(EXCLUDED.region,''), org_nodes.region),
			zone = COALESCE(NULLIF(EXCLUDED.zone,''), org_nodes.zone),
			status = EXCLUDED.status,
			api_public_url = COALESCE(NULLIF(EXCLUDED.api_public_url,''), org_nodes.api_public_url),
			updated_at = now()
		RETURNING `+orgNodeCols,
		in.OrgID, in.NodeID, in.NodeName, in.DeploymentProfile, in.NodeType, in.Visibility,
		in.ManagedBy, in.Region, in.Zone, in.Status, in.APIPublicURL, in.CreatedBy))
}

// GetOrgNode returns an org node by its node_id (peer_id).
func (s *Store) GetOrgNode(ctx context.Context, orgID, nodeID string) (*OrgNode, error) {
	n, err := scanOrgNode(s.db.QueryRowContext(ctx,
		`SELECT `+orgNodeCols+` FROM org_nodes WHERE org_id=$1 AND node_id=$2`, orgID, nodeID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return n, err
}

// ListOrgNodes returns all org nodes for an org, including runtime connectivity
// from the nodes table (runtime_status, access_mode, last_heartbeat).
func (s *Store) ListOrgNodes(ctx context.Context, orgID string) ([]OrgNode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+orgNodeColsJoined+orgNodeRuntimeCols+`
		 FROM org_nodes on2
		 LEFT JOIN nodes n ON n.peer_id = on2.node_id
		 WHERE on2.org_id=$1 ORDER BY on2.created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgNode
	for rows.Next() {
		n, err := scanOrgNodeWithRuntime(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// UpdateOrgNodeInput carries patchable org node fields.
type UpdateOrgNodeInput struct {
	NodeName     *string
	Status       *string
	Region       *string
	Zone         *string
	APIPublicURL *string
}

// UpdateOrgNode patches org node metadata.
func (s *Store) UpdateOrgNode(ctx context.Context, orgID, nodeID string, in UpdateOrgNodeInput) (*OrgNode, error) {
	cur, err := s.GetOrgNode(ctx, orgID, nodeID)
	if err != nil {
		return nil, err
	}
	name, status, region, zone, apiURL := cur.NodeName, cur.Status, cur.Region, cur.Zone, cur.APIPublicURL
	if in.NodeName != nil {
		name = strings.TrimSpace(*in.NodeName)
	}
	if in.Status != nil {
		status = strings.TrimSpace(*in.Status)
	}
	if in.Region != nil {
		region = strings.TrimSpace(*in.Region)
	}
	if in.Zone != nil {
		zone = strings.TrimSpace(*in.Zone)
	}
	if in.APIPublicURL != nil {
		apiURL = strings.TrimSpace(*in.APIPublicURL)
	}
	return scanOrgNode(s.db.QueryRowContext(ctx,
		`UPDATE org_nodes SET node_name=NULLIF($3,''), status=$4, region=NULLIF($5,''),
		 zone=NULLIF($6,''), api_public_url=NULLIF($7,''), updated_at=now()
		 WHERE org_id=$1 AND node_id=$2
		 RETURNING `+orgNodeCols,
		orgID, nodeID, name, status, region, zone, apiURL))
}

// TouchOrgNodeHeartbeat updates last_seen_at for an org node.
func (s *Store) TouchOrgNodeHeartbeat(ctx context.Context, nodeID string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE org_nodes SET last_seen_at=$2, status='active', updated_at=now() WHERE node_id=$1`,
		nodeID, at)
	return err
}

// RuntimeToOrgNodeStatus maps runtime connectivity to org control-plane status.
func RuntimeToOrgNodeStatus(runtimeStatus string) string {
	switch strings.ToLower(strings.TrimSpace(runtimeStatus)) {
	case "online":
		return OrgNodeStatusActive
	case "draining":
		return OrgNodeStatusDegraded
	case "offline", "":
		return OrgNodeStatusDegraded
	default:
		return OrgNodeStatusDegraded
	}
}

// SyncOrgNodeStatusFromRuntime aligns org_nodes.status with nodes.status after
// disconnect or stale marking. Skips nodes manually disabled or in provisioning.
func (s *Store) SyncOrgNodeStatusFromRuntime(ctx context.Context, peerID, runtimeStatus string) error {
	status := RuntimeToOrgNodeStatus(runtimeStatus)
	_, err := s.db.ExecContext(ctx,
		`UPDATE org_nodes SET status=$2, updated_at=now()
		 WHERE node_id=$1
		   AND status NOT IN ($3, $4, $5, $6)`,
		peerID, status,
		OrgNodeStatusDisabled, OrgNodeStatusError, OrgNodeStatusPending, OrgNodeStatusProvisioning)
	return err
}

// SyncAllOrgNodeStatusesFromRuntime bulk-repairs org_nodes.status from nodes.
func (s *Store) SyncAllOrgNodeStatusesFromRuntime(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE org_nodes on2 SET
		   status = CASE n.status
		     WHEN 'online' THEN $1
		     WHEN 'draining' THEN $2
		     ELSE $3
		   END,
		   updated_at = now()
		 FROM nodes n
		 WHERE on2.node_id = n.peer_id
		   AND on2.status NOT IN ($4, $5, $6, $7)`,
		OrgNodeStatusActive, OrgNodeStatusDegraded, OrgNodeStatusDegraded,
		OrgNodeStatusDisabled, OrgNodeStatusError, OrgNodeStatusPending, OrgNodeStatusProvisioning)
	return err
}

// RegisterOrgNodeFromRuntime registers a runtime node under an org using a registration token.
// It rejects attempts to overwrite an existing node unless the caller presents the current node_key
// and the node already belongs to the same org.
func (s *Store) RegisterOrgNodeFromRuntime(ctx context.Context, orgID, tokenID string, r NodeRegistration) (peerID, nodeKey string, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback() //nolint:errcheck

	nodeType := OrgNodeTypeBYOC
	visibility := OrgNodeVisibilityPrivateOrg
	if r.AccessMode == NodeAccessPublic {
		nodeType = OrgNodeTypePublic
		visibility = OrgNodeVisibilityPublicNetwork
	}

	access := r.AccessMode
	if access != NodeAccessPrivate {
		access = NodeAccessPublic
	}

	nodeKey = strings.TrimSpace(r.NodeKey)

	// Verify existing node ownership and key.
	var storedNodeKey, storedOrgID string
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(node_key,''), COALESCE(org_id::text,'') FROM nodes WHERE peer_id = $1 FOR UPDATE`,
		r.PeerID).Scan(&storedNodeKey, &storedOrgID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	existingNode := !errors.Is(err, sql.ErrNoRows)

	if existingNode {
		if storedOrgID != orgID {
			return "", "", ErrConflict
		}
		if storedNodeKey != "" {
			if nodeKey == "" {
				return "", "", ErrConflict
			}
			if subtle.ConstantTimeCompare([]byte(storedNodeKey), []byte(nodeKey)) != 1 {
				return "", "", ErrConflict
			}
		} else {
			if nodeKey == "" {
				nodeKey, err = secrets.NewNodeKey()
				if err != nil {
					return "", "", err
				}
			}
		}
	} else {
		if nodeKey == "" {
			nodeKey, err = secrets.NewNodeKey()
			if err != nil {
				return "", "", err
			}
		}
	}

	var runtimeID string
	if existingNode {
		res, err := tx.ExecContext(ctx,
			`UPDATE nodes SET
				did = $2,
				wallet_address = COALESCE(NULLIF($3,''), wallet_address),
				chain = COALESCE(NULLIF($4,''), chain),
				org_id = $5::uuid,
				name = COALESCE(NULLIF($6,''), name),
				region = COALESCE(NULLIF($7,''), region),
				zone = COALESCE(NULLIF($8,''), zone),
				api_base_url = COALESCE(NULLIF($9,''), api_base_url),
				node_key = $10,
				access_mode = $11,
				deployment_profile = $12,
				updated_at = now()
			 WHERE peer_id = $1 AND COALESCE(org_id::text,'') = $5`,
			r.PeerID, r.DID, r.Wallet, r.Chain, orgID, r.Name, r.Region, r.Zone, r.APIBaseURL, nodeKey, access,
			NormalizeDeploymentProfile(r.DeploymentProfile))
		if err != nil {
			return "", "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return "", "", ErrConflict
		}
	} else {
		err = tx.QueryRowContext(ctx,
			`INSERT INTO nodes (peer_id, did, wallet_address, chain, org_id, name, region, zone, api_base_url, node_key, access_mode, deployment_profile)
			 VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,'')::uuid, NULLIF($6,''), NULLIF($7,''), NULLIF($8,''), NULLIF($9,''), NULLIF($10,''), $11, $12)
			 RETURNING id`,
			r.PeerID, r.DID, r.Wallet, r.Chain, orgID, r.Name, r.Region, r.Zone, r.APIBaseURL, nodeKey, access,
			NormalizeDeploymentProfile(r.DeploymentProfile)).Scan(&runtimeID)
		if err != nil {
			return "", "", err
		}
	}

	// Verify existing org-node ownership and upsert.
	var storedOrgNodeOrgID string
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(org_id::text,'') FROM org_nodes WHERE node_id = $1 FOR UPDATE`,
		r.PeerID).Scan(&storedOrgNodeOrgID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	existingOrgNode := !errors.Is(err, sql.ErrNoRows)

	if existingOrgNode {
		if storedOrgNodeOrgID != orgID {
			return "", "", ErrConflict
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE org_nodes SET
				node_name = COALESCE(NULLIF($2,''), node_name),
				deployment_profile = COALESCE(NULLIF($3,''), deployment_profile),
				node_type = $4,
				visibility = $5,
				region = COALESCE(NULLIF($6,''), region),
				zone = COALESCE(NULLIF($7,''), zone),
				status = $8,
				api_public_url = COALESCE(NULLIF($9,''), api_public_url),
				updated_at = now()
			 WHERE node_id = $1 AND org_id = $10`,
			r.PeerID, r.Name, NormalizeDeploymentProfile(r.DeploymentProfile), nodeType, visibility,
			r.Region, r.Zone, OrgNodeStatusActive, r.APIBaseURL, orgID)
		if err != nil {
			return "", "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return "", "", ErrConflict
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO org_nodes (
				org_id, node_id, node_name, deployment_profile, node_type, visibility,
				managed_by, region, zone, status, api_public_url
			) VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,NULLIF($11,''))
			ON CONFLICT (node_id) DO UPDATE SET
				node_name = COALESCE(NULLIF(EXCLUDED.node_name,''), org_nodes.node_name),
				deployment_profile = COALESCE(NULLIF(EXCLUDED.deployment_profile,''), org_nodes.deployment_profile),
				node_type = EXCLUDED.node_type,
				visibility = EXCLUDED.visibility,
				region = COALESCE(NULLIF(EXCLUDED.region,''), org_nodes.region),
				zone = COALESCE(NULLIF(EXCLUDED.zone,''), org_nodes.zone),
				status = EXCLUDED.status,
				api_public_url = COALESCE(NULLIF(EXCLUDED.api_public_url,''), org_nodes.api_public_url),
				updated_at = now()
			 WHERE org_nodes.org_id = EXCLUDED.org_id`,
			orgID, r.PeerID, r.Name, NormalizeDeploymentProfile(r.DeploymentProfile), nodeType, visibility,
			OrgNodeManagedByOrg, r.Region, r.Zone, OrgNodeStatusActive, r.APIBaseURL); err != nil {
			return "", "", err
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO org_node_services (
			org_id, node_id, service_type, service_name, service_provider, service_status, visibility
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (node_id, service_type) DO UPDATE SET
			service_status = EXCLUDED.service_status, updated_at = now()`,
		orgID, r.PeerID, ServiceTypeVPN, ServiceNameErebrus, ServiceProviderWireguard,
		ServiceStatusActive, ServiceVisibilityOrgOnly); err != nil {
		return "", "", err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE node_registration_tokens SET used_at=now() WHERE id=$1 AND used_at IS NULL`, tokenID); err != nil {
		return "", "", err
	}

	profile := NormalizeDeploymentProfile(r.DeploymentProfile)
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	if err := s.attachProfileServices(ctx, orgID, r.PeerID, profile); err != nil {
		return "", "", err
	}
	return r.PeerID, nodeKey, nil
}

// NormalizeDeploymentProfile returns a valid deployment profile name.
func NormalizeDeploymentProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case DeploymentProfileShield, DeploymentProfileSentinel, DeploymentProfileStandard:
		return strings.ToLower(strings.TrimSpace(profile))
	default:
		return DeploymentProfileStandard
	}
}

func (s *Store) attachProfileServices(ctx context.Context, orgID, nodeID, profile string) error {
	switch profile {
	case DeploymentProfileShield:
		return s.attachShieldService(ctx, orgID, nodeID)
	case DeploymentProfileSentinel:
		if err := s.attachSentinelService(ctx, orgID, nodeID); err != nil {
			if errors.Is(err, ErrNotFound) {
				_, attachErr := s.AttachServiceToNode(ctx, AttachServiceInput{
					OrgID: orgID, NodeID: nodeID,
					ServiceType: ServiceTypeErebrusFirewall, ServiceName: ServiceNameErebrusSentinel,
					ServiceProvider: ServiceProviderUnboundErebrus, Visibility: ServiceVisibilityVPNOnly,
				})
				if attachErr == nil {
					_, _ = s.db.ExecContext(ctx,
						`UPDATE org_node_services SET service_status=$4, updated_at=now()
						 WHERE org_id=$1 AND node_id=$2 AND service_type=$3`,
						orgID, nodeID, ServiceTypeErebrusFirewall, ServiceStatusUnlicensed)
				}
				return attachErr
			}
			return err
		}
	}
	return nil
}

func defaultIfEmpty(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return strings.TrimSpace(val)
}

// AttachServiceInput carries service attachment fields.
type AttachServiceInput struct {
	OrgID           string
	NodeID          string
	ServiceType     string
	ServiceName     string
	ServiceProvider string
	Visibility      string
	ConfigRef       string
	AccessURL       string
	LicenseID       string
	CreatedBy       string
}

// AttachServiceToNode adds or updates a service on an org node.
func (s *Store) AttachServiceToNode(ctx context.Context, in AttachServiceInput) (*OrgNodeService, error) {
	if _, err := s.GetOrgNode(ctx, in.OrgID, in.NodeID); err != nil {
		return nil, err
	}
	return scanOrgNodeService(s.db.QueryRowContext(ctx,
		`INSERT INTO org_node_services (
			org_id, node_id, service_type, service_name, service_provider,
			service_status, visibility, config_ref, access_url, license_id, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),NULLIF($10,'')::uuid,NULLIF($11,'')::uuid)
		ON CONFLICT (node_id, service_type) DO UPDATE SET
			service_name = EXCLUDED.service_name,
			service_provider = EXCLUDED.service_provider,
			service_status = EXCLUDED.service_status,
			visibility = EXCLUDED.visibility,
			config_ref = COALESCE(NULLIF(EXCLUDED.config_ref,''), org_node_services.config_ref),
			access_url = COALESCE(NULLIF(EXCLUDED.access_url,''), org_node_services.access_url),
			license_id = COALESCE(EXCLUDED.license_id, org_node_services.license_id),
			updated_at = now()
		RETURNING `+orgNodeServiceCols,
		in.OrgID, in.NodeID, in.ServiceType, in.ServiceName, in.ServiceProvider,
		ServiceStatusActive, in.Visibility, in.ConfigRef, in.AccessURL, in.LicenseID, in.CreatedBy))
}

const orgNodeServiceCols = `id, org_id, node_id, service_type, service_name, service_provider,
	service_status, visibility, COALESCE(config_ref,''), COALESCE(access_url,''),
	COALESCE(license_id::text,''), COALESCE(created_by::text,''), created_at, updated_at`

func scanOrgNodeService(sc interface{ Scan(...any) error }) (*OrgNodeService, error) {
	var svc OrgNodeService
	err := sc.Scan(
		&svc.ID, &svc.OrgID, &svc.NodeID, &svc.ServiceType, &svc.ServiceName, &svc.ServiceProvider,
		&svc.ServiceStatus, &svc.Visibility, &svc.ConfigRef, &svc.AccessURL,
		&svc.LicenseID, &svc.CreatedBy, &svc.CreatedAt, &svc.UpdatedAt,
	)
	return &svc, err
}

// GetOrgNodeService returns a service by id within an org node.
func (s *Store) GetOrgNodeService(ctx context.Context, orgID, nodeID, serviceID string) (*OrgNodeService, error) {
	svc, err := scanOrgNodeService(s.db.QueryRowContext(ctx,
		`SELECT `+orgNodeServiceCols+` FROM org_node_services WHERE org_id=$1 AND node_id=$2 AND id=$3`,
		orgID, nodeID, serviceID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return svc, err
}

// UpdateOrgNodeServiceInput carries patchable service fields.
type UpdateOrgNodeServiceInput struct {
	ServiceStatus *string
	Visibility    *string
	ConfigRef     *string
	AccessURL     *string
}

// UpdateOrgNodeService patches a node service record.
func (s *Store) UpdateOrgNodeService(ctx context.Context, orgID, nodeID, serviceID string, in UpdateOrgNodeServiceInput) (*OrgNodeService, error) {
	cur, err := s.GetOrgNodeService(ctx, orgID, nodeID, serviceID)
	if err != nil {
		return nil, err
	}
	status, visibility, cfg, url := cur.ServiceStatus, cur.Visibility, cur.ConfigRef, cur.AccessURL
	if in.ServiceStatus != nil {
		status = strings.TrimSpace(*in.ServiceStatus)
	}
	if in.Visibility != nil {
		visibility = strings.TrimSpace(*in.Visibility)
	}
	if in.ConfigRef != nil {
		cfg = strings.TrimSpace(*in.ConfigRef)
	}
	if in.AccessURL != nil {
		url = strings.TrimSpace(*in.AccessURL)
	}
	return scanOrgNodeService(s.db.QueryRowContext(ctx,
		`UPDATE org_node_services SET service_status=$4, visibility=$5,
		 config_ref=NULLIF($6,''), access_url=NULLIF($7,''), updated_at=now()
		 WHERE id=$1 AND org_id=$2 AND node_id=$3
		 RETURNING `+orgNodeServiceCols,
		serviceID, orgID, nodeID, status, visibility, cfg, url))
}

// DeleteOrgNodeService disables a service attachment.
func (s *Store) DeleteOrgNodeService(ctx context.Context, orgID, nodeID, serviceID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE org_node_services SET service_status=$4, updated_at=now()
		 WHERE id=$1 AND org_id=$2 AND node_id=$3`,
		serviceID, orgID, nodeID, ServiceStatusDisabled)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListOrgNodeServices returns services attached to a node.
func (s *Store) ListOrgNodeServices(ctx context.Context, orgID, nodeID string) ([]OrgNodeService, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+orgNodeServiceCols+` FROM org_node_services WHERE org_id=$1 AND node_id=$2 ORDER BY created_at`,
		orgID, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgNodeService
	for rows.Next() {
		svc, err := scanOrgNodeService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *svc)
	}
	return out, rows.Err()
}

// DeploymentProfileAllowsService reports whether a profile supports a service type.
func DeploymentProfileAllowsService(profile, serviceType string) bool {
	switch profile {
	case DeploymentProfileStandard:
		return serviceType == ServiceTypeVPN
	case DeploymentProfileShield:
		return serviceType == ServiceTypeVPN || serviceType == ServiceTypeCommunityFirewall
	case DeploymentProfileSentinel:
		return serviceType == ServiceTypeVPN || serviceType == ServiceTypeErebrusFirewall
	default:
		return false
	}
}

// ValidateServiceEntitlement checks whether an org may attach a service to a node.
func (s *Store) ValidateServiceEntitlement(ctx context.Context, orgID, nodeID, serviceType string) error {
	node, err := s.GetOrgNode(ctx, orgID, nodeID)
	if err != nil {
		return err
	}
	if !DeploymentProfileAllowsService(node.DeploymentProfile, serviceType) {
		return fmt.Errorf("deployment profile %s does not support service %s", node.DeploymentProfile, serviceType)
	}
	ent, err := s.GetOrgEntitlements(ctx, orgID)
	if err != nil {
		return err
	}
	switch serviceType {
	case ServiceTypeCommunityFirewall:
		if ent.ShieldInstancesIncluded <= 0 {
			return fmt.Errorf("plan does not include Shield instances")
		}
		used, err := s.CountShieldInstancesUsed(ctx, orgID)
		if err != nil {
			return err
		}
		if used >= ent.ShieldInstancesIncluded {
			return fmt.Errorf("no Shield instances remaining")
		}
	case ServiceTypeErebrusFirewall:
		avail, err := s.CountAvailableSentinelLicenses(ctx, orgID)
		if err != nil {
			return err
		}
		if avail <= 0 {
			return fmt.Errorf("no Sentinel licenses available")
		}
	}
	services, err := s.ListOrgNodeServices(ctx, orgID, nodeID)
	if err != nil {
		return err
	}
	for _, svc := range services {
		if svc.ServiceType == serviceType && svc.ServiceStatus != ServiceStatusDisabled {
			return fmt.Errorf("service already attached")
		}
	}
	return nil
}