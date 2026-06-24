package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrSocialTaken is returned when a social account is already linked elsewhere.
var ErrSocialTaken = errors.New("social account linked to another user")

// SocialAccount is a verified third-party identity linked to a user.
type SocialAccount struct {
	Provider   string    `json:"provider"`
	ProviderID string    `json:"provider_id"`
	Handle     string    `json:"handle,omitempty"`
	VerifiedAt time.Time `json:"verified_at"`
}

// LinkSocialAccount records a verified provider account for the user. Returns
// created=true only on the first link (so the caller awards XP once). A
// provider account already linked to a different user yields ErrSocialTaken.
func (s *Store) LinkSocialAccount(ctx context.Context, userID, provider, providerID, handle string) (bool, error) {
	var owner string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM social_accounts WHERE provider = $1 AND provider_id = $2`,
		provider, providerID).Scan(&owner)
	switch {
	case err == nil:
		if owner != userID {
			return false, ErrSocialTaken
		}
		_, err = s.db.ExecContext(ctx,
			`UPDATE social_accounts SET handle = NULLIF($3,'') WHERE provider = $1 AND provider_id = $2`,
			provider, providerID, handle)
		return false, err
	case errors.Is(err, sql.ErrNoRows):
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO social_accounts (user_id, provider, provider_id, handle)
			 VALUES ($1, $2, $3, NULLIF($4,''))`,
			userID, provider, providerID, handle)
		if err != nil {
			if isUniqueViolation(err) { // lost a race for this provider_id
				return false, ErrSocialTaken
			}
			return false, err
		}
		return true, nil
	default:
		return false, err
	}
}

// ListSocialAccounts returns a user's linked social accounts.
func (s *Store) ListSocialAccounts(ctx context.Context, userID string) ([]SocialAccount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider, provider_id, COALESCE(handle,''), verified_at
		 FROM social_accounts WHERE user_id = $1 ORDER BY verified_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SocialAccount{}
	for rows.Next() {
		var a SocialAccount
		if err := rows.Scan(&a.Provider, &a.ProviderID, &a.Handle, &a.VerifiedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
