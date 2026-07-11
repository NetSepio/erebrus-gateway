package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

const dropFileCols = `id, COALESCE(upload_id::text,''), owner_user_id, COALESCE(org_id::text,''),
	COALESCE(entitlement_org_id::text,''), node_id, storage_scope, cid, filename, content_type,
	size_bytes, visibility, encrypted, encryption_metadata, status, created_at, deleted_at`

func dropFileScan(f *DropFile) []any {
	return []any{
		&f.ID, &f.UploadID, &f.OwnerUserID, &f.OrgID, &f.EntitlementOrgID, &f.NodeID,
		&f.StorageScope, &f.CID, &f.Filename, &f.ContentType, &f.SizeBytes, &f.Visibility,
		&f.Encrypted, &f.EncryptionMetadata, &f.Status, &f.CreatedAt, &f.DeletedAt,
	}
}

// CommitDropUpload finalizes a reservation: it records the logical file, its pin,
// converts the quota/node reservation into used bytes, and marks the upload
// committed. It is idempotent — re-committing an already-committed upload returns
// the existing file rather than double-counting quota or creating a second pin.
func (s *Store) CommitDropUpload(ctx context.Context, userID, uploadID, cid string, finalSize int64) (*DropFile, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	u := &DropUpload{}
	err = tx.QueryRowContext(ctx,
		`SELECT `+dropUploadCols+` FROM drop_uploads
		 WHERE id = $1::uuid AND owner_user_id = $2::uuid FOR UPDATE`,
		uploadID, userID).Scan(dropUploadScan(u)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if u.Status == DropUploadCommitted {
		f, ferr := scanDropFileByUpload(ctx, tx, uploadID)
		if ferr != nil {
			return nil, ferr
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return f, nil
	}
	if u.Status != DropUploadReserved && u.Status != DropUploadUploading {
		return nil, ErrConflict
	}
	if finalSize <= 0 {
		finalSize = u.DeclaredSizeBytes
	}

	f := &DropFile{}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO drop_files (
			upload_id, owner_user_id, org_id, entitlement_org_id, node_id, storage_scope,
			cid, filename, content_type, size_bytes, visibility, encrypted, encryption_metadata, status)
		 VALUES ($1::uuid, $2::uuid, NULLIF($3,'')::uuid, NULLIF($4,'')::uuid, $5, $6,
		         $7, $8, $9, $10, $11, $12, $13, $14)
		 RETURNING `+dropFileCols,
		u.ID, u.OwnerUserID, u.OrgID, u.EntitlementOrgID, u.NodeID, u.StorageScope,
		cid, u.Filename, u.ContentType, finalSize, u.Visibility, u.Encrypted,
		nullJSON(u.EncryptionMetadata), DropFileActive).
		Scan(dropFileScan(f)...)
	if err != nil {
		return nil, err
	}

	// One pin row per (file, node). Duplicate CID references on the same node are
	// tolerated; a physical unpin is only issued once the final reference is
	// deleted (see MarkDropFileDeletePending). The gateway pins synchronously
	// before commit, so the pin is recorded as pinned.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO drop_pins (file_id, node_id, cid, status, pinned_at)
		 VALUES ($1::uuid, $2, $3, $4, now())
		 ON CONFLICT (file_id, node_id) DO UPDATE SET cid=EXCLUDED.cid, status=EXCLUDED.status, updated_at=now()`,
		f.ID, u.NodeID, cid, DropPinPinned); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE drop_uploads SET status=$2, cid=$3, updated_at=now() WHERE id=$1::uuid`,
		u.ID, DropUploadCommitted, cid); err != nil {
		return nil, err
	}

	// Convert reservation → used.
	if u.StorageScope == DropScopePublic {
		if err := adjustQuotaTx(ctx, tx, DropPrincipalUser, u.OwnerUserID, finalSize, -u.ReservedBytes); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE node_drop_status SET reserved_bytes = GREATEST(reserved_bytes - $2, 0), updated_at=now()
		 WHERE node_id=$1`, u.NodeID, u.ReservedBytes); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return f, nil
}

