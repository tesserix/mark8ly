package refund

import "errors"

// ErrOutsideCoolingOff is returned when the 14-day cooling-off window has
// closed (§8 pattern E). The charge.created timestamp is authoritative.
var ErrOutsideCoolingOff = errors.New("refund: outside 14-day cooling-off window")

// ErrCardFingerprintExists is returned when the same card fingerprint already
// has a refund record (§8 pattern D — cross-subscription fraud guard).
// Exposed as generic "refund_unavailable" at the HTTP layer.
var ErrCardFingerprintExists = errors.New("refund: card fingerprint already refunded")

// ErrProAppNotRefundable is returned when the store has the white-label app
// add-on (Pro+App setup fee is never refundable per §8).
var ErrProAppNotRefundable = errors.New("refund: Pro+App setup fee is not refundable")

// ErrChargeNotFound is returned when Stripe returns 404 for the charge lookup.
// Callers fall back to local first_charge_at when this occurs.
var ErrChargeNotFound = errors.New("refund: stripe charge not found")
