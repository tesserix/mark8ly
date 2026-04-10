package campaign

import (
	"context"
	"log/slog"
)

// Dispatcher is the email delivery interface. The send worker uses this
// to dispatch emails. Ship a LogDispatcher (no-op) for M4; the Pub/Sub
// follow-up becomes a clean interface swap.
type Dispatcher interface {
	Send(ctx context.Context, recipient, subject, body string) error
}

// LogDispatcher is a no-op Dispatcher that logs each send instead of
// delivering. Used for M4 launch until real email integration is wired.
type LogDispatcher struct {
	Logger *slog.Logger
}

// Send logs the email dispatch without actually sending.
func (d *LogDispatcher) Send(_ context.Context, recipient, subject, _ string) error {
	d.Logger.Info("campaign: dispatching email (log-only)",
		"recipient", recipient,
		"subject", subject)
	return nil
}
