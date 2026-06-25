package store

import (
	"context"
	"time"
)

// LeaderEntry is one row of a leaderboard (wallet truncated by the API layer).
type LeaderEntry struct {
	UserID string `json:"-"`
	Wallet string `json:"wallet"`
	Value  int64  `json:"value"`
}

// Leaderboard returns the top users by `metric` ("xp" | "referrals"). A non-nil
// `since` scopes the window (e.g. last 30 days); nil means all-time.
func (s *Store) Leaderboard(ctx context.Context, metric string, since *time.Time, limit, offset int) ([]LeaderEntry, error) {
	var (
		q    string
		args []any
	)
	switch {
	case metric == "referrals":
		if since != nil {
			q = `SELECT u.id, COALESCE(u.wallet_address,''), count(*) AS val
			     FROM xp_events e JOIN users u ON u.id = e.user_id
			     WHERE e.kind = 'referral_qualified' AND e.meta->>'role' = 'referrer' AND e.created_at >= $3
			     GROUP BY u.id ORDER BY val DESC, u.id LIMIT $1 OFFSET $2`
			args = []any{limit, offset, *since}
		} else {
			q = `SELECT u.id, COALESCE(u.wallet_address,''), count(*) AS val
			     FROM xp_events e JOIN users u ON u.id = e.user_id
			     WHERE e.kind = 'referral_qualified' AND e.meta->>'role' = 'referrer'
			     GROUP BY u.id ORDER BY val DESC, u.id LIMIT $1 OFFSET $2`
			args = []any{limit, offset}
		}
	case since != nil: // xp, windowed
		q = `SELECT u.id, COALESCE(u.wallet_address,''), COALESCE(SUM(e.points),0) AS val
		     FROM xp_events e JOIN users u ON u.id = e.user_id
		     WHERE e.created_at >= $3
		     GROUP BY u.id ORDER BY val DESC, u.id LIMIT $1 OFFSET $2`
		args = []any{limit, offset, *since}
	default: // xp, all-time (cached column)
		q = `SELECT id, COALESCE(wallet_address,''), xp_earned AS val
		     FROM users WHERE xp_earned > 0 ORDER BY xp_earned DESC, id LIMIT $1 OFFSET $2`
		args = []any{limit, offset}
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeaderEntry{}
	for rows.Next() {
		var e LeaderEntry
		if err := rows.Scan(&e.UserID, &e.Wallet, &e.Value); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MyRank returns the caller's value and 1-based rank for the same metric/window
// (rank = 1 + the number of users strictly ahead).
func (s *Store) MyRank(ctx context.Context, metric string, since *time.Time, userID string) (rank int, value int64, err error) {
	switch {
	case metric == "referrals":
		if since != nil {
			if err = s.db.QueryRowContext(ctx,
				`SELECT count(*) FROM xp_events
				 WHERE kind='referral_qualified' AND meta->>'role'='referrer' AND user_id=$1 AND created_at>=$2`,
				userID, *since).Scan(&value); err != nil {
				return
			}
			err = s.db.QueryRowContext(ctx,
				`SELECT count(*)+1 FROM (
				   SELECT user_id FROM xp_events
				   WHERE kind='referral_qualified' AND meta->>'role'='referrer' AND created_at>=$2
				   GROUP BY user_id HAVING count(*) > $1) t`, value, *since).Scan(&rank)
		} else {
			if err = s.db.QueryRowContext(ctx,
				`SELECT count(*) FROM xp_events
				 WHERE kind='referral_qualified' AND meta->>'role'='referrer' AND user_id=$1`,
				userID).Scan(&value); err != nil {
				return
			}
			err = s.db.QueryRowContext(ctx,
				`SELECT count(*)+1 FROM (
				   SELECT user_id FROM xp_events
				   WHERE kind='referral_qualified' AND meta->>'role'='referrer'
				   GROUP BY user_id HAVING count(*) > $1) t`, value).Scan(&rank)
		}
	case since != nil: // xp windowed
		err = s.db.QueryRowContext(ctx,
			`SELECT COALESCE(SUM(points),0) FROM xp_events WHERE user_id=$1 AND created_at>=$2`,
			userID, *since).Scan(&value)
		if err != nil {
			return
		}
		err = s.db.QueryRowContext(ctx,
			`SELECT count(*)+1 FROM (
			   SELECT user_id FROM xp_events WHERE created_at>=$2
			   GROUP BY user_id HAVING SUM(points) > $1) t`, value, *since).Scan(&rank)
	default: // xp all-time
		err = s.db.QueryRowContext(ctx, `SELECT xp_earned FROM users WHERE id=$1`, userID).Scan(&value)
		if err != nil {
			return
		}
		err = s.db.QueryRowContext(ctx, `SELECT count(*)+1 FROM users WHERE xp_earned > $1`, value).Scan(&rank)
	}
	return
}
