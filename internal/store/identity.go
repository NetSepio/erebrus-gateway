package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrWalletTaken is returned when attaching a wallet already owned by another user.
var ErrWalletTaken = errors.New("wallet already linked to another account")

const userReturnCols = `id, COALESCE(wallet_address,''), COALESCE(chain,''), role,
	COALESCE(email,''), email_verified, COALESCE(name,''), created_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.WalletAddress, &u.Chain, &u.Role,
		&u.Email, &u.EmailVerified, &u.Name, &u.CreatedAt)
	return &u, err
}

type insertUserParams struct {
	Wallet        string
	Chain         string
	Email         string
	EmailVerified bool
	AdminWallet   string
}

// insertUser creates a new user. Any of wallet/email may be empty (wallet-optional
// accounts). The admin wallet, when matched, is granted role=admin.
func (s *Store) insertUser(ctx context.Context, p insertUserParams) (*User, error) {
	role := "user"
	if p.AdminWallet != "" && p.Wallet != "" && eqFold(p.Wallet, p.AdminWallet) {
		role = "admin"
	}
	return scanUser(s.db.QueryRowContext(ctx,
		`INSERT INTO users (wallet_address, chain, email, email_verified, role)
		 VALUES (NULLIF($1,''), NULLIF($2,''), NULLIF($3,''), $4, $5)
		 RETURNING `+userReturnCols,
		p.Wallet, p.Chain, p.Email, p.EmailVerified, role))
}

// userIDByVerifiedEmail resolves a verified email to its user id (no create).
func (s *Store) userIDByVerifiedEmail(ctx context.Context, email string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE lower(email) = lower($1) AND email_verified`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// ResolveOrCreateUserByEmail returns the account owning a verified email, or
// creates a new wallet-less account with that verified email. Used by email-OTP
// login. The bool reports whether the user was newly created.
func (s *Store) ResolveOrCreateUserByEmail(ctx context.Context, email string) (*User, bool, error) {
	if id, err := s.userIDByVerifiedEmail(ctx, email); err == nil {
		u, gerr := s.GetUser(ctx, id)
		return u, false, gerr
	} else if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	u, err := s.insertUser(ctx, insertUserParams{Email: email, EmailVerified: true})
	if err != nil {
		return nil, false, err
	}
	return u, true, nil
}

// ResolveOrCreateUserBySocial maps a Google/Apple identity to an account. Order:
// (1) an existing social_accounts(provider, providerID) link; (2) else a verified
// email match (links the new identity to that account); (3) else a brand-new
// wallet-less account. The bool reports whether the user was newly created.
func (s *Store) ResolveOrCreateUserBySocial(ctx context.Context, provider, providerID, email, handle string) (*User, bool, error) {
	var uid string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM social_accounts WHERE provider = $1 AND provider_id = $2`,
		provider, providerID).Scan(&uid)
	if err == nil {
		u, gerr := s.GetUser(ctx, uid)
		return u, false, gerr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}

	if email != "" {
		if id, verr := s.userIDByVerifiedEmail(ctx, email); verr == nil {
			if _, lerr := s.LinkSocialAccount(ctx, id, provider, providerID, handle); lerr != nil && !errors.Is(lerr, ErrSocialTaken) {
				return nil, false, lerr
			}
			u, gerr := s.GetUser(ctx, id)
			return u, false, gerr
		} else if !errors.Is(verr, ErrNotFound) {
			return nil, false, verr
		}
	}

	u, err := s.insertUser(ctx, insertUserParams{Email: email, EmailVerified: email != ""})
	if err != nil {
		return nil, false, err
	}
	if _, err := s.LinkSocialAccount(ctx, u.ID, provider, providerID, handle); err != nil && !errors.Is(err, ErrSocialTaken) {
		return nil, false, err
	}
	return u, true, nil
}

// AttachWalletToUser links a wallet to an account (e.g. a social-first user adds
// a wallet). Fails with ErrWalletTaken if another account already owns it.
func (s *Store) AttachWalletToUser(ctx context.Context, userID, wallet, chain string) error {
	var owner string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE lower(wallet_address) = lower($1)`, wallet).Scan(&owner)
	if err == nil && owner != userID {
		return ErrWalletTaken
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET wallet_address = $2, chain = NULLIF($3,''), updated_at = now() WHERE id = $1`,
		userID, wallet, chain)
	return err
}

// ── email login OTPs (userless) ──────────────────────────────────────────────

// UpsertLoginOTP stores (or replaces) the pending login code for an email and
// opportunistically prunes expired rows.
func (s *Store) UpsertLoginOTP(ctx context.Context, email, codeHash string, ttl time.Duration) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO email_login_otps (email, code_hash, attempts, expires_at)
		 VALUES ($1, $2, 0, $3)
		 ON CONFLICT (email) DO UPDATE SET
			code_hash = EXCLUDED.code_hash, attempts = 0,
			expires_at = EXCLUDED.expires_at, created_at = now()`,
		email, codeHash, time.Now().Add(ttl)); err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM email_login_otps WHERE expires_at < now()`)
	return nil
}

// LoginOTP is a pending email-login code.
type LoginOTP struct {
	CodeHash  string
	Attempts  int
	CreatedAt time.Time
}

// GetLoginOTP returns the unexpired pending code for an email.
func (s *Store) GetLoginOTP(ctx context.Context, email string) (*LoginOTP, error) {
	var o LoginOTP
	err := s.db.QueryRowContext(ctx,
		`SELECT code_hash, attempts, created_at FROM email_login_otps
		 WHERE email = $1 AND expires_at > now()`, email).
		Scan(&o.CodeHash, &o.Attempts, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &o, err
}

// BumpLoginOTPAttempts increments and returns the attempt count.
func (s *Store) BumpLoginOTPAttempts(ctx context.Context, email string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`UPDATE email_login_otps SET attempts = attempts + 1 WHERE email = $1 RETURNING attempts`,
		email).Scan(&n)
	return n, err
}

// DeleteLoginOTP removes the pending code for an email (after success/lockout).
func (s *Store) DeleteLoginOTP(ctx context.Context, email string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM email_login_otps WHERE email = $1`, email)
	return err
}
