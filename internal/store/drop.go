package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Drop entitlement/quota errors.
var (
	// ErrDropQuotaExceeded means the reservation would push a principal past its
	// public storage quota (or the per-file ceiling).
	ErrDropQuotaExceeded = errors.New("drop quota exceeded")
	// ErrDropNodeCapacity means the target node cannot fit the reservation.
	ErrDropNodeCapacity = errors.New("drop node capacity exhausted")
	// ErrDropNodeUnavailable means the node is not currently accepting Drop uploads.
	ErrDropNodeUnavailable = errors.New("drop node unavailable")
)

// dropTierOrder ranks effective Drop tiers low→high.
var dropTierOrder = []string{
	DropTierFree, DropTierStarter, DropTierPro, DropTierBusiness, DropTierEnterprise,
}

func dropTierRank(tier string) int {
	for i, t := range dropTierOrder {
		if t == tier {
			return i
		}
	}
	return -1
}

// NormalizeDropTier maps an arbitrary seat/plan tier onto a known Drop tier,
// defaulting unknown/empty values to Free.
func NormalizeDropTier(tier string) string {
	if dropTierRank(tier) >= 0 {
		return tier
	}
	return DropTierFree
}

// DefaultDropQuotaBytes is the built-in public storage policy, used as a fallback
// when drop_tier_limits has no row for a tier (should not happen post-migration).
func DefaultDropQuotaBytes(tier string) int64 {
	switch NormalizeDropTier(tier) {
	case DropTierStarter:
		return DropQuotaStarterBytes
	case DropTierPro:
		return DropQuotaProBytes
	case DropTierBusiness, DropTierEnterprise:
		return DropQuotaBusinessBytes
	default:
		return DropQuotaFreeBytes
	}
}

// DropEntitlement is the resolved public Drop entitlement for a user. It records
// provenance: which organization and seat tier supplied the effective tier.
type DropEntitlement struct {
	UserID             string `json:"user_id"`
	Tier               string `json:"tier"`
	EntitlementOrgID   string `json:"entitlement_org_id,omitempty"`
	SeatTier           string `json:"seat_tier"`
	PublicStorageBytes int64  `json:"public_storage_bytes"`
	MaxFileBytes       int64  `json:"max_file_bytes"`
}

