package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// EmailOTP is a pending email-verification code (stored hashed).
type EmailOTP struct {
	ID        string
	UserID    string
	Email     string
	CodeHash  string
	Attempts  int
	ExpiresAt time.Time
	CreatedAt time.Time
}

// EmailOwner returns the id of the user that has linked an email, or ErrNotFound.
// Used to enforce one-email-one-account. Comparison is case-insensitive.
func (s *Store) EmailOwner(ctx context.Context, email string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE email IS NOT NULL AND lower(email) = lower($1)`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// CreateEmailOTP stores a hashed verification code with a TTL.
func (s *Store) CreateEmailOTP(ctx context.Context, userID, email, codeHash string, ttl time.Duration) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO email_otps (user_id, email, code_hash, expires_at)
		 VALUES ($1, lower($2), $3, $4)`,
		userID, email, codeHash, time.Now().Add(ttl))
	return err
}

// LatestEmailOTP returns the most recent unexpired code for (user, email).
func (s *Store) LatestEmailOTP(ctx context.Context, userID, email string) (*EmailOTP, error) {
	var o EmailOTP
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, email, code_hash, attempts, expires_at, created_at
		   FROM email_otps
		  WHERE user_id = $1 AND lower(email) = lower($2) AND expires_at > now()
		  ORDER BY created_at DESC
		  LIMIT 1`, userID, email).
		Scan(&o.ID, &o.UserID, &o.Email, &o.CodeHash, &o.Attempts, &o.ExpiresAt, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// BumpEmailOTPAttempts increments the attempt counter and returns the new value.
func (s *Store) BumpEmailOTPAttempts(ctx context.Context, id string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`UPDATE email_otps SET attempts = attempts + 1 WHERE id = $1 RETURNING attempts`, id).Scan(&n)
	return n, err
}

// DeleteEmailOTPs removes all pending codes for (user, email) — on success or reset.
func (s *Store) DeleteEmailOTPs(ctx context.Context, userID, email string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM email_otps WHERE user_id = $1 AND lower(email) = lower($2)`, userID, email)
	return err
}

// SetUserEmailVerified links a verified email to the user (email stored lowercased).
func (s *Store) SetUserEmailVerified(ctx context.Context, userID, email string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET email = lower($2), email_verified = true, updated_at = now() WHERE id = $1`,
		userID, email)
	return err
}

// PurgeExpiredEmailOTPs deletes stale codes (called periodically).
func (s *Store) PurgeExpiredEmailOTPs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM email_otps WHERE expires_at < now()`)
	return err
}
