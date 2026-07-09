package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/NetSepio/gateway/internal/wallet"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const flowIDTTL = 10 * time.Minute

// challengeMessage builds the deterministic message the wallet signs. It must be
// reproducible identically at GET /auth and POST /auth.
func (s *Server) challengeMessage(flowID string) string {
	return s.platform.Snapshot().AuthEULA + " Challenge: " + flowID
}

// handleAuthChallenge starts a wallet-signature login: GET /api/v2/auth.
func (s *Server) handleAuthChallenge(c *gin.Context) {
	walletAddr := strings.TrimSpace(c.Query("wallet_address"))
	chain := strings.TrimSpace(c.Query("chain"))
	if walletAddr == "" || chain == "" {
		fail(c, http.StatusBadRequest, "wallet_address and chain are required")
		return
	}
	switch chain {
	case wallet.ChainEVM, wallet.ChainSOL:
	default:
		fail(c, http.StatusBadRequest, "unsupported chain (expected evm or sol)")
		return
	}
	flowID := uuid.NewString()
	if err := s.store.CreateFlowID(c, flowID, walletAddr, chain, flowIDTTL); err != nil {
		fail(c, http.StatusInternalServerError, "failed to create flow id")
		return
	}
	ok(c, http.StatusOK, gin.H{"flow_id": flowID, "message": s.challengeMessage(flowID)})
}

type authCompleteReq struct {
	FlowID    string `json:"flow_id"`
	Signature string `json:"signature"`
	PublicKey string `json:"public_key"`
	Ref       string `json:"ref"` // optional referral code (binds once, on first signup)
}

// handleAuthComplete completes login: POST /api/v2/auth.
func (s *Server) handleAuthComplete(c *gin.Context) {
	var req authCompleteReq
	if err := c.ShouldBindJSON(&req); err != nil || req.FlowID == "" || req.Signature == "" {
		fail(c, http.StatusBadRequest, "flow_id and signature are required")
		return
	}
	flow, err := s.store.GetFlowID(c, req.FlowID)
	if err != nil {
		fail(c, http.StatusUnauthorized, "flow id not found or expired")
		return
	}
	msg := s.challengeMessage(flow.FlowID)
	recovered, err := wallet.Verify(flow.Chain, msg, req.Signature, req.PublicKey)
	if err != nil {
		fail(c, http.StatusUnauthorized, "signature verification failed")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(recovered), strings.TrimSpace(flow.WalletAddress)) {
		fail(c, http.StatusUnauthorized, "signature does not match wallet")
		return
	}
	_ = s.store.DeleteFlowID(c, req.FlowID) // single-use

	u, err := s.store.UpsertUserByWallet(c, flow.WalletAddress, flow.Chain, s.cfg.AdminWalletAddress)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load user")
		return
	}
	// Org invites stay pending until the user accepts them in-app.
	// Optional referral binding: sets referred_by once (immutable, self-blocked).
	// Must precede the trial below so the qualifying trial awards referral XP.
	s.bindReferralCode(c, u.ID, req.Ref)
	s.bootstrapUser(c, u)
	tok, err := s.tokens.IssueUser(u.ID, u.WalletAddress, u.Chain, u.Role)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to issue token")
		return
	}
	s.logActivity(c, u.ID, "auth.login", "") // audit (this route is public, no middleware)
	ok(c, http.StatusOK, gin.H{"token": tok, "user_id": u.ID, "role": u.Role})
}

// bootstrapUser runs the shared first-login bootstrap for any identity (wallet,
// email, Google, Apple): ensure the user owns a personal basic-plan org and, when
// it was just created, grant the automatic 7-day VPN trial. Best-effort — login
// proceeds even if this fails, and the one-trial-per-user index makes a re-grant
// a no-op.
func (s *Server) bootstrapUser(c *gin.Context, u *store.User) {
	if _, created, _ := s.store.EnsurePersonalOrg(c, u.ID, u.WalletAddress); created {
		s.logActivity(c, u.ID, "org.create", "")
		if _, err := s.store.StartTrial(c, u.ID, "pro", s.platform.Snapshot().TrialPeriod); err == nil {
			s.awardReferralXP(c, u.ID)
			s.logActivity(c, u.ID, "subscription.trial", "")
		}
	}
}

// issueAndRespond writes the standard login response for a resolved user.
func (s *Server) issueAndRespond(c *gin.Context, u *store.User, method string) {
	tok, err := s.tokens.IssueUser(u.ID, u.WalletAddress, u.Chain, u.Role)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to issue token")
		return
	}
	s.logActivity(c, u.ID, "auth.login."+method, "")
	ok(c, http.StatusOK, gin.H{"token": tok, "user_id": u.ID, "role": u.Role})
}
