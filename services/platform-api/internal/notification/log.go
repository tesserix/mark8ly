package notification

import (
	"context"
	"log/slog"
)

// LogSender writes the email payload to the logger instead of sending it.
// Used in local dev when SENDGRID_API_KEY is unset, so devs can:
//   - see exactly what would have been sent
//   - copy the OTP code from the log to test the verification flow without
//     hitting a real inbox
//
// This replaces the "test helper" route from the old codebase, which existed
// solely to bypass the email send during e2e tests. Now there's nothing to
// bypass: the LogSender is the test path.
type LogSender struct {
	log *slog.Logger
}

// NewLogSender constructs a LogSender that writes to the given logger.
func NewLogSender(log *slog.Logger) *LogSender {
	return &LogSender{log: log}
}

// Send logs the email payload at INFO level. Always succeeds.
func (l *LogSender) Send(ctx context.Context, msg Email) error {
	if err := validate(msg); err != nil {
		return err
	}
	l.log.Info("notification (log-only)",
		slog.String("to", msg.To),
		slog.String("from", msg.From),
		slog.String("subject", msg.Subject),
		slog.String("text_body", msg.TextBody),
	)
	return nil
}
