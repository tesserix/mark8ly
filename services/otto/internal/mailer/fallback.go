package mailer

import (
	"context"
	"errors"
	"log/slog"
)

// FallbackMailer chains any number of Mailers: every OTP goes to the
// first provider, and each later provider is tried only when the one
// before it errored. Production wiring is built by NewFromConfig from
// EMAIL_PRIMARY_PROVIDER, so the order is pure config — e.g. Resend
// primary with SendGrid catching its failures, or the reverse — and a
// provider outage degrades to a provider switch instead of customers
// stuck at the OTP screen.
//
// Failover is per-message and stateless — no circuit breaker. The first
// provider is retried first on every send, so recovery is automatic.
type FallbackMailer struct {
	providers []Mailer
	log       *slog.Logger
}

// NewFallbackChain constructs a FallbackMailer that tries providers in
// the given order.
func NewFallbackChain(log *slog.Logger, providers ...Mailer) *FallbackMailer {
	return &FallbackMailer{providers: providers, log: log}
}

// SendOTP tries each provider in order until one delivers. All provider
// errors are joined when the whole chain fails so the log shows the full
// failure picture.
func (m *FallbackMailer) SendOTP(ctx context.Context, tenantID, to, recipientName, code, storeName string) error {
	var errs []error
	for i, p := range m.providers {
		err := p.SendOTP(ctx, tenantID, to, recipientName, code, storeName)
		if err == nil {
			if i > 0 && m.log != nil {
				m.log.Info("otp mailer: fallback provider delivered",
					slog.String("provider", nameOf(p)),
					slog.String("to", to),
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
		if i < len(m.providers)-1 && m.log != nil {
			m.log.Warn("otp mailer: provider failed, retrying on next",
				slog.String("provider", nameOf(p)),
				slog.String("next", nameOf(m.providers[i+1])),
				slog.String("tenant_id", tenantID),
				slog.String("to", to),
				slog.String("error", err.Error()),
			)
		}
	}
	return errors.Join(errs...)
}
