// Package oauth verifies OpenID Connect ID tokens (Google, Apple) against the
// provider's published JWKS. Dependency-free: standard-library crypto only.
package oauth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Claims is the verified subset of an ID token we use for identity resolution.
type Claims struct {
	Subject        string // stable provider account id ("sub")
	Email          string // lower-cased; may be empty (e.g. Apple "hide my email" omitted)
	EmailVerified  bool
	Issuer         string
	Aud            string // first audience value from the token
	Nonce          string // Apple ID token nonce claim (hashed value)
	NonceSupported bool   // Apple nonce_supported claim
	CHash          string // Apple authorization code hash
}

// Verifier validates RS256 ID tokens for one provider.
type Verifier struct {
	issuers   map[string]bool
	audiences map[string]bool
	jwksURL   string
	hc        *http.Client

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	ttl       time.Duration
}

// New builds a verifier. audiences is the set of accepted `aud` values (the
// OAuth client IDs registered for this app across web/iOS/Android).
func New(jwksURL string, issuers, audiences []string) *Verifier {
	return &Verifier{
		issuers:   toSet(issuers),
		audiences: toSet(audiences),
		jwksURL:   jwksURL,
		hc:        &http.Client{Timeout: 10 * time.Second},
		keys:      map[string]*rsa.PublicKey{},
		ttl:       time.Hour,
	}
}

// NewGoogle / NewApple are the provider-specific constructors.
func NewGoogle(audiences []string) *Verifier {
	return New("https://www.googleapis.com/oauth2/v3/certs",
		[]string{"https://accounts.google.com", "accounts.google.com"}, audiences)
}

func NewApple(audiences []string) *Verifier {
	return New("https://appleid.apple.com/auth/keys",
		[]string{"https://appleid.apple.com"}, audiences)
}

// Enabled reports whether any audience is configured (else the provider is off).
func (v *Verifier) Enabled() bool { return v != nil && len(v.audiences) > 0 }

// Verify checks signature, issuer, audience and expiry, returning the claims.
func (v *Verifier) Verify(ctx context.Context, idToken string) (*Claims, error) {
	parts := strings.Split(strings.TrimSpace(idToken), ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &hdr); err != nil {
		return nil, fmt.Errorf("bad header: %w", err)
	}
	if hdr.Alg != "RS256" {
		return nil, fmt.Errorf("unexpected alg %q", hdr.Alg)
	}

	var claims struct {
		Iss           string          `json:"iss"`
		Aud           json.RawMessage `json:"aud"`
		Exp           int64           `json:"exp"`
		Sub           string          `json:"sub"`
		Email         string          `json:"email"`
		EmailVerified json.RawMessage `json:"email_verified"`
		Nonce         string          `json:"nonce"`
		NonceSupported bool           `json:"nonce_supported"`
		CHash         string          `json:"c_hash"`
	}
	if err := decodeSegment(parts[1], &claims); err != nil {
		return nil, fmt.Errorf("bad payload: %w", err)
	}
	if !v.issuers[claims.Iss] {
		return nil, fmt.Errorf("untrusted issuer %q", claims.Iss)
	}
	if !v.audienceOK(claims.Aud) {
		return nil, errors.New("audience mismatch")
	}
	if claims.Exp == 0 || time.Now().After(time.Unix(claims.Exp, 0).Add(30*time.Second)) {
		return nil, errors.New("token expired")
	}
	if claims.Sub == "" {
		return nil, errors.New("token missing subject")
	}

	key, err := v.key(ctx, hdr.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("bad signature encoding: %w", err)
	}
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, h[:], sig); err != nil {
		return nil, errors.New("signature verification failed")
	}
	return &Claims{
		Subject:        claims.Sub,
		Email:          strings.ToLower(strings.TrimSpace(claims.Email)),
		EmailVerified:  parseFlexBool(claims.EmailVerified),
		Issuer:         claims.Iss,
		Aud:            firstStringFromRaw(claims.Aud),
		Nonce:          claims.Nonce,
		NonceSupported: claims.NonceSupported,
		CHash:          claims.CHash,
	}, nil
}

// audienceOK accepts an `aud` that is either a JSON string or array of strings.
func (v *Verifier) audienceOK(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return v.audiences[one]
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		for _, a := range many {
			if v.audiences[a] {
				return true
			}
		}
	}
	return false
}

// key returns the RSA public key for kid, (re)fetching the JWKS on a cache miss.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	if k, ok := v.keys[kid]; ok && time.Since(v.fetchedAt) < v.ttl {
		v.mu.Unlock()
		return k, nil
	}
	v.mu.Unlock()
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("unknown signing key")
}

func (v *Verifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks fetch: status %d", resp.StatusCode)
	}
	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nb),
			E: int(new(big.Int).SetBytes(eb).Int64()),
		}
	}
	if len(keys) == 0 {
		return errors.New("jwks contained no usable RSA keys")
	}
	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

func decodeSegment(seg string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// parseFlexBool handles email_verified being a bool or the string "true".
func parseFlexBool(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.EqualFold(s, "true")
	}
	return false
}

func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" {
			m[it] = true
		}
	}
	return m
}

// firstStringFromRaw extracts the first audience from a JSON string or array.
func firstStringFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return one
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil && len(many) > 0 {
		return many[0]
	}
	return ""
}

// AppleNonceOK verifies that an Apple ID token's nonce claim matches the raw
// nonce supplied by the client. Different Apple clients pass the nonce either
// raw, SHA-256 hex encoded, or base64url encoded, so we accept all common
// representations.
func AppleNonceOK(rawNonce, tokenNonce string) bool {
	if tokenNonce == "" {
		return rawNonce == ""
	}
	if rawNonce == "" {
		return false
	}
	if rawNonce == tokenNonce {
		return true
	}
	sum := sha256.Sum256([]byte(rawNonce))
	if base64.RawURLEncoding.EncodeToString(sum[:]) == tokenNonce {
		return true
	}
	if fmt.Sprintf("%x", sum) == tokenNonce {
		return true
	}
	return false
}

// AppleCHashOK validates an Apple ID token's c_hash against the authorization
// code, per OIDC: c_hash = leftmost 128 bits of SHA256(code), base64url.
func AppleCHashOK(code, cHash string) bool {
	if cHash == "" || code == "" {
		return cHash == code
	}
	sum := sha256.Sum256([]byte(code))
	return base64.RawURLEncoding.EncodeToString(sum[:16]) == cHash
}
