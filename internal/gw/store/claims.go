package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

// ErrInsufficientXP is returned when a claim exceeds the claimable balance.
var ErrInsufficientXP = errors.New("insufficient claimable XP")

// ClaimFreeDays spends `cost` claimable XP to grant `days` of entitlement
// (source='rank'), stacked on top of the user's current active period so the
// days are never wasted. Records the claim with IP + device. All in one tx.
func (s *Store) ClaimFreeDays(ctx context.Context, userID string, cost int64, days int, planID, ip, device string) (*Subscription, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// Spend only if the claimable balance (earned - claimed) covers the cost.
	var claimed int64
	err = tx.QueryRowContext(ctx,
		`UPDATE users SET xp_claimed = xp_claimed + $2, updated_at = now()
		 WHERE id = $1 AND xp_earned - xp_claimed >= $2
		 RETURNING xp_claimed`, userID, cost).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInsufficientXP
	}
	if err != nil {
		return nil, err
	}

	reward := strconv.Itoa(days) + "d free entitlement"
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO xp_claims (user_id, kind, xp_spent, reward, ip, device)
		 VALUES ($1, 'free_days', $2, $3, NULLIF($4,''), NULLIF($5,''))`,
		userID, cost, reward, ip, device); err != nil {
		return nil, err
	}

	// Grant/extend a rank entitlement: stack the days on top of the latest active
	// period end (or now), so existing trial/NFT days are not wasted.
	var sub Subscription
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO subscriptions (user_id, plan_id, source, status, current_period_end)
		 VALUES ($1, $2, 'rank', 'active',
		   COALESCE((SELECT max(current_period_end) FROM subscriptions
		             WHERE user_id = $1 AND status = 'active'
		               AND (current_period_end IS NULL OR current_period_end > now())), now())
		   + make_interval(days => $3))
		 RETURNING id, plan_id, source, status, current_period_end, created_at`,
		userID, planID, days).
		Scan(&sub.ID, &sub.PlanID, &sub.Source, &sub.Status, &sub.CurrentPeriodEnd, &sub.CreatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &sub, nil
}
