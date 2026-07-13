package store

import (
	"encoding/json"
	"time"
)

// Org plans.
const (
	OrgPlanBasic      = "basic"
	OrgPlanStarter    = "starter"
	OrgPlanPro        = "pro"
	OrgPlanBusiness   = "business"
	OrgPlanEnterprise = "enterprise"
)

// Org billing status.
const (
	OrgBillingActive   = "active"
	OrgBillingPastDue  = "past_due"
	OrgBillingCanceled = "canceled"
	OrgBillingTrialing = "trialing"
)

// Org verification status.
const (
	OrgVerificationUnverified = "unverified"
	OrgVerificationVerified   = "verified"
	OrgVerificationRejected   = "rejected"
)

// Org member roles.
const (
	OrgRoleOwner        = "owner"
	OrgRoleNodeOperator = "node_operator"
	OrgRoleMember       = "member"
)

// Seat tiers (premium access entitlement; separate from management role).
const (
	SeatTierFree       = "free"
	SeatTierStarter    = "starter"
	SeatTierPro        = "pro"
	SeatTierBusiness   = "business"
	SeatTierEnterprise = "enterprise"
)

// Member status.
const (
	MemberStatusActive    = "active"
	MemberStatusInvited   = "invited"
	MemberStatusSuspended = "suspended"
	MemberStatusRemoved   = "removed"
)

// Node access modes.
const (
	NodeAccessPublic  = "public"
	NodeAccessPrivate = "private"
)

// Deployment profiles.
const (
	DeploymentProfileStandard = "standard"
	DeploymentProfileShield   = "shield"
	DeploymentProfileSentinel = "sentinel"
)

// Org node types.
const (
	OrgNodeTypePublic  = "public"
	OrgNodeTypePrivate = "private"
	OrgNodeTypeBYOC    = "byoc"
	OrgNodeTypeManaged = "managed"
)

// Org node visibility.
const (
	OrgNodeVisibilityPrivateOrg    = "private_org"
	OrgNodeVisibilityPublicNetwork = "public_network"
	OrgNodeVisibilityRestricted    = "restricted"
)

// Org node managed_by.
const (
	OrgNodeManagedByOrg     = "org"
	OrgNodeManagedByErebrus = "erebrus"
)

// Org node status.
const (
	OrgNodeStatusPending      = "pending"
	OrgNodeStatusProvisioning = "provisioning"
	OrgNodeStatusActive       = "active"
	OrgNodeStatusDegraded     = "degraded"
	OrgNodeStatusDisabled     = "disabled"
	OrgNodeStatusError        = "error"
)

// Service types.
const (
	ServiceTypeVPN               = "vpn"
	ServiceTypeCommunityFirewall = "community_firewall"
	ServiceTypeErebrusFirewall   = "erebrus_firewall"
	ServiceTypeDrop              = "drop"
	ServiceTypeAI                = "ai"
	ServiceTypeCustomApp         = "custom_app"
)

// Service names.
const (
	ServiceNameErebrus         = "erebrus"
	ServiceNameErebrusShield   = "erebrus_shield"
	ServiceNameErebrusSentinel = "erebrus_sentinel"
)

// Service providers.
const (
	ServiceProviderWireguard      = "wireguard"
	ServiceProviderAdGuardHome    = "adguard_home"
	ServiceProviderUnboundErebrus = "unbound_erebrus"
	ServiceProviderErebrusDrop    = "erebrus_drop"
	ServiceProviderCustom         = "custom"
)

// Service status.
const (
	ServiceStatusPending         = "pending"
	ServiceStatusProvisioning    = "provisioning"
	ServiceStatusActive          = "active"
	ServiceStatusDegraded        = "degraded"
	ServiceStatusDisabled        = "disabled"
	ServiceStatusUnlicensed      = "unlicensed"
	ServiceStatusUnsupportedPlan = "unsupported_plan"
	ServiceStatusError           = "error"
)

