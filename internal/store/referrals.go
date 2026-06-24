package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"time"
)

// Crockford-ish base32, no ambiguous chars (I/L/O/U), for short shareable codes.
const referralAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ReferralRecent is one recent referee in a referral summary.
type ReferralRecent struct {
	Wallet    string    `json:"wallet"`    // truncated by the API layer
	Qualified bool      `json:"qualified"` // has started their trial (the qualifying action)
	JoinedAt  time.Time `json:"joined_at"`
}

// ReferralSummary backs GET /referrals/me.
type ReferralSummary struct {
	Code          string           `json:"code"`
	ReferredCount int              `json:"referred_count"`
	ReferredBy    string           `json:"referred_by,omitempty"` // referrer wallet (truncated by API)
	Recent        []ReferralRecent `json:"recent"`
}

// EnsureReferralCode returns the user's stable referral code, generating one on
// first use (retrying on the rare unique collision).
func (s *Store) EnsureReferralCode(ctx context.Context, userID string) (string, error) {
	var code sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT referral_code FROM users WHERE id = $1`, userID).Scan(&code); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if code.Valid && code.String != "" {
		return code.String, nil
	}
	for attempt := 0; attempt < 5; attempt++ {
		candidate, err := genReferralCode(8)
		if err != nil {
			return "", err
		}
		var out string
		err = s.db.QueryRowContext(ctx,
			`UPDATE users SET referral_code = $2, updated_at = now()
			 WHERE id = $1 AND referral_code IS NULL
			 RETURNING referral_code`, userID, candidate).Scan(&out)
		if err == nil {
			return out, nil
		}
		if isUniqueViolation(err) {
			continue // code already taken — try another
		}
		if errors.Is(err, sql.ErrNoRows) {
			// Someone set it concurrently; read it back.
			return s.referralCodeOf(ctx, userID)
		}
		return "", err
	}
	return "", errors.New("could not allocate referral code")
}

func (s *Store) referralCodeOf(ctx context.Context, userID string) (string, error) {
	var code sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT referral_code FROM users WHERE id = $1`, userID).Scan(&code)
	return code.String, err
}

// UserIDByReferralCode resolves a referral code to its owner's user id.
func (s *Store) UserIDByReferralCode(ctx context.Context, code string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE referral_code = $1`, code).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// BindReferrer sets referred_by_user_id once: only when currently NULL and the
// referrer is not the user themselves (immutable, one referrer per user, no
// self-referral). Returns whether it bound.
func (s *Store) BindReferrer(ctx context.Context, userID, referrerUserID string) (bool, error) {
	if userID == referrerUserID {
		return false, nil
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET referred_by_user_id = $2, updated_at = now()
		 WHERE id = $1 AND referred_by_user_id IS NULL`, userID, referrerUserID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ReferrerOf returns the user's referrer id, or "" when none.
func (s *Store) ReferrerOf(ctx context.Context, userID string) (string, error) {
	var ref sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT referred_by_user_id::text FROM users WHERE id = $1`, userID).Scan(&ref)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return ref.String, nil
}

// ReferralSummary returns the user's code (allocating it if needed), referrer,
// referred count, and recent referees.
func (s *Store) ReferralSummary(ctx context.Context, userID string) (*ReferralSummary, error) {
	code, err := s.EnsureReferralCode(ctx, userID)
	if err != nil {
		return nil, err
	}
	sum := &ReferralSummary{Code: code, Recent: []ReferralRecent{}}

	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(u.wallet_address,'')
		 FROM users me JOIN users u ON u.id = me.referred_by_user_id
		 WHERE me.id = $1`, userID).Scan(&sum.ReferredBy)

	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM users WHERE referred_by_user_id = $1`, userID).Scan(&sum.ReferredCount); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(u.wallet_address,''), u.created_at,
		        EXISTS(SELECT 1 FROM subscriptions s WHERE s.user_id = u.id AND s.source = 'trial') AS qualified
		 FROM users u
		 WHERE u.referred_by_user_id = $1
		 ORDER BY u.created_at DESC
		 LIMIT 20`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r ReferralRecent
		if err := rows.Scan(&r.Wallet, &r.JoinedAt, &r.Qualified); err != nil {
			return nil, err
		}
		sum.Recent = append(sum.Recent, r)
	}
	return sum, rows.Err()
}

// genReferralCode returns n random characters from referralAlphabet.
func genReferralCode(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = referralAlphabet[int(b[i])%len(referralAlphabet)]
	}
	return string(b), nil
}
