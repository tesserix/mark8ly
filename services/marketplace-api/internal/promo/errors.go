package promo

import "errors"

// ErrInvalidOrExpired is the single Go error returned for every promo
// validation failure (§7.3 pattern A), so no caller can branch on the failure
// mode by comparing errors. The reason travels beside it on
// ApplyOutput.RejectReason.
//
// It is no longer the whole story at the HTTP layer: the handler returns
// PublicReasonFor(out.RejectReason) alongside the message, which keeps
// not_found and expired indistinguishable while letting the seven reasons
// that describe the caller's own subscription through. See public_reason.go.
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
