package mailer

import (
	"fmt"
	"html"
	"strings"
	"time"
)

const (
	brandBg       = "#0A0A0C"
	brandSurface  = "#141418"
	brandAccent   = "#FF6B35"
	brandAccentHi = "#FF8C5A"
	brandText     = "#F4F3F0"
	brandText2    = "#9B9A97"
	brandText3    = "#5C5B58"
)

// OrgInviteEmail carries data for a workspace invitation message.
type OrgInviteEmail struct {
	OrgName     string
	InviterName string
	Role        string
	InviteURL   string
	LogoURL     string
}

func (d OrgInviteEmail) logo() string {
	if u := strings.TrimSpace(d.LogoURL); u != "" {
		return u
	}
	logoURL, _ := brandURLs("Erebrus")
	return logoURL
}

func (d OrgInviteEmail) inviterLine() string {
	inviter := strings.TrimSpace(d.InviterName)
	role := strings.TrimSpace(d.Role)
	if inviter != "" && role != "" {
		return fmt.Sprintf("<strong>%s</strong> invited you to join as <strong>%s</strong>.",
			html.EscapeString(inviter), html.EscapeString(role))
	}
	if inviter != "" {
		return fmt.Sprintf("<strong>%s</strong> invited you to join their workspace.", html.EscapeString(inviter))
	}
	if role != "" {
		return fmt.Sprintf("You have been invited to join as <strong>%s</strong>.", html.EscapeString(role))
	}
	return "You have been invited to join a workspace on Erebrus."
}

func brandURLs(product string) (logoURL, siteURL string) {
	switch strings.ToLower(strings.TrimSpace(product)) {
	case "erebrus ai", "ai":
		return "https://erebrus.io/ai/logo.png", "https://erebrus.io/ai"
	case "erebrus drop", "drop":
		return "https://erebrus.io/drop/logo.png", "https://erebrus.io/drop"
	case "erebrus vpn", "vpn":
		return "https://erebrus.io/vpn/logo.png", "https://erebrus.io/vpn"
	case "erebrus":
		return "https://erebrus.io/favicon.ico", "https://erebrus.io"
	}
	return "https://erebrus.io/favicon.ico", "https://erebrus.io"
}

