package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/NetSepio/gateway/internal/gw/store"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// canonical USDC tokens per chain.
const (
	usdcSolanaMint   = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	usdcBaseContract = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
)

// handlePlans lists subscription plans (public).
func (s *Server) handlePlans(c *gin.Context) {
	plans, err := s.store.ListPlans(c)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list plans")
		return
	}
	ok(c, http.StatusOK, plans)
}

// handleMySubscription returns the caller's current entitlement.
func (s *Server) handleMySubscription(c *gin.Context) {
	sub, err := s.store.ActiveSubscription(c, userID(c))
	if errors.Is(err, store.ErrNotFound) {
		ok(c, http.StatusOK, gin.H{"status": "none", "entitled": false})
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load subscription")
		return
	}
	ok(c, http.StatusOK, gin.H{
		"status": sub.Status, "entitled": true, "plan_id": sub.PlanID,
		"source": sub.Source, "current_period_end": sub.CurrentPeriodEnd,
	})
}

// handleStartTrial grants the one-time free trial (on the 'pro' plan).
func (s *Server) handleStartTrial(c *gin.Context) {
	sub, err := s.store.StartTrial(c, userID(c), "pro", trialPeriod)
	if errors.Is(err, store.ErrTrialUsed) {
		fail(c, http.StatusConflict, "trial already used")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to start trial")
		return
	}
	ok(c, http.StatusCreated, sub)
}

type createPaymentReq struct {
	PlanID string `json:"plan_id"`
	Chain  string `json:"chain"` // solana | base
}

// handleCreatePayment creates a USDC payment request (Reown AppKit pays it).
func (s *Server) handleCreatePayment(c *gin.Context) {
	var req createPaymentReq
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanID == "" {
		fail(c, http.StatusBadRequest, "plan_id and chain required")
		return
	}
	plan, err := s.store.GetPlan(c, req.PlanID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "unknown plan")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load plan")
		return
	}

	var tokenAddr, treasury string
	switch req.Chain {
	case "solana":
		tokenAddr, treasury = usdcSolanaMint, s.cfg.SolanaTreasury
	case "base":
		tokenAddr, treasury = usdcBaseContract, s.cfg.BaseTreasury
	default:
		fail(c, http.StatusBadRequest, "chain must be solana or base")
		return
	}
	if treasury == "" {
		fail(c, http.StatusServiceUnavailable, "payments not configured for this chain")
		return
	}

	reference := uuid.NewString()
	id, err := s.store.CreatePayment(c, store.Payment{
		UserID: userID(c), PlanID: plan.ID, Chain: req.Chain,
		ExpectedAmount: plan.PriceUSDC, Token: tokenAddr, TreasuryAddress: treasury, Reference: reference,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to create payment")
		return
	}
	expiry := time.Now().Add(time.Duration(s.cfg.PaymentExpiryMinutes) * time.Minute).Unix()
	ok(c, http.StatusCreated, gin.H{
		"payment_id": id, "amount_usdc": plan.PriceUSDC, "chain": req.Chain,
		"treasury_address": treasury, "token": tokenAddr, "reference": reference,
		"expires_at": expiry,
	})
}

type confirmPaymentReq struct {
	TxHash string `json:"tx_hash"`
}

// handleConfirmPayment accepts the on-chain tx hash. On-chain verification
// (Solana/Base RPC per docs/v2/payments.md) is not yet wired, so this returns
// 501 until the verifier lands; the payment row is retained for later settling.
func (s *Server) handleConfirmPayment(c *gin.Context) {
	var req confirmPaymentReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TxHash == "" {
		fail(c, http.StatusBadRequest, "tx_hash required")
		return
	}
	if _, err := s.store.GetPayment(c, c.Param("id")); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "payment not found")
		return
	}
	fail(c, http.StatusNotImplemented, "on-chain verification not yet enabled")
}
