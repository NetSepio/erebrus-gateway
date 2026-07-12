package store

import (
	"context"
	"database/sql"
	"errors"
)

// ListPlans returns the active plans, cheapest first.
func (s *Store) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, period_days, max_clients
		 FROM plans WHERE is_active ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.PeriodDays, &p.MaxClients); err != nil {
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
		`SELECT id, name, period_days, max_clients FROM plans WHERE id=$1`, id).
		Scan(&p.ID, &p.Name, &p.PeriodDays, &p.MaxClients)
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

// CountLegacyActiveSubscriptionsByPlan returns retained personal subscription
// row counts for migration visibility. These rows never authorize product use.
func (s *Store) CountLegacyActiveSubscriptionsByPlan(ctx context.Context) (map[string]int, error) {
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
	type sqlState interface{ SQLState() string }
	var st sqlState
	return errors.As(err, &st) && st.SQLState() == "23505"
}
