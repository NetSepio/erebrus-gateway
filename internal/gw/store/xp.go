package store

import (
	"context"
	"encoding/json"
)

// AwardXP appends an XP event and bumps the user's cached lifetime total, in one
// transaction. The xp_events ledger is append-only; xp_earned never decreases.
// Tiers/claims/leaderboard (S6) read this foundation.
func (s *Store) AwardXP(ctx context.Context, userID, kind string, points int64, meta map[string]any) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO xp_events (user_id, kind, points, meta) VALUES ($1, $2, $3, $4::jsonb)`,
		userID, kind, points, string(raw)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET xp_earned = xp_earned + $2, updated_at = now() WHERE id = $1`,
		userID, points); err != nil {
		return err
	}
	return tx.Commit()
}
