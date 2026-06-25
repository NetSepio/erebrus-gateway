// Package socialverify verifies ownership of third-party social accounts (X,
// Telegram) for the XP/social layer. It stores only the provider id + handle,
// never tokens. Each verifier is backend-only and dependency-free.
package socialverify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrTelegram is returned when a Telegram Login Widget payload fails validation.
var ErrTelegram = errors.New("telegram verification failed")

// VerifyTelegram validates a Telegram Login Widget payload against the bot token
// (per Telegram's spec: HMAC-SHA256 over the sorted data-check-string, keyed by
// SHA256(bot_token)) and returns (providerID, handle). `fields` are the widget's
// key=value pairs including "hash"; auth_date guards replay when maxAge > 0.
func VerifyTelegram(botToken string, fields map[string]string, maxAge time.Duration) (string, string, error) {
	hash := strings.ToLower(strings.TrimSpace(fields["hash"]))
	id := fields["id"]
	if botToken == "" || hash == "" || id == "" {
		return "", "", ErrTelegram
	}

	keys := make([]string, 0, len(fields))
	for k := range fields {
		if k == "hash" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(fields[k])
	}

	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(b.String()))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(hash)) {
		return "", "", ErrTelegram
	}

	if maxAge > 0 {
		if ts, err := strconv.ParseInt(fields["auth_date"], 10, 64); err == nil {
			if time.Since(time.Unix(ts, 0)) > maxAge {
				return "", "", errors.New("telegram auth_date too old")
			}
		}
	}
	return id, fields["username"], nil
}
