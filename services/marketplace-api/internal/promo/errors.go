package promo

import "errors"

// ErrInvalidOrExpired is the uniform external error returned for every
// promo validation failure (§7.3 pattern A). The true reason is recorded
// in the audit event but never exposed to the HTTP client.
var ErrInvalidOrExpired = errors.New("promo: invalid or expired")

// ErrAlreadyApplied is returned when the store already has an active
// promo redemption (internal; mapped to ErrInvalidOrExpired at the HTTP layer).
var ErrAlreadyApplied = errors.New("promo: already applied to this store")

// ErrBelowAbsoluteFloor is returned by CheckFloor when the effective
// discounted price falls below the per-plan/currency absolute floor (§7.4).
var ErrBelowAbsoluteFloor = errors.New("promo: effective price below absolute floor")

// ErrCurrencyNotCovered is returned by CheckFloor when the billing currency
// has no floor entry defined for the given plan.
var ErrCurrencyNotCovered = errors.New("promo: currency not covered by floor table")

// ErrProAppRefund indicates a Pro+App setup fee refund was requested, which
// is never refundable per §8.
var ErrProAppRefund = errors.New("refund: Pro+App setup fee is not refundable")