// Service visibility.
const (
	ServiceVisibilityVPNOnly = "vpn_only"
	ServiceVisibilityOrgOnly = "org_only"
	ServiceVisibilityPublic  = "public"
)

// Registration token scopes.
const (
	TokenScopeNodeRegistration = "node_registration"
	TokenScopeFirewallSetup    = "firewall_setup"
	TokenScopeServiceSetup     = "service_setup"
)

// Sentinel license status.
const (
	SentinelLicenseAvailable = "available"
	SentinelLicenseAttached  = "attached"
	SentinelLicenseSuspended = "suspended"
	SentinelLicenseExpired   = "expired"
)

// Sentinel license source.
const (
	SentinelLicenseIncluded         = "included"
	SentinelLicensePurchased        = "purchased"
	SentinelLicenseEnterpriseCustom = "enterprise_custom"
)

// Drop storage scope.
const (
	DropScopePublic     = "public"
	DropScopePrivateOrg = "private_org"
)

// Drop file visibility.
const (
	DropVisibilityPublic  = "public"
	DropVisibilityPrivate = "private"
)

// Drop upload lifecycle status.
const (
	DropUploadReserved  = "reserved"
	DropUploadUploading = "uploading"
	DropUploadCommitted = "committed"
	DropUploadFailed    = "failed"
	DropUploadExpired   = "expired"
)

// Drop file status.
const (
	DropFileActive        = "active"
	DropFileDeletePending = "delete_pending"
	DropFileDeleted       = "deleted"
	DropFileUnavailable   = "unavailable"
)

// Drop pin status.
const (
	DropPinPinning      = "pinning"
	DropPinPinned       = "pinned"
	DropPinUnpinPending = "unpin_pending"
	DropPinUnpinned     = "unpinned"
	DropPinError        = "error"
)

// Node Drop runtime state (mirrors the node capability/status contract).
const (
	DropStateDisabled    = "disabled"
	DropStateStarting    = "starting"
	DropStateActive      = "active"
	DropStateDegraded    = "degraded"
	DropStateFull        = "full"
	DropStateUnreachable = "unreachable"
)

// Drop effective tiers. These share names with org plans/seat tiers, but are a
// distinct concept: the public Drop quota bucket resolved from the highest
// active organization seat.
const (
	DropTierFree       = "free"
	DropTierStarter    = "starter"
	DropTierPro        = "pro"
	DropTierBusiness   = "business"
	DropTierEnterprise = "enterprise"
)

// Drop quota principal types. v1 charges public quota per user; a generic
// principal leaves room for future organization-level quotas.
const (
	DropPrincipalUser = "user"
	DropPrincipalOrg  = "org"
)

// Canonical public Drop quota policy (decimal bytes), seeded into
// drop_tier_limits. Enterprise has no default until product review.
const (
	DropQuotaFreeBytes     int64 = 500_000_000
	DropQuotaStarterBytes  int64 = 1_000_000_000
	DropQuotaProBytes      int64 = 5_000_000_000
	DropQuotaBusinessBytes int64 = 10_000_000_000
	// DropMaxFileBytes is the initial per-file ceiling (1 GB) applied to every
	// tier; larger/resumable uploads are a follow-up.
	DropMaxFileBytes int64 = 1_000_000_000
)

