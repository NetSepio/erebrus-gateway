// Package mailer sends transactional email via the Resend HTTP API
// (https://resend.com). It is dependency-free (net/http + encoding/json) and
// fully optional: when no API key is configured, Enabled() is false and the
// gateway's email features report "not configured" rather than failing.
package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.resend.com"

// ErrDisabled is returned when a send is attempted but no API key is configured.
var ErrDisabled = errors.New("email is not configured")

// Mailer posts to the Resend API. The zero value (or a nil *Mailer) is disabled.
type Mailer struct {
	apiKey  string
	from    string
	baseURL string
	httpc   *http.Client
}

// New builds a Mailer. An empty apiKey yields a disabled mailer (Enabled()==false).
func New(apiKey, from string) *Mailer {
	if strings.TrimSpace(from) == "" {
		from = "Erebrus <no-reply@erebrus.network>"
	}
	return &Mailer{
		apiKey:  strings.TrimSpace(apiKey),
		from:    from,
		baseURL: defaultBaseURL,
		httpc:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled reports whether an API key is configured.
func (m *Mailer) Enabled() bool { return m != nil && m.apiKey != "" }

// SendOTP emails a 6-digit verification code.
func (m *Mailer) SendOTP(ctx context.Context, to, code string) error {
	subject := "Your Erebrus verification code"
	text := fmt.Sprintf("Your Erebrus verification code is %s.\n\nIt expires shortly. If you didn't request this, you can ignore this email.", code)
	html := fmt.Sprintf(`<p>Your Erebrus verification code is</p>`+
		`<p style="font-size:24px;font-weight:700;letter-spacing:4px">%s</p>`+
		`<p>It expires shortly. If you didn't request this, you can ignore this email.</p>`, code)
	return m.send(ctx, to, subject, text, html)
}

func (m *Mailer) send(ctx context.Context, to, subject, text, html string) error {
	if !m.Enabled() {
		return ErrDisabled
	}
	payload, err := json.Marshal(map[string]any{
		"from":    m.from,
		"to":      []string{to},
		"subject": subject,
		"text":    text,
		"html":    html,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("resend request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("resend status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
