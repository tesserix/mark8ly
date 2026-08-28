// Package emaillog records every outbound email as it is sent (#348, piece A).
//
// It exists because transactional mail was fire-and-forget: every mailer
// handed an envelope to email.Sender and nothing wrote a row, so "did the
// merchant get the email?" could not be answered from our own data.
//
// # Why not in internal/email
//
// That package is provider-only and has no database dependency. Putting a
// write in it would invert that, and every provider adapter would inherit a
// reason to know about GORM. This wraps the transport instead, which is also
// why coverage is complete: a mailer cannot opt out of being logged without
// opting out of sending.
package emaillog

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/email"
)

// CustomArgSendID is the custom-arg key carrying this send's id.
//
// SendGrid echoes custom_args on every engagement event and Resend mirrors
// them as tags, so the value returns verbatim from either provider. The row's
// primary key and the correlation key are the SAME value — there is no join
// table and no second identifier to keep in step (#348 piece B depends on
// this).
const CustomArgSendID = "send_id"

// Attribution keys read from the caller's CustomArgs. Every mailer using
// email.Sender already sets these, so this package changes no mailer.
const (
	customArgKind     = "kind"
	customArgTenantID = "tenant_id"
	customArgStoreID  = "store_id"
)

// KindUnknown is recorded when a mailer supplies no kind.
//
// Mirrors the ReasonUnknown fallback from #336: a mailer added later without
// attribution shows up as unattributed and queryable, rather than writing an
// empty string nobody notices.
const KindUnknown = "unknown"

// Send status values. Mirrors the CHECK constraint in migration 000108.
const (
	statusSending = "sending"
	statusSent    = "sent"
	statusFailed  = "failed"
)

// maxErrorLen bounds what a provider error can write. Errors are provider
// text and occasionally carry a whole response body.
const maxErrorLen = 2000

// Sender wraps an email.Sender and records each send.
type Sender struct {
	inner  email.Sender
	db     *gorm.DB
	logger *slog.Logger
}

// NewSender wraps inner so every send it performs is recorded in email_sends.
// logger may be nil.
func NewSender(inner email.Sender, db *gorm.DB, logger *slog.Logger) *Sender {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sender{inner: inner, db: db, logger: logger}
}

// Send records the attempt, delivers, then records the outcome.
//
// # Write-before-send is deliberate
//
// A process death mid-send leaves a row at `sending`, which is
// DISTINGUISHABLE from "never attempted" and therefore actionable. Writing a
// single row after the send instead would lose the record entirely on a
// crash — the exact silent gap this exists to close, merely narrowed to a
// smaller window. Same reasoning as the outbox `pending` state in #336: a
// stuck row that can be seen is worth more than a clean absence.
//
// # A failing log write never blocks the send
//
// If either write errors it is logged loudly and delivery proceeds. An
// observability feature that can take down transactional mail is worse than
// no observability feature. The consequence — a failed pre-write means an
// unlogged send — is accepted deliberately.
func (s *Sender) Send(ctx context.Context, msg email.Message) error {
	sendID := uuid.New()

	if err := s.begin(ctx, sendID, msg); err != nil {
		s.logger.Error("emaillog: could not record send attempt; delivering anyway",
			"send_id", sendID, "kind", attr(msg, customArgKind, KindUnknown), "err", err)
		// Deliberately NOT returning: see the doc comment. The send proceeds
		// without a log row rather than the mail not going out.
		return s.inner.Send(ctx, withSendID(msg, sendID))
	}

	sendErr := s.inner.Send(ctx, withSendID(msg, sendID))
	s.finish(ctx, sendID, sendErr)
	return sendErr
}

// withSendID returns a copy of msg carrying the send id.
//
// Copy-on-write: the caller's CustomArgs map may be shared across sends or
// reused, so it is never mutated.
func withSendID(msg email.Message, sendID uuid.UUID) email.Message {
	args := make(map[string]string, len(msg.CustomArgs)+1)
	for k, v := range msg.CustomArgs {
		args[k] = v
	}
	args[CustomArgSendID] = sendID.String()
	msg.CustomArgs = args
	return msg
}

func (s *Sender) begin(ctx context.Context, sendID uuid.UUID, msg email.Message) error {
	return s.db.WithContext(ctx).Exec(
		`INSERT INTO email_sends (id, tenant_id, store_id, recipient, kind, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, now())`,
		sendID,
		uuidArg(msg, customArgTenantID),
		uuidArg(msg, customArgStoreID),
		msg.To,
		attr(msg, customArgKind, KindUnknown),
		statusSending,
	).Error
}

func (s *Sender) finish(ctx context.Context, sendID uuid.UUID, sendErr error) {
	var err error
	if sendErr != nil {
		err = s.db.WithContext(ctx).Exec(
			`UPDATE email_sends SET status = ?, error = ? WHERE id = ?`,
			statusFailed, truncate(sendErr.Error(), maxErrorLen), sendID).Error
	} else {
		err = s.db.WithContext(ctx).Exec(
			`UPDATE email_sends SET status = ?, sent_at = ? WHERE id = ?`,
			statusSent, time.Now().UTC(), sendID).Error
	}
	if err != nil {
		// The mail is already delivered (or already failed). Nothing is
		// recoverable here; the row stays at `sending`, which is exactly the
		// stuck state the partial index exists to find.
		s.logger.Error("emaillog: could not record send outcome; row left at sending",
			"send_id", sendID, "err", err)
	}
}

// attr reads a CustomArgs value, falling back when absent or blank.
func attr(msg email.Message, key, fallback string) string {
	if v, ok := msg.CustomArgs[key]; ok && v != "" {
		return v
	}
	return fallback
}

// uuidArg parses an attribution id, returning nil for absent or malformed
// values. Nil is correct rather than defensive: platform-level mail (signup,
// anomaly cron) genuinely has no tenant or store, which is why both columns
// are nullable.
func uuidArg(msg email.Message, key string) *uuid.UUID {
	raw, ok := msg.CustomArgs[key]
	if !ok || raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
