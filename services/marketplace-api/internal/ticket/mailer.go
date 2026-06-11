// Package ticket — mailer.go
//
// Mailer for the two customer-facing transactional emails the support
// flow needs:
//
//   - NotifyTicketCreated:  fired by InternalHandler when slm-router
//     escalates an Otto chat to a human. Tells the customer their
//     ticket is logged and the merchant will respond.
//   - NotifyTicketResolved: fired by Service.UpdateStatus on every
//     transition to "resolved". Confirms the merchant marked it
//     done and links back to the case if the customer disagrees.
//
// Both emails are best-effort. Provider downtime or template errors
// must NOT fail the underlying HTTP request — the ticket exists in
// the DB regardless of email outcome. Delivery rides the shared
// internal/email transport (SendGrid primary, Resend fallback).
package ticket

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/email"
)

// EmailNotifier implements TicketNotifier on top of the shared
// internal/email transport. Mirrors the orderdoc mailer shape so
// future template/translation work can extract a shared helper.
type EmailNotifier struct {
	sender     email.Sender
	from       string
	publicHost string // e.g. "https://mystore.mark8ly.com" for deep links
	logger     *slog.Logger
}

// NewEmailNotifier constructs a notifier. publicHost is used to
// build customer-facing links into the storefront ticket page (e.g.
// `/support/tickets/{number}`). When empty, the email still renders
// but without the "view case" CTA.
func NewEmailNotifier(sender email.Sender, from, publicHost string, logger *slog.Logger) *EmailNotifier {
	return &EmailNotifier{
		sender:     sender,
		from:       from,
		publicHost: strings.TrimRight(publicHost, "/"),
		logger:     logger,
	}
}

// NotifyTicketCreated emails the customer that a support case has
// been logged with a case ID they can quote.
func (m *EmailNotifier) NotifyTicketCreated(ctx context.Context, t *Ticket) {
	if t == nil || t.SubmittedByEmail == "" {
		return
	}
	subject := fmt.Sprintf("Your support request is logged — case %s", t.TicketNumber)
	body := m.renderCreated(t)
	m.send(ctx, t.SubmittedByEmail, t.SubmittedByName, subject, body, "ticket_created")
}

// NotifyTicketResolved emails the customer that the merchant marked
// the ticket resolved, and tells them how to reopen.
func (m *EmailNotifier) NotifyTicketResolved(ctx context.Context, t *Ticket) {
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

func (m *EmailNotifier) renderCreated(t *Ticket) string {
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

func (m *EmailNotifier) renderResolved(t *Ticket) string {
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

func (m *EmailNotifier) caseLink(t *Ticket) string {
	if m.publicHost == "" || t == nil {
		return ""
	}
	return m.publicHost + "/support/tickets/" + t.TicketNumber
}

// ---------------------------------------------------------------------------
// dispatch
// ---------------------------------------------------------------------------

// send hands the rendered envelope to the shared transport. Errors are
// logged, never returned — ticket emails are best-effort by contract
// (see the package comment). In dev the transport is a LogSender, so
// the dispatch path still runs end-to-end without a provider account.
func (m *EmailNotifier) send(ctx context.Context, toEmail, toName, subject, body, kind string) {
	err := m.sender.Send(ctx, email.Message{
		From:     m.from,
		FromName: "Mark8ly Support",
		To:       toEmail,
		ToName:   toName,
		Subject:  subject,
		HTMLBody: body,
		// Wave 1.5 attribution — product/kind let notification-service
		// group engagement events without parsing subjects.
		CustomArgs: map[string]string{"product": "mark8ly", "kind": kind},
	})
	if err != nil {
		m.log("ticket email send failed", kind, err)
		return
	}
	if m.logger != nil {
		m.logger.Info("ticket email sent", "kind", kind, "to", toEmail)
	}
}

func (m *EmailNotifier) log(msg, kind string, err error) {
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
