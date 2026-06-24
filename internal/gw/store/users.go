package store

import (
	"context"
	"database/sql"
	"errors"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// UpsertUserByWallet returns the user for a wallet, creating it on first login.
// The configured admin wallet is granted role=admin on creation.
func (s *Store) UpsertUserByWallet(ctx context.Context, wallet, chain, adminWallet string) (*User, error) {
	role := "user"
	if adminWallet != "" && eqFold(wallet, adminWallet) {
		role = "admin"
	}
	var u User
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO users (wallet_address, chain, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (wallet_address) DO UPDATE SET updated_at = now()
		 RETURNING id, wallet_address, chain, role, COALESCE(email,''), email_verified, COALESCE(name,''), created_at`,
		wallet, chain, role).
		Scan(&u.ID, &u.WalletAddress, &u.Chain, &u.Role, &u.Email, &u.EmailVerified, &u.Name, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUser returns a user by id.
func (s *Store) GetUser(ctx context.Context, id string) (*User, error) {
	var u User
	var chain sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(wallet_address,''), COALESCE(chain,''), role,
		        COALESCE(email,''), email_verified, COALESCE(name,''), created_at
		 FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.WalletAddress, &chain.String, &u.Role, &u.Email, &u.EmailVerified, &u.Name, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Chain = chain.String
	return &u, nil
}

// UpdateProfile updates the mutable profile fields. Email is intentionally NOT
// settable here — it is only linked through the verified OTP flow (auth/email),
// so a linked email is always proven (no unverified squatting on the UNIQUE col).
func (s *Store) UpdateProfile(ctx context.Context, id, name string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET name = NULLIF($2,''), updated_at = now() WHERE id = $1`,
		id, name)
	return err
}

// CountUsers returns the total user count (admin stats).
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

// ListUsers returns a page of users (admin).
func (s *Store) ListUsers(ctx context.Context, limit, offset int) ([]User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(wallet_address,''), COALESCE(chain,''), role,
		        COALESCE(email,''), email_verified, COALESCE(name,''), created_at
		 FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.WalletAddress, &u.Chain, &u.Role, &u.Email, &u.EmailVerified, &u.Name, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func eqFold(a, b string) bool {
	return len(a) == len(b) && toLower(a) == toLower(b)
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
