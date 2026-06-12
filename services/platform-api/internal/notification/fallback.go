package notification

import (
	"context"
	"errors"
	"log/slog"
)

// FallbackSender chains any number of Senders: every Email goes to the
// first provider, and each later provider is tried only when the one
// before it errored. Production wiring is built by NewFromConfig from
// EMAIL_PRIMARY_PROVIDER, so the order is pure config — e.g. Resend
// primary with SendGrid catching its failures, or the reverse — and a
// provider outage degrades to a provider switch instead of dropped
// transactional mail.
//
// The failover is per-message and stateless — no circuit breaker. The
// first provider is retried first on every send, so recovery is automatic
// and there is no shared state to reset.
type FallbackSender struct {
	providers []Sender
	log       *slog.Logger
}

// NewFallbackChain constructs a FallbackSender that tries providers in
// the given order.
func NewFallbackChain(log *slog.Logger, providers ...Sender) *FallbackSender {
	return &FallbackSender{providers: providers, log: log}
}

// NewFallbackSender constructs a two-provider chain. Kept as the
// common-case constructor; NewFallbackChain accepts any number.
func NewFallbackSender(primary, fallback Sender, log *slog.Logger) *FallbackSender {
	return NewFallbackChain(log, primary, fallback)
}

// Send tries each provider in order until one delivers. Validation errors
// are returned immediately — a malformed Email fails on every provider,
// so retrying it would only burn the other providers' quota. All provider
// errors are joined when the whole chain fails so the log shows the full
// failure picture.
func (f *FallbackSender) Send(ctx context.Context, msg Email) error {
	if err := validate(msg); err != nil {
		return err
	}

	var errs []error
	for i, p := range f.providers {
		err := p.Send(ctx, msg)
		if err == nil {
			if i > 0 {
				f.log.Info("notification: fallback provider delivered",
					slog.String("provider", nameOf(p)),
					slog.String("to", msg.To),
					slog.String("subject", msg.Subject),
				)
			}
			return nil
		}
		errs = append(errs, err)

		// A cancelled context will fail on every remaining provider —
		// bail early instead of burning their quota.
		if ctx.Err() != nil {
			break
		}
		if i < len(f.providers)-1 {
			f.log.Warn("notification: provider failed, retrying on next",
				slog.String("provider", nameOf(p)),
				slog.String("next", nameOf(f.providers[i+1])),
				slog.String("to", msg.To),
				slog.String("subject", msg.Subject),
				slog.String("error", err.Error()),
			)
		}
	}
	return errors.Join(errs...)
}
