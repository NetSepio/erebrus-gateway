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

// SendOrgInvite emails a branded org membership invitation.
func (m *Mailer) SendOrgInvite(ctx context.Context, to string, data OrgInviteEmail) error {
	orgName := strings.TrimSpace(data.OrgName)
	inviteURL := strings.TrimSpace(data.InviteURL)
	subject := fmt.Sprintf("You've been invited to %s on Erebrus", orgName)
	inviter := strings.TrimSpace(data.InviterName)
	role := strings.TrimSpace(data.Role)
	text := fmt.Sprintf("You've been invited to join %s on Erebrus.\n\n", orgName)
	if inviter != "" && role != "" {
		text += fmt.Sprintf("%s invited you as %s.\n\n", inviter, role)
	}
	text += fmt.Sprintf("Accept your invitation:\n%s\n\nIf you didn't expect this invitation, you can ignore this email.", inviteURL)
	return m.send(ctx, to, subject, text, renderOrgInviteHTML(data))
}

// SendOrgInviteAccepted notifies parties that an invite was accepted.
func (m *Mailer) SendOrgInviteAccepted(ctx context.Context, to, orgDisplayName, inviteeLabel, role, workspaceURL string, toInviter bool) error {
	subject, text, html := renderOrgInviteAcceptedHTML(orgDisplayName, inviteeLabel, role, workspaceURL, toInviter)
	return m.send(ctx, to, subject, text, html)
}

// SendOrgInviteDeclined notifies parties that an invite was declined.
func (m *Mailer) SendOrgInviteDeclined(ctx context.Context, to, orgDisplayName, inviteeLabel, role string, toInviter bool) error {
	subject, text, html := renderOrgInviteDeclinedHTML(orgDisplayName, inviteeLabel, role, toInviter)
	return m.send(ctx, to, subject, text, html)
}

// SendDeletionProcessed confirms to a user that their account deletion request was fulfilled.
func (m *Mailer) SendDeletionProcessed(ctx context.Context, to string) error {
	subject := "Your Erebrus account has been deleted"
	text := "Your account deletion request has been processed and your personal information has been removed. Your wallet address has been retained for our records. If you did not request this deletion, please contact support immediately."
	return m.send(ctx, to, subject, text, "")
}

// SendOTP emails a 6-digit verification code. The optional app argument customizes
// the product name in the subject and body (e.g. "Erebrus Drop", "Erebrus AI");
// an empty value defaults to "Erebrus".
func (m *Mailer) SendOTP(ctx context.Context, to, code, app string) error {
	product := otpProductName(app)
	subject := fmt.Sprintf("Your %s verification code", product)
	text := fmt.Sprintf("Your %s verification code is %s.\n\nIt expires shortly. If you didn't request this, you can ignore this email.", product, code)
	return m.send(ctx, to, subject, text, renderOTPHTML(code, product))
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
