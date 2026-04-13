package campaign

import (
	"context"
	"log/slog"
)

// Dispatcher delivers fully-rendered campaign emails. The send worker
// loads the store branding once per campaign dispatch and renders the
// editorial envelope before calling Send — the dispatcher itself is a
// thin transport adapter and does no theming work.
//
// Implementations:
//   - LogDispatcher       — dev / no-API-key fallback, logs only
//   - SendGridDispatcher  — production, POSTs to SendGrid v3 API
type Dispatcher interface {
	Send(ctx context.Context, msg OutboundEmail) error
}

// OutboundEmail is one rendered campaign email ready to hand to a
// provider. The html and text bodies have already been wrapped in the
// store's branded envelope by the send worker.
type OutboundEmail struct {
	Recipient string
	Subject   string
	HTMLBody  string
	TextBody  string
}

// LogDispatcher is a no-op Dispatcher that logs each send instead of
// delivering. Used when SENDGRID_API_KEY is empty (local dev, tests).
type LogDispatcher struct {
	Logger *slog.Logger
}

// Send logs the email dispatch without actually sending.
func (d *LogDispatcher) Send(_ context.Context, msg OutboundEmail) error {
	d.Logger.Info("campaign: dispatching email (log-only)",
		"recipient", msg.Recipient,
		"subject", msg.Subject,
		"html_bytes", len(msg.HTMLBody),
		"text_bytes", len(msg.TextBody))
	return nil
}
