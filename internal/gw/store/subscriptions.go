package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ListPlans returns the active plans, cheapest first.
func (s *Store) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, price_usdc::text, period_days, max_clients
		 FROM plans WHERE is_active ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.PriceUSDC, &p.PeriodDays, &p.MaxClients); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPlan returns a plan by id.
func (s *Store) GetPlan(ctx context.Context, id string) (*Plan, error) {
	var p Plan
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, price_usdc::text, period_days, max_clients FROM plans WHERE id=$1`, id).
		Scan(&p.ID, &p.Name, &p.PriceUSDC, &p.PeriodDays, &p.MaxClients)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

// HasConsumedTrial reports whether the user has ever started a v2 trial.
func (s *Store) HasConsumedTrial(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM subscriptions WHERE user_id = $1 AND source = 'trial')`,
		userID).Scan(&exists)
	return exists, err
}

// LastSubscription returns the user's most recent subscription row, if any.
func (s *Store) LastSubscription(ctx context.Context, userID string) (*Subscription, error) {
	var sub Subscription
	err := s.db.QueryRowContext(ctx,
		`SELECT id, plan_id, source, status, current_period_end, created_at
		 FROM subscriptions
		 WHERE user_id = $1
		 ORDER BY current_period_end DESC NULLS LAST, created_at DESC
		 LIMIT 1`, userID).
		Scan(&sub.ID, &sub.PlanID, &sub.Source, &sub.Status, &sub.CurrentPeriodEnd, &sub.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &sub, err
}

// ActiveSubscription returns the user's current active, non-expired subscription
// (the entitlement check). Returns ErrNotFound when there is none.
func (s *Store) ActiveSubscription(ctx context.Context, userID string) (*Subscription, error) {
	var sub Subscription
	err := s.db.QueryRowContext(ctx,
		`SELECT id, plan_id, source, status, current_period_end, created_at
		 FROM subscriptions
		 WHERE user_id = $1 AND status = 'active'
		   AND (current_period_end IS NULL OR current_period_end > now())
		 ORDER BY current_period_end DESC NULLS LAST
		 LIMIT 1`, userID).
		Scan(&sub.ID, &sub.PlanID, &sub.Source, &sub.Status, &sub.CurrentPeriodEnd, &sub.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &sub, err
}

// StartTrial creates a one-time trial subscription on the given plan. The
// partial unique index idx_subs_one_trial enforces one trial per user; a
// conflict surfaces as ErrTrialUsed.
var ErrTrialUsed = errors.New("trial already used")

func (s *Store) StartTrial(ctx context.Context, userID, planID string, period time.Duration) (*Subscription, error) {
	end := time.Now().Add(period)
	var sub Subscription
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO subscriptions (user_id, plan_id, source, status, current_period_end)
		 VALUES ($1, $2, 'trial', 'active', $3)
		 RETURNING id, plan_id, source, status, current_period_end, created_at`,
		userID, planID, end).
		Scan(&sub.ID, &sub.PlanID, &sub.Source, &sub.Status, &sub.CurrentPeriodEnd, &sub.CreatedAt)
	if err != nil {
		// 23505 = unique_violation (the one-trial-per-user index)
		if isUniqueViolation(err) {
			return nil, ErrTrialUsed
		}
		return nil, err
	}
	return &sub, nil
}

// GrantNFTSubscription creates or refreshes a user's NFT-gated subscription,
// extending the period on each successful ownership re-check. One per user
// (idx_subs_one_nft).
func (s *Store) GrantNFTSubscription(ctx context.Context, userID, planID string, period time.Duration) (*Subscription, error) {
	end := time.Now().Add(period)
	var sub Subscription
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO subscriptions (user_id, plan_id, source, status, current_period_end)
		 VALUES ($1, $2, 'nft', 'active', $3)
		 ON CONFLICT (user_id) WHERE source = 'nft'
		 DO UPDATE SET status='active', plan_id=EXCLUDED.plan_id, current_period_end=EXCLUDED.current_period_end
		 RETURNING id, plan_id, source, status, current_period_end, created_at`,
		userID, planID, end).
		Scan(&sub.ID, &sub.PlanID, &sub.Source, &sub.Status, &sub.CurrentPeriodEnd, &sub.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// CountActiveSubscriptionsByPlan returns active subscription counts per plan (admin).
func (s *Store) CountActiveSubscriptionsByPlan(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT plan_id, count(*) FROM subscriptions
		 WHERE status='active' AND (current_period_end IS NULL OR current_period_end > now())
		 GROUP BY plan_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var plan string
		var n int
		if err := rows.Scan(&plan, &n); err != nil {
			return nil, err
		}
		out[plan] = n
	}
	return out, rows.Err()
}

func isUniqueViolation(err error) bool {
	// lib/pq surfaces pq.Error with Code "23505"; avoid importing pq here by
	// matching the SQLSTATE in the error text as a fallback.
	type sqlState interface{ SQLState() string }
	var st sqlState
	if errors.As(err, &st) {
		return st.SQLState() == "23505"
	}
	return false
}