// ResolveDropEntitlement is the canonical organization-only entitlement resolver.
// It finds the user's highest active organization seat across all active
// memberships and maps it to a public Drop tier + quota. Personal trials,
// per-user subscriptions, and NFT grants are intentionally ignored. Every user
// is expected to own a personal basic organization (see the 0026 backfill), so
// an authenticated user always resolves to at least the Free tier.
func (s *Store) ResolveDropEntitlement(ctx context.Context, userID string) (*DropEntitlement, error) {
	ent := &DropEntitlement{UserID: userID, Tier: DropTierFree, SeatTier: SeatTierFree}

	var orgID, seatTier sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT o.id::text, eff.seat_tier
		 FROM org_members m
		 JOIN orgs o ON o.id = m.org_id
		 CROSS JOIN LATERAL (
		     SELECT CASE
		         WHEN m.role IN ('owner','node_operator') THEN CASE o.plan
		             WHEN 'starter' THEN 'starter'
		             WHEN 'pro' THEN 'pro'
		             WHEN 'business' THEN 'business'
		             WHEN 'enterprise' THEN 'enterprise'
		             ELSE 'free' END
		         ELSE m.seat_tier END AS seat_tier
		 ) eff
		 WHERE m.user_id = $1::uuid AND m.status = 'active' AND o.billing_status = $2
		 ORDER BY array_position(
		     ARRAY['free','starter','pro','business','enterprise']::text[], eff.seat_tier) DESC NULLS LAST
		 LIMIT 1`, userID, OrgBillingActive).Scan(&orgID, &seatTier)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if orgID.Valid {
		ent.EntitlementOrgID = orgID.String
	}
	if seatTier.Valid {
		ent.SeatTier = NormalizeDropTier(seatTier.String)
	}
	ent.Tier = NormalizeDropTier(ent.SeatTier)

	limit, ferr := s.GetDropTierLimit(ctx, ent.Tier)
	if ferr != nil {
		return nil, ferr
	}
	ent.PublicStorageBytes = limit.PublicStorageBytes
	ent.MaxFileBytes = limit.MaxFileBytes
	return ent, nil
}

// GetDropTierLimit returns the storage policy for a tier, falling back to the
// built-in defaults when no row exists.
func (s *Store) GetDropTierLimit(ctx context.Context, tier string) (*DropTierLimit, error) {
	tier = NormalizeDropTier(tier)
	l := &DropTierLimit{Tier: tier}
	err := s.db.QueryRowContext(ctx,
		`SELECT tier, public_storage_bytes, max_file_bytes, created_at, updated_at
		 FROM drop_tier_limits WHERE tier = $1`, tier).
		Scan(&l.Tier, &l.PublicStorageBytes, &l.MaxFileBytes, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		l.PublicStorageBytes = DefaultDropQuotaBytes(tier)
		l.MaxFileBytes = DropMaxFileBytes
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	return l, nil
}

// GetDropQuotaUsage returns the current used/reserved counters for a principal,
// defaulting to zero when no row exists.
func (s *Store) GetDropQuotaUsage(ctx context.Context, principalType, principalID string) (*DropQuotaUsage, error) {
	u := &DropQuotaUsage{PrincipalType: principalType, PrincipalID: principalID}
	err := s.db.QueryRowContext(ctx,
		`SELECT used_bytes, reserved_bytes, updated_at
		 FROM drop_quota_usage WHERE principal_type = $1 AND principal_id = $2`,
		principalType, principalID).Scan(&u.UsedBytes, &u.ReservedBytes, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ReserveDropUploadInput carries the fields needed to open an upload reservation.
type ReserveDropUploadInput struct {
	UserID             string
	OrgID              string // private-org storage owner ("" for public)
	EntitlementOrgID   string // provenance of the effective tier
	NodeID             string
	Scope              string // public | private_org
	Visibility         string // public | private
	Filename           string
	ContentType        string
	DeclaredSize       int64
	SHA256             string
	Encrypted          bool
	EncryptionMetadata json.RawMessage
	IdempotencyKey     string
	TTL                time.Duration
}

// ReserveDropUpload atomically opens (or idempotently returns) an upload
// reservation. It locks the per-user public quota row and the node capacity row
// in a single transaction so concurrent uploads cannot oversubscribe either.
// Public uploads reserve against the user's quota; private-org uploads are
// governed only by node capacity.
func (s *Store) ReserveDropUpload(ctx context.Context, in ReserveDropUploadInput) (*DropUpload, error) {
	if in.TTL <= 0 {
		in.TTL = 30 * time.Minute
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// Idempotency: reuse a live reservation for the same key.
	if existing, ok, err := scanDropUploadByKey(ctx, tx, in.UserID, in.IdempotencyKey); err != nil {
		return nil, err
	} else if ok {
		if existing.Status == DropUploadReserved || existing.Status == DropUploadUploading ||
			existing.Status == DropUploadCommitted {
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return existing, nil
		}
		// Preserve terminal attempts (including any orphan-pin reconciliation
		// metadata) while freeing the caller's key for an explicit retry.
		if _, err := tx.ExecContext(ctx,
			`UPDATE drop_uploads
			 SET idempotency_key = idempotency_key || ':terminal:' || id::text,
			     updated_at = now()
			 WHERE id = $1::uuid`, existing.ID); err != nil {
			return nil, err
		}
	}

	// Per-file ceiling.
	if in.Scope == DropScopePublic {
		ent, err := s.resolveTierLimitTx(ctx, tx, in.EntitlementOrgID, in.UserID)
		if err != nil {
			return nil, err
		}
		if in.DeclaredSize > ent.MaxFileBytes {
			return nil, ErrDropQuotaExceeded
		}
		// Lock + check the user's public quota.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO drop_quota_usage (principal_type, principal_id) VALUES ($1,$2)
			 ON CONFLICT (principal_type, principal_id) DO NOTHING`,
			DropPrincipalUser, in.UserID); err != nil {
			return nil, err
		}
		var used, reserved int64
		if err := tx.QueryRowContext(ctx,
			`SELECT used_bytes, reserved_bytes FROM drop_quota_usage
			 WHERE principal_type=$1 AND principal_id=$2 FOR UPDATE`,
			DropPrincipalUser, in.UserID).Scan(&used, &reserved); err != nil {
			return nil, err
		}
		if used+reserved+in.DeclaredSize > ent.PublicStorageBytes {
			return nil, ErrDropQuotaExceeded
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE drop_quota_usage SET reserved_bytes = reserved_bytes + $3, updated_at = now()
			 WHERE principal_type=$1 AND principal_id=$2`,
			DropPrincipalUser, in.UserID, in.DeclaredSize); err != nil {
			return nil, err
		}
	}

	// Lock + check node capacity.
	if err := reserveNodeCapacityTx(ctx, tx, in.NodeID, in.Scope, in.DeclaredSize); err != nil {
		return nil, err
	}

	u := &DropUpload{}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO drop_uploads (
			owner_user_id, org_id, entitlement_org_id, node_id, storage_scope, visibility,
			filename, content_type, declared_size_bytes, reserved_bytes, sha256, encrypted,
			encryption_metadata, status, idempotency_key, expires_at)
		 VALUES ($1, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid, $4, $5, $6, $7, $8, $9, $9, NULLIF($10,''),
		         $11, $12, $13, $14, now() + ($15 || ' seconds')::interval)
		 RETURNING `+dropUploadCols,
		in.UserID, in.OrgID, in.EntitlementOrgID, in.NodeID, in.Scope, in.Visibility,
		in.Filename, in.ContentType, in.DeclaredSize, in.SHA256, in.Encrypted,
		nullJSON(in.EncryptionMetadata), DropUploadReserved, in.IdempotencyKey,
		int64(in.TTL.Seconds())).
		Scan(dropUploadScan(u)...)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return u, nil
}