// User is a gateway account, keyed by wallet. An optional verified email may be
// linked for perks/recovery (never required to use the VPN).
type User struct {
	ID            string `json:"id"`
	WalletAddress string `json:"wallet_address,omitempty"`
	Chain         string `json:"chain,omitempty"`
	Role          string `json:"role"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name,omitempty"`
	// ProfilePicture is the bare IPFS CID of the avatar (no gateway prefix).
	ProfilePicture string    `json:"profile_picture,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// Node is a registered VPN node and its latest control-plane snapshot.
type Node struct {
	ID                string          `json:"id"`
	PeerID            string          `json:"peer_id"`
	DID               string          `json:"did"`
	WalletAddress     string          `json:"wallet_address,omitempty"`
	Chain             string          `json:"chain,omitempty"` // SOLANA | ETHEREUM (set at enrollment)
	OrgID             string          `json:"org_id,omitempty"`
	AccessMode        string          `json:"access_mode"`
	MinTier           int             `json:"min_tier"`
	Name              string          `json:"name"`
	Region            string          `json:"region"`
	Zone              string          `json:"zone,omitempty"`
	IP                string          `json:"ip,omitempty"` // never serialized publicly
	IPHash            string          `json:"ip_hash,omitempty"`
	Spec              json.RawMessage `json:"spec"`
	Capabilities      json.RawMessage `json:"capabilities"`
	Endpoints         json.RawMessage `json:"endpoints"`
	Protocols         []string        `json:"protocols"`
	Status            string          `json:"status"`
	Load              json.RawMessage `json:"load"`
	Speedtest         json.RawMessage `json:"speedtest"`
	RxBytes           int64           `json:"rx_bytes"`
	TxBytes           int64           `json:"tx_bytes"`
	Version           string          `json:"version"`
	DeploymentProfile string          `json:"deployment_profile"` // standard | shield | sentinel
	LastHeartbeat     *time.Time      `json:"last_heartbeat,omitempty"`
	LastPeerHandshake *time.Time      `json:"last_peer_handshake,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

// Plan is a subscription tier.
type Plan struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PeriodDays int    `json:"period_days"`
	MaxClients int    `json:"max_clients"`
}

// Subscription is a user's (or org's) entitlement.
type Subscription struct {
	ID               string     
	PlanID           string     
	Source           string     
	Status           string     
	CurrentPeriodEnd *time.Time 
	CreatedAt        time.Time  
}

// Org is a workspace; members and API keys operate within it.
type Org struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Slug                 string    `json:"slug"`
	Plan                 string    `json:"plan"`
	BillingStatus        string    `json:"billing_status"`
	VerificationStatus   string    `json:"verification_status"`
	PublicProfileEnabled bool      `json:"public_profile_enabled"`
	OwnerUserID          string    `json:"owner_user_id"`
	Role                 string    `json:"role,omitempty"`      // caller's role, when listed for a user
	SeatTier             string    `json:"seat_tier,omitempty"` // caller's seat tier, when listed for a user
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// OrgProfile holds org branding and contact metadata.
type OrgProfile struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	LegalName    string    `json:"legal_name,omitempty"`
	DisplayName  string    `json:"display_name,omitempty"`
	Description  string    `json:"description,omitempty"`
	LogoURL      string    `json:"logo_url,omitempty"`
	WebsiteURL   string    `json:"website_url,omitempty"`
	PublicEmail  string    `json:"public_email,omitempty"`
	BillingEmail string    `json:"billing_email,omitempty"`
	SupportEmail string    `json:"support_email,omitempty"`
	Country      string    `json:"country,omitempty"`
	Timezone     string    `json:"timezone,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OrgEntitlement is the plan-derived resource limits for an org.
type OrgEntitlement struct {
	ID                       string    `json:"id"`
	OrgID                    string    `json:"org_id"`
	Plan                     string    `json:"plan"`
	PaidSeatsIncluded        int       `json:"paid_seats_included"`
	ManagedVPNNodesIncluded  int       `json:"managed_vpn_nodes_included"`
	ShieldInstancesIncluded  int       `json:"shield_instances_included"`
	SentinelLicensesIncluded int       `json:"sentinel_licenses_included"`
	PublicNodeAccessTier     string    `json:"public_node_access_tier,omitempty"`
	APIQuotaMonthly          *int      `json:"api_quota_monthly,omitempty"`
	BandwidthPolicy          string    `json:"bandwidth_policy,omitempty"`
	SupportTier              string    `json:"support_tier,omitempty"`
	AuditLogsEnabled         bool      `json:"audit_logs_enabled"`
	AdvancedAnalyticsEnabled bool      `json:"advanced_analytics_enabled"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// Member is a user's membership in an org.
