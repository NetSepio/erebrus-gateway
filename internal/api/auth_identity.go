package api

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/oauth"
	"github.com/NetSepio/gateway/internal/store"
	"github.com/NetSepio/gateway/internal/wallet"
	"github.com/gin-gonic/gin"
)

// handleAuthMethods reports which login methods the gateway has configured, so
// clients can show only the ones that will work (no errors for absent secrets).
func (s *Server) handleAuthMethods(c *gin.Context) {
	ok(c, http.StatusOK, gin.H{
		"wallet": true,
		"email":  s.mailer.Enabled(),
		"google": s.google.Enabled(),
		"apple":  s.apple.Enabled(),
	})
}

// ── Email login (passwordless, identity-resolving) ───────────────────────────
// Distinct from /auth/email (which links an email to an already-authed account):
// these are public and resolve-or-create the account by verified email.

func (s *Server) handleEmailLoginStart(c *gin.Context) {
	if !s.mailer.Enabled() {
		fail(c, http.StatusServiceUnavailable, "email is not configured")
		return
	}
	var req emailOTPReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "email is required")
		return
	}
	email, valid := normalizeEmail(req.Email)
	if !valid {
		fail(c, http.StatusBadRequest, "invalid email address")
		return
	}
	if otp, err := s.store.GetLoginOTP(c, email); err == nil && time.Since(otp.CreatedAt) < otpResendCooldown {
		fail(c, http.StatusTooManyRequests, "please wait before requesting another code")
		return
	}
	code, err := generateOTP()
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to generate code")
		return
	}
	plat := s.platform.Snapshot()
	if err := s.store.UpsertLoginOTP(c, email, hashOTP(code), plat.MagicLinkExpiration); err != nil {
		fail(c, http.StatusInternalServerError, "failed to store code")
		return
	}
	if err := s.mailer.SendOTP(c, email, code); err != nil {
		fail(c, http.StatusBadGateway, "failed to send email")
		return
	}
	ok(c, http.StatusOK, gin.H{"status": "sent", "expires_in": int(plat.MagicLinkExpiration.Seconds())})
}

func (s *Server) handleEmailLoginVerify(c *gin.Context) {
	var req emailVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		fail(c, http.StatusBadRequest, "email and code are required")
		return
	}
	email, valid := normalizeEmail(req.Email)
	if !valid {
		fail(c, http.StatusBadRequest, "invalid email address")
		return
	}
	otp, err := s.store.GetLoginOTP(c, email)
	if err != nil {
		fail(c, http.StatusUnauthorized, "no pending code; request a new one")
		return
	}
	if otp.Attempts >= otpMaxAttempts {
		_ = s.store.DeleteLoginOTP(c, email)
		fail(c, http.StatusTooManyRequests, "too many attempts; request a new code")
		return
	}
	if subtle.ConstantTimeCompare([]byte(otp.CodeHash), []byte(hashOTP(strings.TrimSpace(req.Code)))) != 1 {
		n, _ := s.store.BumpLoginOTPAttempts(c, email)
		remaining := otpMaxAttempts - n
		if remaining < 0 {
			remaining = 0
		}
		fail(c, http.StatusUnauthorized, fmt.Sprintf("invalid code (%d attempts left)", remaining))
		return
	}
	_ = s.store.DeleteLoginOTP(c, email)

	u, created, err := s.store.ResolveOrCreateUserByEmail(c, email)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve account")
		return
	}
	// Bind before the bootstrap below so the auto-trial awards referral XP.
	s.bindReferralCode(c, u.ID, req.Ref)
	s.finishIdentityLogin(c, u, created, email, "email")
}

// ── Google / Apple login (OIDC id_token) ─────────────────────────────────────

type oidcReq struct {
	IDToken string `json:"id_token"`
	Ref     string `json:"ref"` // optional referral code (binds once, on first signup)
}

func (s *Server) handleGoogleAuth(c *gin.Context) { s.oidcLogin(c, "google", s.google) }
func (s *Server) handleAppleAuth(c *gin.Context)  { s.oidcLogin(c, "apple", s.apple) }