// ReleaseDropReservation rolls back a reservation's quota and node holds for an
// upload that will not commit (expired/failed). Idempotent: it only releases a
// reservation still in a reservable state.
func (s *Store) ReleaseDropReservation(ctx context.Context, uploadID, status string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	u := &DropUpload{}
	err = tx.QueryRowContext(ctx,
		`SELECT `+dropUploadCols+` FROM drop_uploads WHERE id=$1::uuid FOR UPDATE`, uploadID).
		Scan(dropUploadScan(u)...)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if u.Status != DropUploadReserved && u.Status != DropUploadUploading {
		return tx.Commit() // already terminal; nothing to release
	}
	if u.StorageScope == DropScopePublic {
		if err := adjustQuotaTx(ctx, tx, DropPrincipalUser, u.OwnerUserID, 0, -u.ReservedBytes); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE node_drop_status SET reserved_bytes = GREATEST(reserved_bytes - $2, 0), updated_at=now()
		 WHERE node_id=$1`, u.NodeID, u.ReservedBytes); err != nil {
		return err
	}
	if status == "" {
		status = DropUploadExpired
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE drop_uploads SET status=$2, reserved_bytes=0, updated_at=now() WHERE id=$1::uuid`,
		uploadID, status); err != nil {
		return err
	}
	return tx.Commit()
}

// ExpireDropReservations releases reservations whose TTL has elapsed. Returns the
// number released. Safe to run repeatedly (idempotent per reservation).
func (s *Store) ExpireDropReservations(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text FROM drop_uploads
		 WHERE status IN ($1,$2) AND expires_at < now()
		 ORDER BY expires_at ASC LIMIT $3`,
		DropUploadReserved, DropUploadUploading, limit)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		if err := s.ReleaseDropReservation(ctx, id, DropUploadExpired); err == nil {
			n++
		}
	}
	return n, nil
}

// adjustQuotaTx applies deltas to a principal's used/reserved counters, clamping
// at zero.
func adjustQuotaTx(ctx context.Context, tx *sql.Tx, principalType, principalID string, usedDelta, reservedDelta int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO drop_quota_usage (principal_type, principal_id, used_bytes, reserved_bytes)
		 VALUES ($1,$2, GREATEST($3,0), GREATEST($4,0))
		 ON CONFLICT (principal_type, principal_id) DO UPDATE
		 SET used_bytes = GREATEST(drop_quota_usage.used_bytes + $3, 0),
		     reserved_bytes = GREATEST(drop_quota_usage.reserved_bytes + $4, 0),
		     updated_at = now()`,
		principalType, principalID, usedDelta, reservedDelta)
	return err
}

