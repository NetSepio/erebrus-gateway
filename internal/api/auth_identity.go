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
		"apple":  s.appleEnabled(),
	})
}

func (s *Server) appleEnabled() bool {
	if s.apple.Enabled() {
		return true
	}
	return len(s.appleVerifiers) > 0
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
	if err := s.mailer.SendOTP(c, email, code, req.App); err != nil {
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
	// Bind before bootstrap so first-organization qualification awards referral XP.
	s.bindReferralCode(c, u.ID, req.Ref)
	s.finishIdentityLogin(c, u, created, email, "email")
}

// ── Google / Apple login (OIDC id_token) ─────────────────────────────────────

type oidcReq struct {
	IDToken string `json:"id_token"`
	Ref     string `json:"ref"` // optional referral code (binds once, on first signup)
}

type appleAuthReq struct {
	IDToken           string `json:"id_token"`
	AuthorizationCode string `json:"authorization_code"`
	Nonce             string `json:"nonce"`
	State             string `json:"state"`
	Ref               string `json:"ref"` // optional referral code
}

func (s *Server) handleGoogleAuth(c *gin.Context) { s.oidcLogin(c, "google", s.google) }

func (s *Server) handleAppleAuth(c *gin.Context) {
	if !s.appleEnabled() {
		fail(c, http.StatusServiceUnavailable, "apple login is not configured")
		return
	}
	var req appleAuthReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.IDToken) == "" {
		fail(c, http.StatusBadRequest, "id_token is required")
		return
	}

	v, app, ok := s.appleVerifierForState(req.State)
	if !ok {
		fail(c, http.StatusServiceUnavailable, "apple login is not configured for this app")
		return
	}

	claims, err := v.Verify(c, req.IDToken)
	if err != nil {
		// Per-app Service IDs validate Android/web tokens. iOS native tokens use
		// the iOS bundle ID as aud, configured in APPLE_CLIENT_IDS, so fall back
		// to the legacy verifier when the per-app verifier rejects the audience.
		if err.Error() == "audience mismatch" && s.apple.Enabled() {
			claims, err = s.apple.Verify(c, req.IDToken)
		}
		if err != nil {
			fail(c, http.StatusUnauthorized, "invalid apple token")
			return
		}
	}

	if req.State != "" && !strings.HasPrefix(req.State, app+".") {
		fail(c, http.StatusUnauthorized, "invalid apple state")
		return
	}
	if err := s.validateAppleClaims(claims, req.Nonce, req.AuthorizationCode); err != nil {
		fail(c, http.StatusUnauthorized, err.Error())
		return
	}

	// Only a provider-verified email is trusted for account resolution/merge.
	email := claims.Email
	if email != "" && !claims.EmailVerified {
		email = ""
	}
	u, created, err := s.store.ResolveOrCreateUserBySocial(c, "apple", claims.Subject, email, email)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve account")
		return
	}
	// Bind before bootstrap so first-organization qualification awards referral XP.
	s.bindReferralCode(c, u.ID, req.Ref)
	s.finishIdentityLogin(c, u, created, email, "apple")
}

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
	// Bind before bootstrap so first-organization qualification awards referral XP.
	s.bindReferralCode(c, u.ID, req.Ref)
	s.finishIdentityLogin(c, u, created, email, provider)
}

// appleVerifierForState returns the Apple verifier and app key for a state
// value. State is expected to be "app.<random>" (e.g. "drop.abc123").
func (s *Server) appleVerifierForState(state string) (*oauth.Verifier, string, bool) {
	if state != "" {
		if i := strings.IndexByte(state, '.'); i > 0 {
			app := state[:i]
			if v, ok := s.appleVerifiers[app]; ok {
				return v, app, true
			}
		}
	}
	return s.apple, "legacy", s.apple.Enabled()
}

func (s *Server) validateAppleClaims(claims *oauth.Claims, rawNonce, code string) error {
	if claims.NonceSupported && claims.Nonce == "" {
		return errors.New("apple token missing nonce")
	}
	if claims.Nonce != "" {
		if !oauth.AppleNonceOK(rawNonce, claims.Nonce) {
			return errors.New("apple nonce mismatch")
		}
	} else if rawNonce != "" {
		return errors.New("apple nonce mismatch")
	}
	if claims.CHash != "" {
		if code == "" {
			return errors.New("apple authorization code required")
		}
		if !oauth.AppleCHashOK(code, claims.CHash) {
			return errors.New("apple authorization code mismatch")
		}
	}
	return nil
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
