package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Payment is a USDC payment request.
type Payment struct {
	ID              string
	UserID          string
	PlanID          string
	Chain           string
	ExpectedAmount  string
	Token           string
	TreasuryAddress string
	Reference       string
	Status          string
	CreatedAt       time.Time
}

// CreatePayment inserts a pending crypto payment and returns its id.
func (s *Store) CreatePayment(ctx context.Context, p Payment) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO crypto_payments
		   (user_id, plan_id, chain, expected_amount, token, treasury_address, reference, status)
		 VALUES ($1, $2, $3, $4::numeric, $5, $6, $7, 'pending')
		 RETURNING id`,
		p.UserID, p.PlanID, p.Chain, p.ExpectedAmount, p.Token, p.TreasuryAddress, p.Reference).Scan(&id)
	return id, err
}

// GetPayment returns a payment by id.
func (s *Store) GetPayment(ctx context.Context, id string) (*Payment, error) {
	var p Payment
	err := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(user_id::text,''), plan_id, chain, expected_amount::text,
		        token, treasury_address, COALESCE(reference,''), status, created_at
		 FROM crypto_payments WHERE id = $1`, id).
		Scan(&p.ID, &p.UserID, &p.PlanID, &p.Chain, &p.ExpectedAmount,
			&p.Token, &p.TreasuryAddress, &p.Reference, &p.Status, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

// ConfirmPaymentAndActivate marks a payment confirmed and creates/extends the
// user's subscription in one transaction. tx_hash uniqueness guards replay.
func (s *Store) ConfirmPaymentAndActivate(ctx context.Context, paymentID, txHash, payer string, period time.Duration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var userID, planID, status string
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(user_id::text,''), plan_id, status FROM crypto_payments WHERE id=$1 FOR UPDATE`,
		paymentID).Scan(&userID, &planID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status == "confirmed" {
		return nil // idempotent
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE crypto_payments SET status='confirmed', tx_hash=$2, payer_address=NULLIF($3,''), confirmed_at=now()
		 WHERE id=$1`, paymentID, txHash, payer); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO subscriptions (user_id, plan_id, source, status, current_period_end, payment_id)
		 VALUES ($1, $2, 'crypto', 'active', now() + $3::interval, $4)`,
		userID, planID, period.String(), paymentID); err != nil {
		return err
	}
	return tx.Commit()
}
