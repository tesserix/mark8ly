// Package ticket — mailer.go
//
// Thin SendGrid v3 client for the two customer-facing transactional
// emails the support flow needs:
//
//   - NotifyTicketCreated:  fired by InternalHandler when slm-router
//     escalates an Otto chat to a human. Tells the customer their
//     ticket is logged and the merchant will respond.
//   - NotifyTicketResolved: fired by Service.UpdateStatus on every
//     transition to "resolved". Confirms the merchant marked it
//     done and links back to the case if the customer disagrees.
//
// Both emails are best-effort. SendGrid downtime or template errors
// must NOT fail the underlying HTTP request — the ticket exists in
// the DB regardless of email outcome.
package ticket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// SendGridNotifier implements TicketNotifier via the SendGrid v3
// /mail/send REST endpoint. No SDK, mirrors the orderdoc mailer
// shape so future template/translation work can extract a shared
// helper.
type SendGridNotifier struct {
	apiKey     string
	from       string
	publicHost string // e.g. "https://mystore.mark8ly.com" for deep links
	client     *http.Client
	logger     *slog.Logger
}

// NewSendGridNotifier constructs a notifier. publicHost is used to
// build customer-facing links into the storefront ticket page (e.g.
// `/support/tickets/{number}`). When empty, the email still renders
// but without the "view case" CTA.
func NewSendGridNotifier(apiKey, from, publicHost string, logger *slog.Logger) *SendGridNotifier {
	return &SendGridNotifier{
		apiKey:     apiKey,
		from:       from,
		publicHost: strings.TrimRight(publicHost, "/"),
		client:     &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

// NotifyTicketCreated emails the customer that a support case has
// been logged with a case ID they can quote.
func (m *SendGridNotifier) NotifyTicketCreated(ctx context.Context, t *Ticket) {
	if t == nil || t.SubmittedByEmail == "" {
		return
	}
	subject := fmt.Sprintf("Your support request is logged — case %s", t.TicketNumber)
	body := m.renderCreated(t)
	m.send(ctx, t.SubmittedByEmail, t.SubmittedByName, subject, body, "ticket_created")
}

// NotifyTicketResolved emails the customer that the merchant marked
// the ticket resolved, and tells them how to reopen.
func (m *SendGridNotifier) NotifyTicketResolved(ctx context.Context, t *Ticket) {
	if t == nil || t.SubmittedByEmail == "" {
		return
	}
	subject := fmt.Sprintf("Your support case %s is resolved", t.TicketNumber)
	body := m.renderResolved(t)
	m.send(ctx, t.SubmittedByEmail, t.SubmittedByName, subject, body, "ticket_resolved")
}

// ---------------------------------------------------------------------------
// HTML rendering — kept inline for now. When the template engine in
// emailtemplates supports ticket envelopes these become loader.Render()
// calls and the strings live in a DB row.
// ---------------------------------------------------------------------------

func (m *SendGridNotifier) renderCreated(t *Ticket) string {
	link := m.caseLink(t)
	return strings.Join([]string{
		`<p>Hi ` + html.EscapeString(coalesce(t.SubmittedByName, "there")) + `,</p>`,
		`<p>Thanks for reaching out. We've logged your support request as case <strong>` + html.EscapeString(t.TicketNumber) + `</strong>.</p>`,
		`<p>The merchant will respond shortly. Quote this case number if you need to follow up.</p>`,
		`<p><strong>Subject:</strong> ` + html.EscapeString(t.Subject) + `</p>`,
		ifNonEmpty(link, `<p><a href="`+html.EscapeString(link)+`">View case `+html.EscapeString(t.TicketNumber)+`</a></p>`),
		`<p style="color:#888;font-size:12px">If you didn't reach out for support you can safely ignore this email.</p>`,
	}, "\n")
}

func (m *SendGridNotifier) renderResolved(t *Ticket) string {
	link := m.caseLink(t)
	return strings.Join([]string{
		`<p>Hi ` + html.EscapeString(coalesce(t.SubmittedByName, "there")) + `,</p>`,
		`<p>Your support case <strong>` + html.EscapeString(t.TicketNumber) + `</strong> has been marked resolved.</p>`,
		`<p>If you're happy, no further action is needed. If the issue is still open, reply to this email or open the case below — we'll pick it up again.</p>`,
		`<p><strong>Subject:</strong> ` + html.EscapeString(t.Subject) + `</p>`,
		ifNonEmpty(link, `<p><a href="`+html.EscapeString(link)+`">Reopen case `+html.EscapeString(t.TicketNumber)+`</a></p>`),
		`<p style="color:#888;font-size:12px">Thanks for shopping with us.</p>`,
	}, "\n")
}

func (m *SendGridNotifier) caseLink(t *Ticket) string {
	if m.publicHost == "" || t == nil {
		return ""
	}
	return m.publicHost + "/support/tickets/" + t.TicketNumber
}

// ---------------------------------------------------------------------------
// SendGrid v3 client
// ---------------------------------------------------------------------------

type sgPersonalization struct {
	To []sgAddress `json:"to"`
}

type sgAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sgContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sgEnvelope struct {
	Personalizations []sgPersonalization `json:"personalizations"`
	From             sgAddress           `json:"from"`
	Subject          string              `json:"subject"`
	Content          []sgContent         `json:"content"`
}

func (m *SendGridNotifier) send(ctx context.Context, toEmail, toName, subject, body, kind string) {
	if m.apiKey == "" {
		// Log-only fallback in dev / when SendGrid isn't configured
		// yet. The merchant still sees the ticket in their dashboard
		// — the customer just doesn't get a copy.
		if m.logger != nil {
			m.logger.Info("ticket email skipped: SendGrid not configured",
				"kind", kind, "to", toEmail, "subject", subject)
		}
		return
	}
	env := sgEnvelope{
		Personalizations: []sgPersonalization{{To: []sgAddress{{Email: toEmail, Name: toName}}}},
		From:             sgAddress{Email: m.from, Name: "Mark8ly Support"},
		Subject:          subject,
		Content:          []sgContent{{Type: "text/html", Value: body}},
	}
	payload, err := json.Marshal(env)
	if err != nil {
		m.log("ticket email marshal failed", kind, err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.sendgrid.com/v3/mail/send", bytes.NewReader(payload))
	if err != nil {
		m.log("ticket email request build failed", kind, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := m.client.Do(req)
	if err != nil {
		m.log("ticket email send failed", kind, err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		m.log("ticket email rejected by SendGrid",
			kind, fmt.Errorf("status %d", res.StatusCode))
		return
	}
	if m.logger != nil {
		m.logger.Info("ticket email sent", "kind", kind, "to", toEmail)
	}
}

func (m *SendGridNotifier) log(msg, kind string, err error) {
	if m.logger == nil {
		return
	}
	m.logger.Warn(msg, "kind", kind, "err", err)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func coalesce(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func ifNonEmpty(check, html string) string {
	if check == "" {
		return ""
	}
	return html
}
