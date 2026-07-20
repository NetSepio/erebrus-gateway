package oauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	testIssuer = "https://accounts.google.com"
	testAud    = "client-123.apps.googleusercontent.com"
	testKid    = "test-key-1"
)

func newTestVerifier(t *testing.T) (*Verifier, *rsa.PrivateKey, func()) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	jwks := map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": testKid, "alg": "RS256", "use": "sig",
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	v := New(srv.URL, []string{testIssuer}, []string{testAud})
	return v, key, srv.Close
}

func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	header := enc(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"})
	payload := enc(claims)
	signing := header + "." + payload
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func baseClaims() map[string]any {
	return map[string]any{
		"iss": testIssuer, "aud": testAud, "sub": "user-sub-1",
		"email": "User@Example.com", "email_verified": true,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
}

func TestVerifyValidToken(t *testing.T) {
	v, key, done := newTestVerifier(t)
	defer done()
	tok := signToken(t, key, testKid, baseClaims())
	claims, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "user-sub-1" {
		t.Fatalf("subject = %q", claims.Subject)
	}
	if claims.Email != "user@example.com" { // lower-cased
		t.Fatalf("email = %q", claims.Email)
	}
	if !claims.EmailVerified {
		t.Fatal("email_verified should be true")
	}
}

func TestVerifyEmailVerifiedAsString(t *testing.T) {
	v, key, done := newTestVerifier(t)
	defer done()
	c := baseClaims()
	c["email_verified"] = "true" // Apple sometimes sends a string
	claims, err := v.Verify(context.Background(), signToken(t, key, testKid, c))
	if err != nil || !claims.EmailVerified {
		t.Fatalf("string email_verified not parsed: err=%v", err)
	}
}

func TestVerifyAudienceArray(t *testing.T) {
	v, key, done := newTestVerifier(t)
	defer done()
	c := baseClaims()
	c["aud"] = []string{"other", testAud}
	if _, err := v.Verify(context.Background(), signToken(t, key, testKid, c)); err != nil {
		t.Fatalf("array aud should pass: %v", err)
	}
}

func TestVerifyRejects(t *testing.T) {
	v, key, done := newTestVerifier(t)
	defer done()

	t.Run("bad audience", func(t *testing.T) {
		c := baseClaims()
		c["aud"] = "someone-else"
		if _, err := v.Verify(context.Background(), signToken(t, key, testKid, c)); err == nil {
			t.Fatal("expected audience mismatch")
		}
	})
	t.Run("wrong issuer", func(t *testing.T) {
		c := baseClaims()
		c["iss"] = "https://evil.example"
		if _, err := v.Verify(context.Background(), signToken(t, key, testKid, c)); err == nil {
			t.Fatal("expected untrusted issuer")
		}
	})
	t.Run("expired", func(t *testing.T) {
		c := baseClaims()
		c["exp"] = time.Now().Add(-time.Hour).Unix()
		if _, err := v.Verify(context.Background(), signToken(t, key, testKid, c)); err == nil {
			t.Fatal("expected expiry error")
		}
	})
	t.Run("unknown kid", func(t *testing.T) {
		if _, err := v.Verify(context.Background(), signToken(t, key, "no-such-kid", baseClaims())); err == nil {
			t.Fatal("expected unknown key error")
		}
	})
	t.Run("tampered signature", func(t *testing.T) {
		tok := signToken(t, key, testKid, baseClaims())
		if _, err := v.Verify(context.Background(), tok[:len(tok)-2]+"xy"); err == nil {
			t.Fatal("expected signature failure")
		}
	})
}

func TestEnabled(t *testing.T) {
	if New("u", []string{"i"}, nil).Enabled() {
		t.Fatal("no audiences => disabled")
	}
	if !New("u", []string{"i"}, []string{"a"}).Enabled() {
		t.Fatal("with audience => enabled")
	}
}

func TestAppleNonceOK(t *testing.T) {
	raw := "random-nonce-123"
	if !AppleNonceOK(raw, raw) {
		t.Fatal("raw nonce should match")
	}

	sum := sha256.Sum256([]byte(raw))
	b64 := base64.RawURLEncoding.EncodeToString(sum[:])
	if !AppleNonceOK(raw, b64) {
		t.Fatal("base64url SHA-256 nonce should match")
	}

	hex := fmt.Sprintf("%x", sum)
	if !AppleNonceOK(raw, hex) {
		t.Fatal("hex SHA-256 nonce should match")
	}

	if AppleNonceOK(raw, "other") {
		t.Fatal("mismatched nonce should fail")
	}
}

func TestAppleCHashOK(t *testing.T) {
	code := "test-code-456"
	sum := sha256.Sum256([]byte(code))
	cHash := base64.RawURLEncoding.EncodeToString(sum[:16])
	if !AppleCHashOK(code, cHash) {
		t.Fatal("valid c_hash should match")
	}
	if AppleCHashOK(code, "bad") {
		t.Fatal("invalid c_hash should fail")
	}
}