func (s *Server) oidcLogin(c *gin.Context, provider string, v *oauth.Verifier) {
	if !v.Enabled() {
		fail(c, http.StatusServiceUnavailable, provider+" login is not configured")
		return
	}
	var req oidcReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.IDToken) == "" {
		fail(c, http.StatusBadRequest, "id_token is required")
		return
	}
	claims, err := v.Verify(c, req.IDToken)
	if err != nil {
		fail(c, http.StatusUnauthorized, "invalid "+provider+" token")
		return
	}
	// Only a provider-verified email is trusted for account resolution/merge.
	email := claims.Email
	if email != "" && !claims.EmailVerified {
		email = ""
	}
	u, created, err := s.store.ResolveOrCreateUserBySocial(c, provider, claims.Subject, email, email)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve account")
		return
	}
	// Bind before the bootstrap below so the auto-trial awards referral XP.
	s.bindReferralCode(c, u.ID, req.Ref)
	s.finishIdentityLogin(c, u, created, email, provider)
}

// finishIdentityLogin accepts pending org invites for the email, runs the
// first-login bootstrap, and issues the session token.
func (s *Server) finishIdentityLogin(c *gin.Context, u *store.User, created bool, email, method string) {
	// Org invites stay pending until the user accepts them in-app.
	s.bootstrapUser(c, u)
	if created {
		s.logActivity(c, u.ID, "auth.signup."+method, "")
	}
	s.issueAndRespond(c, u, method)
}

// ── Connect (authed linking) ─────────────────────────────────────────────────

func (s *Server) handleLinkGoogle(c *gin.Context) { s.oidcLink(c, "google", s.google) }
func (s *Server) handleLinkApple(c *gin.Context)  { s.oidcLink(c, "apple", s.apple) }

func (s *Server) oidcLink(c *gin.Context, provider string, v *oauth.Verifier) {
	if !v.Enabled() {
		fail(c, http.StatusServiceUnavailable, provider+" linking is not configured")
		return
	}
	var req oidcReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.IDToken) == "" {
		fail(c, http.StatusBadRequest, "id_token is required")
		return
	}
	claims, err := v.Verify(c, req.IDToken)
	if err != nil {
		fail(c, http.StatusUnauthorized, "invalid "+provider+" token")
		return
	}
	s.linkSocial(c, provider, claims.Subject, claims.Email)
}

// handleLinkWallet attaches a wallet to the authed account. The client first
// gets a challenge from GET /auth (wallet_address+chain), signs it, then posts
// the flow_id + signature here.
func (s *Server) handleLinkWallet(c *gin.Context) {
	var req struct {
		FlowID    string `json:"flow_id"`
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.FlowID == "" || req.Signature == "" {
		fail(c, http.StatusBadRequest, "flow_id and signature are required")
		return
	}
	flow, err := s.store.GetFlowID(c, req.FlowID)
	if err != nil {
		fail(c, http.StatusUnauthorized, "flow id not found or expired")
		return
	}
	recovered, err := wallet.Verify(flow.Chain, s.challengeMessage(flow.FlowID), req.Signature, req.PublicKey)
	if err != nil || !strings.EqualFold(strings.TrimSpace(recovered), strings.TrimSpace(flow.WalletAddress)) {
		fail(c, http.StatusUnauthorized, "signature does not match wallet")
		return
	}
	_ = s.store.DeleteFlowID(c, req.FlowID)

	if err := s.store.AttachWalletToUser(c, userID(c), flow.WalletAddress, flow.Chain); errors.Is(err, store.ErrWalletTaken) {
		fail(c, http.StatusConflict, "wallet already linked to another account")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to link wallet")
		return
	}
	s.logActivity(c, userID(c), "account.wallet.link", "")
	ok(c, http.StatusOK, gin.H{"wallet_address": flow.WalletAddress, "chain": flow.Chain})
}