type Member struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	WalletAddress string    `json:"wallet_address,omitempty"`
	Email         string    `json:"email,omitempty"`
	Name          string    `json:"name,omitempty"`
	Role          string    `json:"role"`
	SeatTier      string    `json:"seat_tier"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// OrgNode is the org control-plane record for a runtime node.
type OrgNode struct {
	ID                string     `json:"id"`
	OrgID             string     `json:"org_id"`
	NodeID            string     `json:"node_id"`
	NodeName          string     `json:"node_name,omitempty"`
	DeploymentProfile string     `json:"deployment_profile"`
	NodeType          string     `json:"node_type"`
	Visibility        string     `json:"visibility"`
	ManagedBy         string     `json:"managed_by"`
	Region            string     `json:"region,omitempty"`
	Zone              string     `json:"zone,omitempty"`
	Status            string     `json:"status"`
	RuntimeStatus     string     `json:"runtime_status,omitempty"`
	AccessMode        string     `json:"access_mode,omitempty"`
	LastHeartbeat     *time.Time `json:"last_heartbeat,omitempty"`
	APIPublicURL      string     `json:"api_public_url,omitempty"`
	LastSeenAt        *time.Time `json:"last_seen_at,omitempty"`
	CreatedBy         string     `json:"created_by,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// OrgNodeService is a capability attached to an org node.
