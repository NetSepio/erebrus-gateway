package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/NetSepio/gateway/internal/oauth"
	"github.com/gin-gonic/gin"
)

// handleAppleCallback receives Apple's form_post redirect for a specific app,
// validates the Apple payload, and redirects to the allowlisted Android package
// via an intent:// URL, or to the webapp for non-relay Apple IDs.
// The callback path maps to a fixed app; the client can never supply an
// arbitrary package name or redirect target.
func (s *Server) handleAppleCallback(c *gin.Context) {
	// Apple sends a form_post body first so we can read state if needed.
	if err := c.Request.ParseForm(); err != nil {
		s.appleRedirect(c, "auth", stateFromForm(c), "parse_error", "failed to parse callback form", nil)
		return
	}

	app := c.Param("app")
	if app == "" {
		// Bare /apple/callback is the legacy web auth service ID (com.erebrus.auth).
		app = "auth"
	}

	androidPkg := s.applePackages[app]
	if androidPkg == "" {
		fail(c, http.StatusServiceUnavailable, "apple callback not configured for "+app)
		return
	}

	state := c.PostForm("state")
	code := c.PostForm("code")
	idToken := c.PostForm("id_token")
	user := c.PostForm("user")
	errCode := c.PostForm("error")
	errDesc := c.PostForm("error_description")

	// Apple itself reported an error (e.g. user cancelled).
	if errCode != "" {
		s.appleRedirect(c, app, state, errCode, errDesc, nil)
		return
	}

	if state == "" {
		s.appleRedirect(c, app, state, "missing_state", "", nil)
		return
	}
	if !strings.HasPrefix(state, app+".") {
		s.appleRedirect(c, app, state, "invalid_state", "", nil)
		return
	}
	if idToken == "" {
		s.appleRedirect(c, app, state, "missing_id_token", "", nil)
		return
	}

	v, ok := s.appleVerifiers[app]
	if !ok {
		s.appleRedirect(c, app, state, "not_configured", "no verifier for app", nil)
		return
	}

	claims, err := v.Verify(c, idToken)
	if err != nil {
		s.appleRedirect(c, app, state, "invalid_token", err.Error(), nil)
		return
	}

	// Validate the authorization code against the c_hash in the ID token.
	if code == "" {
		s.appleRedirect(c, app, state, "missing_code", "", nil)
		return
	}
	if claims.CHash != "" && !oauth.AppleCHashOK(code, claims.CHash) {
		s.appleRedirect(c, app, state, "code_mismatch", "", nil)
		return
	}

	// The callback cannot validate the raw nonce (it is not present in the
	// form_post), but it verifies the ID token signature/issuer/audience and
	// that a nonce was requested. The raw nonce is validated by /auth/apple.
	if claims.NonceSupported && claims.Nonce == "" {
		s.appleRedirect(c, app, state, "missing_nonce", "", nil)
		return
	}

	s.appleRedirect(c, app, state, "", "", url.Values{
		"code":     {code},
		"id_token": {idToken},
		"user":     {user},
		"state":    {state},
	})
}

func stateFromForm(c *gin.Context) string {
	if err := c.Request.ParseForm(); err != nil {
		return ""
	}
	return c.Request.PostForm.Get("state")
}

// appleRedirect forwards the callback result to either the Android intent URL
// or the webapp callback URL, depending on APPLE_ANDROID_RELAY_IDS.
func (s *Server) appleRedirect(c *gin.Context, app, state, errCode, errDesc string, extra url.Values) {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	if errCode != "" {
		q.Set("error", errCode)
		if errDesc != "" {
			q.Set("error_description", errDesc)
		}
	} else if len(extra) > 0 {
		for k, vv := range extra {
			for _, v := range vv {
				if v != "" {
					q.Add(k, v)
				}
			}
		}
	}

	if s.appleAndroidRelayIDs[app] {
		androidPkg := s.applePackages[app]
		intent := fmt.Sprintf(
			"intent://callback?%s#Intent;package=%s;scheme=signinwithapple;end",
			q.Encode(),
			androidPkg,
		)
		c.Redirect(http.StatusFound, intent)
		return
	}

	origin := strings.TrimRight(s.cfg.ErebrusPublicBaseURL, "/")
	c.Redirect(http.StatusFound, origin+"/auth/apple/callback?"+q.Encode())
}
