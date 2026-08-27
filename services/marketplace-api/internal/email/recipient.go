package email

// recipient.go — the guard that keeps billing mail out of the bit bucket
// and keeps the delivery counters honest.
//
// subscription/service.go mints billing+<store_id>@mark8ly.local whenever a
// subscription is bootstrapped without a real email. `.local` is unroutable,
// so sending there hard-bounces and costs sender reputation. Rather than
// discover that at the provider, we classify the address up front and return
// a typed error — never nil — so a caller can count the skip instead of
// recording a delivery that never happened.

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUndeliverable marks a recipient we refuse to attempt delivery to.
// Wrapped, so callers can errors.Is regardless of reason.
var ErrUndeliverable = errors.New("undeliverable recipient")

// Reasons an address is refused. These become the `reason` label on
// mark8ly_subscription_billing_emails_skipped_total, so keep them stable.
const (
	ReasonNoAddress          = "no_address"
	ReasonInvalidAddress     = "invalid_address"
	ReasonPlaceholderAddress = "placeholder_address"
)

// placeholderSuffixes are domains that can never receive mail: the RFC 2606
// reserved TLDs plus bare `localhost`. `.local` catches the
// billing+<uuid>@mark8ly.local addresses minted at bootstrap.
var placeholderSuffixes = []string{
	".local",
	".invalid",
	".test",
	".example",
	"localhost",
}

// UndeliverableError carries why an address was refused.
type UndeliverableError struct{ Reason string }

func (e *UndeliverableError) Error() string {
	return fmt.Sprintf("email: %s: %s", ErrUndeliverable.Error(), e.Reason)
}

// Unwrap lets errors.Is(err, ErrUndeliverable) succeed.
func (e *UndeliverableError) Unwrap() error { return ErrUndeliverable }

// ValidateRecipient reports whether `to` is worth handing to a provider.
// Deliberately conservative: this is a bounce guard, not an RFC 5321 parser.
func ValidateRecipient(to string) error {
	addr := strings.TrimSpace(to)
	if addr == "" {
		return &UndeliverableError{Reason: ReasonNoAddress}
	}

	local, domain, found := strings.Cut(addr, "@")
	if !found || local == "" || domain == "" || strings.Contains(domain, "@") {
		return &UndeliverableError{Reason: ReasonInvalidAddress}
	}

	lower := strings.ToLower(domain)
	for _, suffix := range placeholderSuffixes {
		if lower == suffix || strings.HasSuffix(lower, suffix) {
			return &UndeliverableError{Reason: ReasonPlaceholderAddress}
		}
	}
	if !strings.Contains(lower, ".") {
		// No dot at all — not a routable public domain.
		return &UndeliverableError{Reason: ReasonPlaceholderAddress}
	}
	return nil
}

// UndeliverableReason extracts the reason from an error produced by
// ValidateRecipient, reporting false for anything else (e.g. a transport
// failure, which must be counted differently).
func UndeliverableReason(err error) (string, bool) {
	var ue *UndeliverableError
	if errors.As(err, &ue) {
		return ue.Reason, true
	}
	return "", false
}
