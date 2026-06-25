package socialverify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// telegramHash recomputes the widget hash for a payload (the client-side step).
func telegramHash(botToken string, fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+fields[k])
	}
	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyTelegram(t *testing.T) {
	const bot = "123456:test-bot-token"
	fields := map[string]string{
		"id":        "777000",
		"username":  "erebrususer",
		"auth_date": strconv.FormatInt(time.Now().Unix(), 10),
	}
	fields["hash"] = telegramHash(bot, fields)

	id, handle, err := VerifyTelegram(bot, fields, 24*time.Hour)
	if err != nil {
		t.Fatalf("VerifyTelegram: %v", err)
	}
	if id != "777000" || handle != "erebrususer" {
		t.Fatalf("got id=%q handle=%q", id, handle)
	}

	// Tampered field invalidates the hash.
	bad := map[string]string{"id": "999", "username": "erebrususer", "auth_date": fields["auth_date"], "hash": fields["hash"]}
	if _, _, err := VerifyTelegram(bot, bad, 24*time.Hour); err == nil {
		t.Fatal("expected failure for tampered payload")
	}

	// Stale auth_date rejected.
	old := map[string]string{"id": "777000", "auth_date": strconv.FormatInt(time.Now().Add(-48*time.Hour).Unix(), 10)}
	old["hash"] = telegramHash(bot, old)
	if _, _, err := VerifyTelegram(bot, old, 24*time.Hour); err == nil {
		t.Fatal("expected failure for stale auth_date")
	}
}

func TestVerifyX(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2/users/me" || r.Header.Get("Authorization") != "Bearer tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"id":"42","username":"erebrus"}}`))
	}))
	defer srv.Close()

	x := NewXVerifier(srv.URL)
	id, handle, err := x.Verify(context.Background(), "tok123")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id != "42" || handle != "erebrus" {
		t.Fatalf("got id=%q handle=%q", id, handle)
	}
	if _, _, err := x.Verify(context.Background(), "wrong"); err == nil {
		t.Fatal("expected failure for bad token")
	}
}
