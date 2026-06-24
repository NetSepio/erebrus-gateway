package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// defaultTierThresholds are the §5b cutoffs: T0..T4.
var defaultTierThresholds = []int64{0, 100, 500, 2000, 10000}

// SetTierThresholds overrides the XP→tier cutoffs (ascending). Empty keeps the
// defaults. Wired from config at startup.
func (s *Store) SetTierThresholds(t []int64) {
	if len(t) > 0 {
		s.tierThresholds = t
	}
}

// TierThresholds returns the configured cutoffs (or the defaults).
func (s *Store) TierThresholds() []int64 {
	if len(s.tierThresholds) == 0 {
		return defaultTierThresholds
	}
	return s.tierThresholds
}

// tierForXP returns the highest tier index whose threshold xp meets.
func tierForXP(xp int64, thresholds []int64) int {
	tier := 0
	for i, t := range thresholds {
		if xp >= t {
			tier = i
		}
	}
	return tier
}

// AwardXP appends an XP event (no dedup) and bumps the user's cached lifetime
// total + tier, in one transaction. xp_events is append-only; xp_earned never
// decreases. Used for per-event awards like referrals.
func (s *Store) AwardXP(ctx context.Context, userID, kind string, points int64, meta map[string]any) error {
	_, err := s.awardXP(ctx, userID, kind, points, meta, "")
	return err
}

// AwardXPOnce is AwardXP guarded by a dedup key: the award happens at most once
// per key (e.g. once ever, per-day, per-month). Returns whether it awarded.
func (s *Store) AwardXPOnce(ctx context.Context, userID, kind string, points int64, meta map[string]any, dedupKey string) (bool, error) {
	return s.awardXP(ctx, userID, kind, points, meta, dedupKey)
}

func (s *Store) awardXP(ctx context.Context, userID, kind string, points int64, meta map[string]any, dedupKey string) (bool, error) {
	raw, err := json.Marshal(meta)
	if err != nil {
		return false, err
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck

	if dedupKey == "" {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO xp_events (user_id, kind, points, meta) VALUES ($1,$2,$3,$4::jsonb)`,
			userID, kind, points, string(raw)); err != nil {
			return false, err
		}
	} else {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO xp_events (user_id, kind, points, meta, dedup_key)
			 VALUES ($1,$2,$3,$4::jsonb,$5) ON CONFLICT (dedup_key) DO NOTHING`,
			userID, kind, points, string(raw), dedupKey)
		if err != nil {
			return false, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return false, tx.Commit() // already awarded for this key — no-op
		}
	}

	var earned int64
	if err := tx.QueryRowContext(ctx,
		`UPDATE users SET xp_earned = xp_earned + $2, updated_at = now()
		 WHERE id = $1 RETURNING xp_earned`, userID, points).Scan(&earned); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET tier = $2 WHERE id = $1`, userID, tierForXP(earned, s.TierThresholds())); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// UserXP returns a user's lifetime earned, claimed, and current tier.
func (s *Store) UserXP(ctx context.Context, userID string) (earned, claimed int64, tier int, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT xp_earned, xp_claimed, tier FROM users WHERE id = $1`, userID).
		Scan(&earned, &claimed, &tier)
	if err == sql.ErrNoRows {
		return 0, 0, 0, ErrNotFound
	}
	return
}

// XPBreakdown returns earned XP summed by event kind.
func (s *Store) XPBreakdown(ctx context.Context, userID string) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT kind, COALESCE(SUM(points),0) FROM xp_events WHERE user_id = $1 GROUP BY kind`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var p int64
		if err := rows.Scan(&k, &p); err != nil {
			return nil, err
		}
		out[k] = p
	}
	return out, rows.Err()
}

// AwardOperatorUptimeXP grants `points` once per UTC day to the owner of each
// healthy node (heartbeat within the last hour), capped to the operator's 5
// most-recently-active nodes. Idempotent via a per-node-per-day dedup key.
func (s *Store) AwardOperatorUptimeXP(ctx context.Context, points int64) error {
	day := time.Now().UTC().Format("20060102")
	rows, err := s.db.QueryContext(ctx,
		`SELECT node_id, owner_user_id FROM (
		   SELECT id AS node_id, owner_user_id,
		          row_number() OVER (PARTITION BY owner_user_id ORDER BY last_heartbeat DESC) AS rn
		   FROM nodes
		   WHERE owner_user_id IS NOT NULL
		     AND status = 'online'
		     AND last_heartbeat IS NOT NULL
		     AND last_heartbeat > now() - interval '1 hour'
		 ) ranked
		 WHERE rn <= 5`)
	if err != nil {
		return err
	}
	type pair struct{ node, owner string }
	var healthy []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.node, &p.owner); err != nil {
			rows.Close()
			return err
		}
		healthy = append(healthy, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range healthy {
		_, _ = s.AwardXPOnce(ctx, p.owner, "operator_uptime_day", points,
			map[string]any{"node_id": p.node, "day": day}, "uptime:"+p.node+":"+day)
	}
	return nil
}
