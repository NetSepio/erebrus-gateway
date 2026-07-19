package api

import (
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Email linking lets an authenticated wallet account attach a verified email for
// perks/ranking + recovery (PROD-STREAMLINE-PLAN §2). It is optional and never
// required to use the VPN. Both routes require a wallet PASETO bearer.
const (
	otpMaxAttempts    = 5
	otpResendCooldown = 60 * time.Second
)

type emailOTPReq struct {
	Email string `json:"email"`
	App   string `json:"app"` // optional product name for the OTP email (e.g. "Erebrus Drop")
}

// handleEmailOTPStart sends a 6-digit code to the requested email: POST /api/v2/auth/email.
func (s *Server) handleEmailOTPStart(c *gin.Context) {
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
	uid := userID(c)

	// One email → one account.
	if owner, err := s.store.EmailOwner(c, email); err == nil && owner != uid {
		fail(c, http.StatusConflict, "email already linked to another account")
		return
	}
	// Resend cooldown: reject if an unexpired code was just issued.
	if last, err := s.store.LatestEmailOTP(c, uid, email); err == nil && time.Since(last.CreatedAt) < otpResendCooldown {
		fail(c, http.StatusTooManyRequests, "please wait before requesting another code")
		return
	}

	code, err := generateOTP()
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to generate code")
		return
	}
	plat := s.platform.Snapshot()
	if err := s.store.CreateEmailOTP(c, uid, email, hashOTP(code), plat.MagicLinkExpiration); err != nil {
		fail(c, http.StatusInternalServerError, "failed to store code")
		return
	}
	if err := s.mailer.SendOTP(c, email, code, req.App); err != nil {
		fail(c, http.StatusBadGateway, "failed to send email")
		return
	}
	s.logActivity(c, uid, "auth.email.request", "")
	ok(c, http.StatusOK, gin.H{"status": "sent", "expires_in": int(plat.MagicLinkExpiration.Seconds())})
}

type emailVerifyReq struct {
	Email string `json:"email"`
	Code  string `json:"code"`
	Ref   string `json:"ref"` // optional referral code (login flow only; ignored on link)
}

// handleEmailOTPVerify checks a code and links the email: POST /api/v2/auth/email/verify.
func (s *Server) handleEmailOTPVerify(c *gin.Context) {
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
	uid := userID(c)
	if owner, err := s.store.EmailOwner(c, email); err == nil && owner != uid {
		fail(c, http.StatusConflict, "email already linked to another account")
		return
	}
	otp, err := s.store.LatestEmailOTP(c, uid, email)
	if err != nil {
		fail(c, http.StatusUnauthorized, "no pending code; request a new one")
		return
	}
	if otp.Attempts >= otpMaxAttempts {
		_ = s.store.DeleteEmailOTPs(c, uid, email)
		fail(c, http.StatusTooManyRequests, "too many attempts; request a new code")
		return
	}
	if subtle.ConstantTimeCompare([]byte(otp.CodeHash), []byte(hashOTP(strings.TrimSpace(req.Code)))) != 1 {
		n, _ := s.store.BumpEmailOTPAttempts(c, otp.ID)
		remaining := otpMaxAttempts - n
		if remaining < 0 {
			remaining = 0
		}
		fail(c, http.StatusUnauthorized, fmt.Sprintf("invalid code (%d attempts left)", remaining))
		return
	}
	if err := s.store.SetUserEmailVerified(c, uid, email); err != nil {
		fail(c, http.StatusInternalServerError, "failed to link email")
		return
	}
	// Org invites stay pending until the user accepts them in-app.
	_ = s.store.DeleteEmailOTPs(c, uid, email)
	plat := s.platform.Snapshot()
	// XP driver: verifying an email earns XP once (best-effort).
	_, _ = s.store.AwardXPOnce(c, uid, "email_verified", plat.XPEmailVerified,
		map[string]any{}, "email_verified:"+uid)
	// Record email as a linked social account too (no social_verified XP — email
	// has its own email_verified driver).
	_, _ = s.store.LinkSocialAccount(c, uid, "email", email, "")
	s.logActivity(c, uid, "auth.email.verify", "")

	u, _ := s.store.GetUser(c, uid)
	ok(c, http.StatusOK, u)
}

// normalizeEmail lowercases/trims and does a minimal structural check. Returns
// (normalized, true) when plausibly valid.
func normalizeEmail(s string) (string, bool) {
	e := strings.ToLower(strings.TrimSpace(s))
	if len(e) < 3 || len(e) > 254 || strings.ContainsAny(e, " \t\r\n") {
		return "", false
	}
	at := strings.IndexByte(e, '@')
	if at <= 0 || at == len(e)-1 {
		return "", false
	}
	if strings.IndexByte(e[at+1:], '.') < 0 { // domain needs a dot
		return "", false
	}
	return e, true
}

// generateOTP returns a uniformly random 6-digit code.
func generateOTP() (string, error) {
	n, err := crand.Int(crand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// hashOTP returns sha256(code) hex; codes are never stored in cleartext.
func hashOTP(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
