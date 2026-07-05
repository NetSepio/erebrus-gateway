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
		from = "Erebrus <no-reply@info.erebrus.io>"
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

// SendOrgInvite emails an org membership invitation with a link to sign in.
func (m *Mailer) SendOrgInvite(ctx context.Context, to, orgName, inviteURL string) error {
	subject := fmt.Sprintf("You've been invited to %s on Erebrus", orgName)
	text := fmt.Sprintf("You've been invited to join %s on Erebrus.\n\nSign in with your wallet and verify your email to accept:\n%s\n\nIf you didn't expect this invitation, you can ignore this email.", orgName, inviteURL)
	html := fmt.Sprintf(`<p>You've been invited to join <strong>%s</strong> on Erebrus.</p>`+
		`<p><a href="%s">Sign in and accept your invitation</a></p>`+
		`<p>If you didn't expect this invitation, you can ignore this email.</p>`, orgName, inviteURL)
	return m.send(ctx, to, subject, text, html)
}

// SendOrgInviteAccepted notifies parties that an invite was accepted.
func (m *Mailer) SendOrgInviteAccepted(ctx context.Context, to, orgDisplayName, inviteeLabel, role, workspaceURL string, toInviter bool) error {
	subject := fmt.Sprintf("%s joined %s on Erebrus", inviteeLabel, orgDisplayName)
	if toInviter {
		subject = fmt.Sprintf("%s accepted your %s workspace invite", inviteeLabel, orgDisplayName)
	}
	text := fmt.Sprintf("%s accepted the invitation to join %s as %s.\n\nOpen workspace: %s",
		inviteeLabel, orgDisplayName, role, workspaceURL)
	html := fmt.Sprintf(`<p><strong>%s</strong> accepted the invitation to join <strong>%s</strong> as <strong>%s</strong>.</p>`+
		`<p><a href="%s">Open workspace</a></p>`, inviteeLabel, orgDisplayName, role, workspaceURL)
	if !toInviter {
		subject = fmt.Sprintf("You're now a member of %s", orgDisplayName)
		text = fmt.Sprintf("Welcome to %s — you joined as %s.\n\nOpen your workspace: %s", orgDisplayName, role, workspaceURL)
		html = fmt.Sprintf(`<p>Welcome to <strong>%s</strong>. You joined as <strong>%s</strong>.</p>`+
			`<p><a href="%s">Open workspace</a></p>`, orgDisplayName, role, workspaceURL)
	}
	return m.send(ctx, to, subject, text, html)
}

// SendOrgInviteDeclined notifies parties that an invite was declined.
func (m *Mailer) SendOrgInviteDeclined(ctx context.Context, to, orgDisplayName, inviteeLabel, role string, toInviter bool) error {
	subject := fmt.Sprintf("Invite to %s was declined", orgDisplayName)
	if toInviter {
		subject = fmt.Sprintf("%s declined your %s workspace invite", inviteeLabel, orgDisplayName)
	}
	text := fmt.Sprintf("%s declined the invitation to join %s as %s.", inviteeLabel, orgDisplayName, role)
	html := fmt.Sprintf(`<p><strong>%s</strong> declined the invitation to join <strong>%s</strong> as <strong>%s</strong>.</p>`,
		inviteeLabel, orgDisplayName, role)
	if !toInviter {
		subject = fmt.Sprintf("You declined the %s workspace invite", orgDisplayName)
		text = fmt.Sprintf("You declined the invitation to join %s as %s. No further action is needed.", orgDisplayName, role)
		html = fmt.Sprintf(`<p>You declined the invitation to join <strong>%s</strong> as <strong>%s</strong>.</p>`+
			`<p>No further action is needed.</p>`, orgDisplayName, role)
	}
	return m.send(ctx, to, subject, text, html)
}

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
