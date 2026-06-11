package notification

import (
	"context"
	"errors"
	"log/slog"
)

// FallbackSender chains two Senders: every Email goes to primary first, and
// only when primary errors does it retry on fallback. Production wiring is
// SendGrid (primary) → Resend (fallback) so a SendGrid outage, rate limit
// or account suspension degrades to a provider switch instead of dropped
// transactional mail.
//
// The failover is per-message and stateless — no circuit breaker. SendGrid
// is retried first on every send, so recovery is automatic and there is no
// shared state to reset.
type FallbackSender struct {
	primary  Sender
	fallback Sender
	log      *slog.Logger
}

// NewFallbackSender constructs a FallbackSender.
func NewFallbackSender(primary, fallback Sender, log *slog.Logger) *FallbackSender {
	return &FallbackSender{primary: primary, fallback: fallback, log: log}
}

// Send tries primary, then fallback. Validation errors are returned
// immediately — a malformed Email fails on every provider, so retrying it
// would only burn the fallback's quota.
func (f *FallbackSender) Send(ctx context.Context, msg Email) error {
	if err := validate(msg); err != nil {
		return err
	}

	primaryErr := f.primary.Send(ctx, msg)
	if primaryErr == nil {
		return nil
	}
	// A cancelled context will fail on the fallback too — bail early.
	if ctx.Err() != nil {
		return primaryErr
	}

	f.log.Warn("notification: primary provider failed, retrying on fallback",
		slog.String("to", msg.To),
		slog.String("subject", msg.Subject),
		slog.String("error", primaryErr.Error()),
	)

	fallbackErr := f.fallback.Send(ctx, msg)
	if fallbackErr == nil {
		f.log.Info("notification: fallback provider delivered",
			slog.String("to", msg.To),
			slog.String("subject", msg.Subject),
		)
		return nil
	}
	return errors.Join(primaryErr, fallbackErr)
}
