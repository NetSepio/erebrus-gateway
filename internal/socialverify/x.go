package socialverify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrX is returned when an X (Twitter) token cannot be verified.
var ErrX = errors.New("x verification failed")

// XVerifier resolves an X user from their OAuth2 access token (obtained by the
// frontend via PKCE). It needs no app secret — it calls the API as the user.
type XVerifier struct {
	baseURL string
	http    *http.Client
}

// NewXVerifier builds an X verifier. baseURL defaults to the public API (override
// for tests/self-host).
func NewXVerifier(baseURL string) *XVerifier {
	if baseURL == "" {
		baseURL = "https://api.twitter.com"
	}
	return &XVerifier{baseURL: baseURL, http: &http.Client{Timeout: 8 * time.Second}}
}

// Verify calls GET /2/users/me with the access token and returns (providerID,
// handle).
func (x *XVerifier) Verify(ctx context.Context, accessToken string) (string, string, error) {
	if accessToken == "" {
		return "", "", ErrX
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, x.baseURL+"/2/users/me", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := x.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("x request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", "", ErrX
	}
	var out struct {
		Data struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	if out.Data.ID == "" {
		return "", "", ErrX
	}
	return out.Data.ID, out.Data.Username, nil
}
