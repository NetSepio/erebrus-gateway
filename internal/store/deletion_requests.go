package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// DeletionRequest is a user request to delete their account and associated data.
type DeletionRequest struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id,omitempty"`
	WalletAddress string    `json:"wallet_address,omitempty"`
	Email         string    `json:"email,omitempty"`
	Name          string    `json:"name,omitempty"`
	Status        string    `json:"status"`
	RequestedAt   time.Time `json:"requested_at"`
	FulfilledAt   *time.Time `json:"fulfilled_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// CreateDeletionRequest inserts a new pending deletion request for a user.
// If a pending request already exists for the user, it returns the existing one.
func (s *Store) CreateDeletionRequest(ctx context.Context, userID, wallet, email, name string) (string, error) {
	var existingID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM deletion_requests WHERE user_id = $1 AND status = 'pending'`, userID).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	var id string
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO deletion_requests (user_id, wallet_address, email, name, status, requested_at)
		 VALUES ($1, $2, $3, $4, 'pending', now())
		 RETURNING id`,
		userID, wallet, email, name).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// ListDeletionRequests returns pending and fulfilled deletion requests ordered by requested_at DESC.
func (s *Store) ListDeletionRequests(ctx context.Context, limit, offset int) ([]DeletionRequest, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM deletion_requests`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, wallet_address, email, name, status, requested_at, fulfilled_at, created_at
		 FROM deletion_requests
		 ORDER BY requested_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []DeletionRequest
	for rows.Next() {
		var r DeletionRequest
		var userID sql.NullString
		var fulfilledAt sql.NullTime
		if err := rows.Scan(&r.ID, &userID, &r.WalletAddress, &r.Email, &r.Name, &r.Status, &r.RequestedAt, &fulfilledAt, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		r.UserID = userID.String
		r.FulfilledAt = nullableTime(fulfilledAt)
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// HasPendingDeletionRequest reports whether a user has an open deletion request.
func (s *Store) HasPendingDeletionRequest(ctx context.Context, userID string) (bool, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM deletion_requests WHERE user_id = $1 AND status = 'pending')`, userID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// GetDeletionRequestByUserID returns the pending deletion request for a user, if any.
func (s *Store) GetDeletionRequestByUserID(ctx context.Context, userID string) (*DeletionRequest, error) {
	var r DeletionRequest
	var userIDVal sql.NullString
	var fulfilledAt sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, wallet_address, email, name, status, requested_at, fulfilled_at, created_at
		 FROM deletion_requests WHERE user_id = $1 AND status = 'pending'`, userID).
		Scan(&r.ID, &userIDVal, &r.WalletAddress, &r.Email, &r.Name, &r.Status, &r.RequestedAt, &fulfilledAt, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.UserID = userIDVal.String
	r.FulfilledAt = nullableTime(fulfilledAt)
	return &r, nil
}

// GetDeletionRequest returns a single deletion request by id.
func (s *Store) GetDeletionRequest(ctx context.Context, id string) (*DeletionRequest, error) {
	var r DeletionRequest
	var userID sql.NullString
	var fulfilledAt sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, wallet_address, email, name, status, requested_at, fulfilled_at, created_at
		 FROM deletion_requests WHERE id = $1`, id).
		Scan(&r.ID, &userID, &r.WalletAddress, &r.Email, &r.Name, &r.Status, &r.RequestedAt, &fulfilledAt, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.UserID = userID.String
	r.FulfilledAt = nullableTime(fulfilledAt)
	return &r, nil
}

// FulfillDeletionRequest marks a request as fulfilled and deletes the user in one transaction.
// It returns the email to notify after the user is deleted.
func (s *Store) FulfillDeletionRequest(ctx context.Context, id string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var userID, email string
	if err := tx.QueryRowContext(ctx,
		`SELECT user_id, email FROM deletion_requests WHERE id = $1 AND status = 'pending'`, id).
		Scan(&userID, &email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	if userID != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			return "", err
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE deletion_requests SET status = 'fulfilled', fulfilled_at = now(), updated_at = now() WHERE id = $1`, id); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return email, nil
}

func nullableTime(nt sql.NullTime) *time.Time {
	if nt.Valid {
		return &nt.Time
	}
	return nil
}
