package mailer

import (
	"fmt"
	"log/slog"
	"strings"
)

// Registered provider names. Adding a provider is three small steps:
//
//  1. write an adapter implementing Mailer (one file — see resend.go for
//     the template) and give it a Name() so chain logs identify it;
//  2. register a factory in providerFactories below and append the name
//     to defaultOrder;
//  3. surface its API key in internal/config and pass it through the keys
//     map in cmd/server/main.go.
//
// Everything else — ordering, failover, logging, degraded modes — is
// generic and picks the new provider up automatically.
const (
	ProviderSendGrid = "sendgrid"
	ProviderResend   = "resend"
)

// defaultOrder is the chain ordering for providers not named in
// EMAIL_PRIMARY_PROVIDER. Listed explicitly (not map iteration) so chain
// composition is deterministic.
var defaultOrder = []string{ProviderSendGrid, ProviderResend}

// providerFactories build a provider adapter from its API key and the
// configured From identity.
var providerFactories = map[string]func(apiKey, fromEmail, fromName string) Mailer{
	ProviderSendGrid: func(k, e, n string) Mailer { return NewSendgridMailer(k, e, n) },
	ProviderResend:   func(k, e, n string) Mailer { return NewResendMailer(k, e, n) },
}

// Named is optionally implemented by providers so chain logs can identify
// them. Adapters should implement it; anything else falls back to its
// Go type name.
type Named interface{ Name() string }

func nameOf(m Mailer) string {
	if n, ok := m.(Named); ok {
		return n.Name()
	}
	return fmt.Sprintf("%T", m)
}

// NewFromConfig builds the process-wide Mailer. keys maps provider name →
// API key (empty = not configured). order is the EMAIL_PRIMARY_PROVIDER
// value — a single provider name or a comma-separated preference list
// ("resend" or "resend,sendgrid"). Providers named there send first, in
// that order; configured providers not named follow in defaultOrder.
// Unknown names are warned about and skipped, so a typo can never take
// OTP mail down.
//
//   - ≥2 providers configured → FallbackMailer chain in the resolved order
//   - 1 provider configured   → that provider alone (warn: no fallback)
//   - none configured         → LogMailer (dev — codes print to stdout)
//
// Every provider ships the identical OTP body (shared otpEmail builder),
// so reordering is purely a config change.
func NewFromConfig(keys map[string]string, order, fromEmail, fromName string, log *slog.Logger) Mailer {
	var (
		chain []Mailer
		names []string
	)
	for _, name := range resolveOrder(order, log) {
		if keys[name] == "" {
			continue
		}
		chain = append(chain, providerFactories[name](keys[name], fromEmail, fromName))
		names = append(names, name)
	}

	switch len(chain) {
	case 0:
		log.Warn("otp: no provider API keys set — falling back to stdout log mailer (codes will not reach real inboxes)")
		return &LogMailer{Logger: log}
	case 1:
		log.Warn("otp: single provider configured — no fallback",
			slog.String("provider", names[0]))
		return chain[0]
	default:
		log.Info("otp: provider chain configured",
			slog.String("order", strings.Join(names, " → ")))
		return NewFallbackChain(log, chain...)
	}
}

// resolveOrder turns the EMAIL_PRIMARY_PROVIDER value into a full
// provider-name ordering: named (and known) providers first, then every
// remaining registered provider in defaultOrder.
func resolveOrder(order string, log *slog.Logger) []string {
	seen := make(map[string]bool, len(defaultOrder))
	var out []string
	for _, raw := range strings.Split(order, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" || seen[name] {
			continue
		}
		if _, ok := providerFactories[name]; !ok {
			log.Warn("otp: unknown provider in EMAIL_PRIMARY_PROVIDER — skipping",
				slog.String("value", name))
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, name := range defaultOrder {
		if !seen[name] {
			out = append(out, name)
		}
	}
	return out
}