type OrgNodeService struct {
	ID              string    `json:"id"`
	OrgID           string    `json:"org_id"`
	NodeID          string    `json:"node_id"`
	ServiceType     string    `json:"service_type"`
	ServiceName     string    `json:"service_name"`
	ServiceProvider string    `json:"service_provider"`
	ServiceStatus   string    `json:"service_status"`
	Visibility      string    `json:"visibility"`
	ConfigRef       string    `json:"config_ref,omitempty"`
	AccessURL       string    `json:"access_url,omitempty"`
	LicenseID       string    `json:"license_id,omitempty"`
	CreatedBy       string    `json:"created_by,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// NodeRegistrationToken is a scoped, expiring org credential for node setup.
type NodeRegistrationToken struct {
	ID        string     `json:"id"`
	OrgID     string     `json:"org_id"`
	PeerID    string     `json:"peer_id,omitempty"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedBy string     `json:"created_by"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// SentinelLicense tracks per-node Sentinel entitlements.
type SentinelLicense struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	NodeID    string    `json:"node_id,omitempty"`
	Status    string    `json:"status"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// APIKey is an org-scoped credential (the secret is shown only once at creation).
type APIKey struct {
	ID         string     `json:"id"`
	OrgID      string     `json:"org_id"`
	Name       string     `json:"name,omitempty"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// Client is a provisioned VPN client.
type Client struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	OrgID         string     `json:"org_id,omitempty"`
	NodeID        string     `json:"node_id"`
	Name          string     `json:"name"`
	WGPublicKey   string     `json:"wg_public_key"`
	WGAllowedIP   string     `json:"wg_allowed_ip,omitempty"`
	Status        string     `json:"status"`
	RxBytes       int64      `json:"rx_bytes"`
	TxBytes       int64      `json:"tx_bytes"`
	LastHandshake *time.Time `json:"last_handshake,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// DropTierLimit is the public Drop quota policy for one effective tier. Kept in
// a table (not handlers) so limits can be tuned by admins without a code change.
type DropTierLimit struct {
	Tier               string    `json:"tier"`
	PublicStorageBytes int64     `json:"public_storage_bytes"`
	MaxFileBytes       int64     `json:"max_file_bytes"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// DropUpload is a short-lived reservation + idempotency record for one logical
// upload, from initialization through commit/expiry.
type DropUpload struct {
	ID                 string          `json:"id"`
	OwnerUserID        string          `json:"owner_user_id"`
	OrgID              string          `json:"org_id,omitempty"`             // private storage owner
	EntitlementOrgID   string          `json:"entitlement_org_id,omitempty"` // provenance of the effective tier
	NodeID             string          `json:"node_id"`
	StorageScope       string          `json:"storage_scope"`
	Visibility         string          `json:"visibility"`
	Filename           string          `json:"filename"`
	ContentType        string          `json:"content_type"`
	DeclaredSizeBytes  int64           `json:"declared_size_bytes"`
	ReservedBytes      int64           `json:"reserved_bytes"`
	SHA256             string          `json:"sha256,omitempty"`
	Encrypted          bool            `json:"encrypted"`
	EncryptionMetadata json.RawMessage `json:"encryption_metadata,omitempty"`
	Status             string          `json:"status"`
	IdempotencyKey     string          `json:"idempotency_key"`
	CID                string          `json:"cid,omitempty"`
	Error              string          `json:"error,omitempty"`
	ExpiresAt          time.Time       `json:"expires_at"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// DropFile is the user-visible logical file backed by one or more physical pins.
type DropFile struct {
	ID                 string          `json:"id"`
	UploadID           string          `json:"upload_id,omitempty"`
	OwnerUserID        string          `json:"owner_user_id"`
	OrgID              string          `json:"org_id,omitempty"`
	EntitlementOrgID   string          `json:"entitlement_org_id,omitempty"`
	NodeID             string          `json:"node_id"`
	StorageScope       string          `json:"storage_scope"`
	CID                string          `json:"cid"`
	Filename           string          `json:"filename"`
	ContentType        string          `json:"content_type"`
	SizeBytes          int64           `json:"size_bytes"`
	Visibility         string          `json:"visibility"`
	Encrypted          bool            `json:"encrypted"`
	EncryptionMetadata json.RawMessage `json:"encryption_metadata,omitempty"`
	Status             string          `json:"status"`
	CreatedAt          time.Time       `json:"created_at"`
	DeletedAt          *time.Time      `json:"deleted_at,omitempty"`
}

// DropPin is the physical pin state on one node, designed for future replication.
type DropPin struct {
	ID        string     `json:"id"`
	FileID    string     `json:"file_id"`
	NodeID    string     `json:"node_id"`
	CID       string     `json:"cid"`
	Status    string     `json:"status"`
	LastError string     `json:"last_error,omitempty"`
	PinnedAt  *time.Time `json:"pinned_at,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// DropQuotaUsage is an atomically maintained per-principal counter.
type DropQuotaUsage struct {
	PrincipalType string    `json:"principal_type"`
	PrincipalID   string    `json:"principal_id"`
	UsedBytes     int64     `json:"used_bytes"`
	ReservedBytes int64     `json:"reserved_bytes"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NodeDropStatus is the latest gateway-internal Drop status for a node. Exact
// capacity lives here (PostgreSQL), never as Prometheus labels.
type NodeDropStatus struct {
	NodeID               string     `json:"node_id"`
	Enabled              bool       `json:"enabled"`
	AcceptsPublicUploads bool       `json:"accepts_public_uploads"`
	WebUIAvailable       bool       `json:"webui_available"`
	State                string     `json:"state"`
	KuboVersion          string     `json:"kubo_version,omitempty"`
	RepoSizeBytes        int64      `json:"repo_size_bytes"`
	StorageMaxBytes      int64      `json:"storage_max_bytes"`
	NumObjects           int64      `json:"num_objects"`
	ReservedBytes        int64      `json:"reserved_bytes"`
	LastReportedAt       *time.Time `json:"last_reported_at,omitempty"`
}

// DropCryptoProfile stores only the encrypted backup and public metadata of a
// user's client-side Drop vault. The gateway never receives the recovery secret
// or the plaintext vault.
type DropCryptoProfile struct {
	UserID         string          `json:"user_id"`
	Version        int             `json:"version"`
	PublicKey      string          `json:"public_key,omitempty"`
	EncryptedVault string          `json:"encrypted_vault"`
	KDFMetadata    json.RawMessage `json:"kdf_metadata,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