func scanDropFileByUpload(ctx context.Context, tx *sql.Tx, uploadID string) (*DropFile, error) {
	f := &DropFile{}
	err := tx.QueryRowContext(ctx,
		`SELECT `+dropFileCols+` FROM drop_files WHERE upload_id=$1::uuid ORDER BY created_at DESC LIMIT 1`,
		uploadID).Scan(dropFileScan(f)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return f, err
}

// ListDropFilesByUser returns a user's active files, newest first.
func (s *Store) ListDropFilesByUser(ctx context.Context, userID string) ([]*DropFile, error) {
	return s.queryDropFiles(ctx,
		`SELECT `+dropFileCols+` FROM drop_files
		 WHERE owner_user_id=$1::uuid AND status <> $2 ORDER BY created_at DESC`,
		userID, DropFileDeleted)
}

// ListDropFilesByOrg returns a private org's active files, newest first.
func (s *Store) ListDropFilesByOrg(ctx context.Context, orgID string) ([]*DropFile, error) {
	return s.queryDropFiles(ctx,
		`SELECT `+dropFileCols+` FROM drop_files
		 WHERE org_id=$1::uuid AND status <> $2 ORDER BY created_at DESC`,
		orgID, DropFileDeleted)
}

func (s *Store) queryDropFiles(ctx context.Context, q string, args ...any) ([]*DropFile, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DropFile
	for rows.Next() {
		f := &DropFile{}
		if err := rows.Scan(dropFileScan(f)...); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetDropFile returns a single active file by id (no ownership filter — callers
// must authorize access based on the returned owner/org/visibility fields).
func (s *Store) GetDropFile(ctx context.Context, fileID string) (*DropFile, error) {
	f := &DropFile{}
	err := s.db.QueryRowContext(ctx,
		`SELECT `+dropFileCols+` FROM drop_files WHERE id=$1::uuid`, fileID).Scan(dropFileScan(f)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

// CountActivePinRefs returns how many non-deleted files still reference a CID on
// a node. Used to decide when a physical unpin is safe.
func (s *Store) CountActivePinRefs(ctx context.Context, nodeID, cid, excludeFileID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM drop_files
		 WHERE node_id=$1 AND cid=$2 AND status <> $3 AND id <> COALESCE(NULLIF($4,'')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)`,
		nodeID, cid, DropFileDeleted, excludeFileID).Scan(&n)
	return n, err
}

// MarkDropFileDeletePending flags a file for deletion, releases its quota, and
// reports whether the physical CID still has other references on the node (i.e.
// whether an unpin should be skipped). Idempotent.
func (s *Store) MarkDropFileDeletePending(ctx context.Context, fileID string) (unpinNeeded bool, f *DropFile, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	f = &DropFile{}
	err = tx.QueryRowContext(ctx,
		`SELECT `+dropFileCols+` FROM drop_files WHERE id=$1::uuid FOR UPDATE`, fileID).Scan(dropFileScan(f)...)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, ErrNotFound
	}
	if err != nil {
		return false, nil, err
	}
	if f.Status == DropFileDeleted || f.Status == DropFileDeletePending {
		return false, f, tx.Commit() // already releasing/released
	}

	// Release quota once, at the transition out of active.
	if f.StorageScope == DropScopePublic {
		if err := adjustQuotaTx(ctx, tx, DropPrincipalUser, f.OwnerUserID, -f.SizeBytes, 0); err != nil {
			return false, nil, err
		}
	}

	var otherRefs int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM drop_files
		 WHERE node_id=$1 AND cid=$2 AND status NOT IN ($3,$4) AND id <> $5::uuid`,
		f.NodeID, f.CID, DropFileDeleted, DropFileDeletePending, f.ID).Scan(&otherRefs); err != nil {
		return false, nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE drop_files SET status=$2, deleted_at=now() WHERE id=$1::uuid`,
		f.ID, DropFileDeletePending); err != nil {
		return false, nil, err
	}
	pinStatus := DropPinUnpinPending
	if otherRefs > 0 {
		pinStatus = DropPinUnpinned // reference remains; skip physical unpin
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE drop_pins SET status=$2, updated_at=now() WHERE file_id=$1::uuid`, f.ID, pinStatus); err != nil {
		return false, nil, err
	}
	if err := tx.Commit(); err != nil {
		return false, nil, err
	}
	return otherRefs == 0, f, nil
}

// FinalizeDropFileDeletion marks a delete-pending file fully deleted after any
// physical unpin has been attempted. Idempotent.
func (s *Store) FinalizeDropFileDeletion(ctx context.Context, fileID string, unpinned bool) error {
	pinStatus := DropPinUnpinned
	if !unpinned {
		pinStatus = DropPinError
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx,
		`UPDATE drop_pins SET status=$2, updated_at=now() WHERE file_id=$1::uuid`, fileID, pinStatus); err != nil {
		return err
	}
	if unpinned {
		if _, err := tx.ExecContext(ctx,
			`UPDATE drop_files SET status=$2 WHERE id=$1::uuid AND status=$3`,
			fileID, DropFileDeleted, DropFileDeletePending); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ── crypto profiles (encrypted client-side vault) ────

// GetDropCryptoProfile returns a user's encrypted Drop vault, or ErrNotFound.
func (s *Store) GetDropCryptoProfile(ctx context.Context, userID string) (*DropCryptoProfile, error) {
	p := &DropCryptoProfile{}
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, version, COALESCE(public_key,''), encrypted_vault, kdf_metadata, created_at, updated_at
		 FROM drop_crypto_profiles WHERE user_id=$1::uuid`, userID).
		Scan(&p.UserID, &p.Version, &p.PublicKey, &p.EncryptedVault, &p.KDFMetadata, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// PutDropCryptoProfile upserts a user's encrypted Drop vault with optimistic
// version guarding: the write only succeeds when expectedVersion matches the
// stored version (0 for a first write). Returns ErrConflict on a version clash.
func (s *Store) PutDropCryptoProfile(ctx context.Context, userID, publicKey, encryptedVault string, kdf json.RawMessage, expectedVersion int) (*DropCryptoProfile, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var current int
	err = tx.QueryRowContext(ctx,
		`SELECT version FROM drop_crypto_profiles WHERE user_id=$1::uuid FOR UPDATE`, userID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		current = 0
	} else if err != nil {
		return nil, err
	}
	if current != expectedVersion {
		return nil, ErrConflict
	}
	next := current + 1
	p := &DropCryptoProfile{}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO drop_crypto_profiles (user_id, version, public_key, encrypted_vault, kdf_metadata)
		 VALUES ($1::uuid, $2, NULLIF($3,''), $4, $5)
		 ON CONFLICT (user_id) DO UPDATE
		 SET version=EXCLUDED.version, public_key=EXCLUDED.public_key,
		     encrypted_vault=EXCLUDED.encrypted_vault, kdf_metadata=EXCLUDED.kdf_metadata, updated_at=now()
		 RETURNING user_id, version, COALESCE(public_key,''), encrypted_vault, kdf_metadata, created_at, updated_at`,
		userID, next, publicKey, encryptedVault, nullJSON(kdf)).
		Scan(&p.UserID, &p.Version, &p.PublicKey, &p.EncryptedVault, &p.KDFMetadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p, nil
}
