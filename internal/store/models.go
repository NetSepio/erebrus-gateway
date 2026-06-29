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
	OrgRoleAdmin        = "admin"
	OrgRoleNodeOperator = "node_operator"
	OrgRoleMember       = "member"
	OrgRoleViewer       = "viewer"
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

// User is a gateway account, keyed by wallet. An optional verified email may be
// linked for perks/recovery (never required to use the VPN).
type User struct {
	ID            string    `json:"id"`
	WalletAddress string    `json:"wallet_address,omitempty"`
	Chain         string    `json:"chain,omitempty"`
	Role          string    `json:"role"`
	Email         string    `json:"email,omitempty"`
	EmailVerified bool      `json:"email_verified"`
	Name          string    `json:"name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Node is a registered VPN node and its latest control-plane snapshot.
type Node struct {
	ID            string          `json:"id"`
	PeerID        string          `json:"peer_id"`
	DID           string          `json:"did"`
	WalletAddress string          `json:"wallet_address,omitempty"`
	Chain         string          `json:"chain,omitempty"` // SOLANA | ETHEREUM (set at enrollment)
	OrgID         string          `json:"org_id,omitempty"`
	AccessMode    string          `json:"access_mode"`
	MinTier       int             `json:"min_tier"`
	Name          string          `json:"name"`
	Region        string          `json:"region"`
	Zone          string          `json:"zone,omitempty"`
	IP            string          `json:"ip,omitempty"` // never serialized publicly
	IPHash        string          `json:"ip_hash,omitempty"`
	Spec          json.RawMessage `json:"spec"`
	Capabilities  json.RawMessage `json:"capabilities"`
	Endpoints     json.RawMessage `json:"endpoints"`
	Protocols     []string        `json:"protocols"`
	Status        string          `json:"status"`
	Load          json.RawMessage `json:"load"`
	Speedtest     json.RawMessage `json:"speedtest"`
	RxBytes       int64           `json:"rx_bytes"`
	TxBytes       int64           `json:"tx_bytes"`
	Version       string          `json:"version"`
	LastHeartbeat     *time.Time `json:"last_heartbeat,omitempty"`
	LastPeerHandshake *time.Time `json:"last_peer_handshake,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
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
	ID               string     `json:"id"`
	PlanID           string     `json:"plan_id"`
	Source           string     `json:"source"`
	Status           string     `json:"status"`
	CurrentPeriodEnd *time.Time `json:"current_period_end,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// Org is a workspace; members and API keys operate within it.
type Org struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Slug                  string    `json:"slug"`
	Plan                  string    `json:"plan"`
	BillingStatus         string    `json:"billing_status"`
	VerificationStatus    string    `json:"verification_status"`
	PublicProfileEnabled  bool      `json:"public_profile_enabled"`
	OwnerUserID           string    `json:"owner_user_id"`
	Role                  string    `json:"role,omitempty"` // caller's role, when listed for a user
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
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
	ID                         string    `json:"id"`
	OrgID                      string    `json:"org_id"`
	Plan                       string    `json:"plan"`
	PaidSeatsIncluded          int       `json:"paid_seats_included"`
	ManagedVPNNodesIncluded    int       `json:"managed_vpn_nodes_included"`
	ShieldInstancesIncluded    int       `json:"shield_instances_included"`
	SentinelLicensesIncluded   int       `json:"sentinel_licenses_included"`
	PublicNodeAccessTier       string    `json:"public_node_access_tier,omitempty"`
	APIQuotaMonthly            *int      `json:"api_quota_monthly,omitempty"`
	BandwidthPolicy            string    `json:"bandwidth_policy,omitempty"`
	SupportTier                string    `json:"support_tier,omitempty"`
	AuditLogsEnabled           bool      `json:"audit_logs_enabled"`
	AdvancedAnalyticsEnabled   bool      `json:"advanced_analytics_enabled"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

// Member is a user's membership in an org.
type Member struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	WalletAddress string    `json:"wallet_address,omitempty"`
	Role          string    `json:"role"`
	SeatTier      string    `json:"seat_tier"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
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