// reserveNodeCapacityTx locks a node's Drop status row and reserves bytes when
// its advertised capacity is known. Rejects nodes that are not accepting uploads.
func reserveNodeCapacityTx(ctx context.Context, tx *sql.Tx, nodeID, scope string, size int64) error {
	var enabled, acceptsPublic bool
	var state, nodeStatus string
	var storageMax, repoSize, reserved int64
	err := tx.QueryRowContext(ctx,
		`SELECT d.enabled, d.accepts_public_uploads, d.state, d.storage_max_bytes,
		        d.repo_size_bytes, d.reserved_bytes, n.status
		 FROM node_drop_status d JOIN nodes n ON n.peer_id = d.node_id
		 WHERE d.node_id = $1 FOR UPDATE OF d`, nodeID).
		Scan(&enabled, &acceptsPublic, &state, &storageMax, &repoSize, &reserved, &nodeStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDropNodeUnavailable
	}
	if err != nil {
		return err
	}
	if !enabled || state != DropStateActive || nodeStatus != "online" {
		return ErrDropNodeUnavailable
	}
	if scope == DropScopePublic && !acceptsPublic {
		return ErrDropNodeUnavailable
	}
	if storageMax > 0 && repoSize+reserved+size > storageMax {
		return ErrDropNodeCapacity
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE node_drop_status SET reserved_bytes = reserved_bytes + $2, updated_at = now()
		 WHERE node_id = $1`, nodeID, size)
	return err
}

func (s *Store) resolveTierLimitTx(ctx context.Context, tx *sql.Tx, entitlementOrgID, userID string) (*DropTierLimit, error) {
	// Resolve outside the reservation lock is fine; the tier is stable enough for
	// one upload. Fall back to a fresh resolve when provenance is unknown.
	ent, err := s.ResolveDropEntitlement(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &DropTierLimit{Tier: ent.Tier, PublicStorageBytes: ent.PublicStorageBytes, MaxFileBytes: ent.MaxFileBytes}, nil
}

const dropUploadCols = `id, owner_user_id, COALESCE(org_id::text,''), COALESCE(entitlement_org_id::text,''),
	node_id, storage_scope, visibility, filename, content_type, declared_size_bytes, reserved_bytes,
	COALESCE(sha256,''), encrypted, encryption_metadata, status, idempotency_key, COALESCE(cid,''),
	COALESCE(error,''), expires_at, created_at, updated_at`

func dropUploadScan(u *DropUpload) []any {
	return []any{
		&u.ID, &u.OwnerUserID, &u.OrgID, &u.EntitlementOrgID, &u.NodeID, &u.StorageScope,
		&u.Visibility, &u.Filename, &u.ContentType, &u.DeclaredSizeBytes, &u.ReservedBytes,
		&u.SHA256, &u.Encrypted, &u.EncryptionMetadata, &u.Status, &u.IdempotencyKey, &u.CID,
		&u.Error, &u.ExpiresAt, &u.CreatedAt, &u.UpdatedAt,
	}
}

func scanDropUploadByKey(ctx context.Context, tx *sql.Tx, userID, key string) (*DropUpload, bool, error) {
	u := &DropUpload{}
	err := tx.QueryRowContext(ctx,
		`SELECT `+dropUploadCols+` FROM drop_uploads
		 WHERE owner_user_id = $1::uuid AND idempotency_key = $2 FOR UPDATE`,
		userID, key).Scan(dropUploadScan(u)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return u, true, nil
}

// GetDropUpload returns an upload owned by the user.
func (s *Store) GetDropUpload(ctx context.Context, userID, uploadID string) (*DropUpload, error) {
	u := &DropUpload{}
	err := s.db.QueryRowContext(ctx,
		`SELECT `+dropUploadCols+` FROM drop_uploads WHERE id = $1::uuid AND owner_user_id = $2::uuid`,
		uploadID, userID).Scan(dropUploadScan(u)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func nullJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}