func renderBrandedEmail(preheader, title, bodyHTML, product string) string {
	year := time.Now().Year()
	preheader = html.EscapeString(strings.TrimSpace(preheader))
	title = html.EscapeString(strings.TrimSpace(title))
	product = html.EscapeString(strings.TrimSpace(product))
	if product == "" {
		product = "Erebrus"
	}
	logoURL, siteURL := brandURLs(product)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="dark">
<title>%s</title>
</head>
<body style="margin:0;padding:0;background:%s;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<span style="display:none!important;visibility:hidden;opacity:0;height:0;width:0;overflow:hidden;">%s</span>
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:%s;padding:32px 16px;">
<tr><td align="center">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;background:%s;border:1px solid rgba(255,255,255,0.08);border-radius:16px;overflow:hidden;">
<tr><td style="padding:28px 32px 20px;text-align:center;border-bottom:1px solid rgba(255,255,255,0.06);">
<a href="%s" style="text-decoration:none;">
<img src="%s" alt="%s" width="48" height="48" style="display:block;margin:0 auto 12px;border-radius:12px;">
</a>
<div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:%s;font-weight:600;">%s</div>
</td></tr>
<tr><td style="padding:32px;">
<h1 style="margin:0 0 16px;font-size:22px;line-height:1.3;font-weight:700;color:%s;">%s</h1>
%s
</td></tr>
<tr><td style="padding:24px 32px 28px;border-top:1px solid rgba(255,255,255,0.06);background:rgba(0,0,0,0.2);">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
<tr><td style="font-size:12px;line-height:1.6;color:%s;text-align:center;">
<div style="margin-bottom:8px;">Erebrus &copy; %d NetSepio LLC</div>
<div>
<a href="https://erebrus.io/privacy" style="color:%s;text-decoration:none;">Privacy</a>
&nbsp;&middot;&nbsp;
<a href="https://erebrus.io/terms" style="color:%s;text-decoration:none;">Terms</a>
&nbsp;&middot;&nbsp;
<a href="mailto:support@netsepio.com" style="color:%s;text-decoration:none;">Contact</a>
</div>
<div style="margin-top:8px;color:%s;">support@netsepio.com</div>
</td></tr>
</table>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`,
		title,
		brandBg, preheader, brandBg,
		brandSurface,
		siteURL, logoURL, product, brandText3, product,
		brandText, title,
		bodyHTML,
		brandText2, year,
		brandAccentHi, brandAccentHi, brandAccentHi,
		brandText3,
	)
}

func ctaButton(href, label string) string {
	href = html.EscapeString(strings.TrimSpace(href))
	label = html.EscapeString(strings.TrimSpace(label))
	return fmt.Sprintf(`<table role="presentation" cellpadding="0" cellspacing="0" style="margin:24px 0 8px;">
<tr><td style="border-radius:10px;background:%s;">
<a href="%s" style="display:inline-block;padding:14px 28px;font-size:15px;font-weight:600;color:#0A0A0C;text-decoration:none;">%s</a>
</td></tr>
</table>`, brandAccent, href, label)
}

func bodyParagraph(text string) string {
	return fmt.Sprintf(`<p style="margin:0 0 14px;font-size:15px;line-height:1.65;color:%s;">%s</p>`, brandText2, text)
}

func renderOrgInviteHTML(data OrgInviteEmail) string {
	org := html.EscapeString(strings.TrimSpace(data.OrgName))
	url := strings.TrimSpace(data.InviteURL)
	body := bodyParagraph(data.inviterLine()) +
		bodyParagraph(fmt.Sprintf("Join <strong style=\"color:%s;\">%s</strong> on Erebrus — your private network for devices, teams, and nodes.", brandText, org)) +
		ctaButton(url, "Accept invitation") +
		bodyParagraph(fmt.Sprintf(`Or copy this link:<br><a href="%s" style="color:%s;word-break:break-all;">%s</a>`,
			html.EscapeString(url), brandAccentHi, html.EscapeString(url))) +
		bodyParagraph("If you didn't expect this invitation, you can safely ignore this email.")
	return renderBrandedEmail(
		fmt.Sprintf("Join %s on Erebrus", data.OrgName),
		fmt.Sprintf("Join %s", data.OrgName),
		body,
		"Erebrus",
	)
}

func renderOrgInviteAcceptedHTML(org, invitee, role, workspaceURL string, toInviter bool) (subject, text, htmlOut string) {
	org = strings.TrimSpace(org)
	invitee = strings.TrimSpace(invitee)
	role = strings.TrimSpace(role)
	if invitee == "" {
		invitee = "A user"
	}
	if role == "" {
		role = "Member"
	}
	url := strings.TrimSpace(workspaceURL)

	if toInviter {
		subject = fmt.Sprintf("%s accepted your %s workspace invite", invitee, org)
		text = fmt.Sprintf("%s accepted the invitation to join %s as %s.\n\nOpen workspace: %s",
			invitee, org, role, url)
		body := bodyParagraph(fmt.Sprintf("<strong style=\"color:%s;\">%s</strong> accepted your invitation to join <strong style=\"color:%s;\">%s</strong> as <strong>%s</strong>.",
			brandText, html.EscapeString(invitee), brandText, html.EscapeString(org), html.EscapeString(role))) +
			ctaButton(url, "Open workspace")
		htmlOut = renderBrandedEmail(subject, "Invitation accepted", body, "Erebrus")
		return
	}

	subject = fmt.Sprintf("You're now a member of %s", org)
	text = fmt.Sprintf("Welcome to %s — you joined as %s.\n\nOpen your workspace: %s", org, role, url)
	body := bodyParagraph(fmt.Sprintf("Welcome to <strong style=\"color:%s;\">%s</strong>. You joined as <strong>%s</strong>.",
		brandText, html.EscapeString(org), html.EscapeString(role))) +
		bodyParagraph("You can now access workspace nodes, manage clients, and collaborate with your team.") +
		ctaButton(url, "Open workspace")
	htmlOut = renderBrandedEmail(subject, fmt.Sprintf("Welcome to %s", org), body, "Erebrus")
	return
}

func renderOrgInviteDeclinedHTML(org, invitee, role string, toInviter bool) (subject, text, htmlOut string) {
	org = strings.TrimSpace(org)
	invitee = strings.TrimSpace(invitee)
	role = strings.TrimSpace(role)
	if invitee == "" {
		invitee = "A user"
	}
	if role == "" {
		role = "Member"
	}

	if toInviter {
		subject = fmt.Sprintf("%s declined your %s workspace invite", invitee, org)
		text = fmt.Sprintf("%s declined the invitation to join %s as %s.", invitee, org, role)
		body := bodyParagraph(fmt.Sprintf("<strong style=\"color:%s;\">%s</strong> declined the invitation to join <strong style=\"color:%s;\">%s</strong> as <strong>%s</strong>.",
			brandText, html.EscapeString(invitee), brandText, html.EscapeString(org), html.EscapeString(role))) +
			bodyParagraph("No further action is required on your part.")
		htmlOut = renderBrandedEmail(subject, "Invitation declined", body, "Erebrus")
		return
	}

	subject = fmt.Sprintf("You declined the %s workspace invite", org)
	text = fmt.Sprintf("You declined the invitation to join %s as %s. No further action is needed.", org, role)
	body := bodyParagraph(fmt.Sprintf("You declined the invitation to join <strong style=\"color:%s;\">%s</strong> as <strong>%s</strong>.",
		brandText, html.EscapeString(org), html.EscapeString(role))) +
		bodyParagraph("No further action is needed.")
	htmlOut = renderBrandedEmail(subject, "Invitation declined", body, "Erebrus")
	return
}

func otpProductName(app string) string {
	app = strings.TrimSpace(app)
	if app == "" {
		return "Erebrus"
	}
	switch strings.ToLower(app) {
	case "drop", "erebrus-drop", "erebrus drop":
		return "Erebrus Drop"
	case "ai", "erebrus-ai", "erebrus ai":
		return "Erebrus AI"
	case "vpn", "erebrus-vpn", "erebrus vpn":
		return "Erebrus VPN"
	case "erebrus":
		return "Erebrus"
	}
	return "Erebrus"
}

func renderOTPHTML(code, product string) string {
	product = strings.TrimSpace(product)
	if product == "" {
		product = "Erebrus"
	}
	code = html.EscapeString(strings.TrimSpace(code))
	displayProduct := html.EscapeString(product)
	body := bodyParagraph(fmt.Sprintf("Enter this verification code to sign in to %s:", displayProduct)) +
		fmt.Sprintf(`<p style="margin:20px 0;font-size:32px;font-weight:700;letter-spacing:0.35em;color:%s;text-align:center;">%s</p>`, brandAccent, code) +
		bodyParagraph("This code expires shortly. If you didn't request it, you can ignore this email.")
	return renderBrandedEmail(
		fmt.Sprintf("Your %s verification code", product),
		"Verification code",
		body,
		product,
	)
}
