package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/gw/wallet"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const flowIDTTL = 10 * time.Minute

// challengeMessage builds the deterministic message the wallet signs. It must be
// reproducible identically at GET /auth and POST /auth.
func (s *Server) challengeMessage(flowID string) string {
	return s.cfg.AuthEULA + " Challenge: " + flowID
}

// handleFlowID starts a wallet-signature login: GET /api/v2/auth.
func (s *Server) handleFlowID(c *gin.Context) {
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

type authenticateReq struct {
	FlowID    string `json:"flow_id"`
	Signature string `json:"signature"`
	PublicKey string `json:"public_key"`
	Ref       string `json:"ref"` // optional referral code (binds once, on first signup)
}

// handleAuthenticate completes login: POST /api/v2/auth.
func (s *Server) handleAuthenticate(c *gin.Context) {
	var req authenticateReq
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
	// Optional referral binding: sets referred_by once (immutable, self-blocked).
	// XP is awarded later, on the referee's first trial start (the qualifying action).
	if ref := strings.TrimSpace(req.Ref); ref != "" {
		if referrerID, err := s.store.UserIDByReferralCode(c, ref); err == nil {
			_, _ = s.store.BindReferrer(c, u.ID, referrerID)
		}
	}
	tok, err := s.tokens.IssueUser(u.ID, u.WalletAddress, u.Chain, u.Role)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to issue token")
		return
	}
	s.logActivity(c, u.ID, "auth.login", "") // audit (this route is public, no middleware)
	ok(c, http.StatusOK, gin.H{"token": tok, "user_id": u.ID, "role": u.Role})
}
