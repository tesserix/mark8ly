// Package consolepromo ingests promo-code DEFINITIONS from the tesserix-home
// console into this service's promo_codes table (#726 step 2).
//
// # Why rows and not a cache
//
// The obvious shape for "read a catalog from the console" is the one
// internal/billing/consolecatalog uses: fetch, cache, serve from memory,
// fall open to something older. That is not sufficient here.
// promo_redemptions.promo_code_id is NOT NULL REFERENCES promo_codes(id)
// (migration 000061), so a redemption must point at a real row. The codes
// have to land in the table; an in-memory catalog could never be redeemed
// against.
//
// So this package is deliberately consolecatalog's shape for the transport
// (the same OAuth client-credentials token, the same fetch-with-ETag, the
// same "unconfigured is a supported state") with a database write where
// consolecatalog has a cache.
//
// # Direction of ownership
//
// The console owns promo definitions; mark8ly consumes them and never mints
// one (#726). There is intentionally no local create path here.
package consolepromo

import (
	"encoding/json"
	"time"
)

// Catalog is the console's published promo catalog. The shape is a contract
// owned by the console: additive changes only.
type Catalog struct {
	Source     string `json:"source"`
	Mode       string `json:"mode"`
	RevisionID string `json:"revision_id"`
	Codes      []Code `json:"codes"`
}

// Code is one published promo definition.
//
// Every optional field is a pointer or a json.Number rather than a plain
// value, because for each of them the zero value is a legitimate reading
// that must not be confused with absence: max_redemptions 0 would mean "no
// redemptions permitted" while absent means "unlimited", and
// trial_extension_days 0 would mean "extends the trial by nothing" while
// absent means "extends no trial".
type Code struct {
	Code string `json:"code"`
	// TrialExtensionDays is nil when the code extends no trial.
	TrialExtensionDays *int `json:"trial_extension_days"`
	// Discount is nil for a trial-extension-only code.
	Discount *Discount `json:"discount"`
	// ValidFrom is nil when the console did not bound the start; the mapper
	// then uses the ingest time, matching the column's own DEFAULT now().
	ValidFrom *time.Time `json:"valid_from"`
	// ValidUntil is nil for a code with no end date.
	ValidUntil *time.Time `json:"valid_until"`
	// MaxRedemptions is nil for unlimited global redemptions.
	MaxRedemptions *int `json:"max_redemptions"`
}

// Discount is the money part of a promo definition.
type Discount struct {
	// Kind is DiscountKindPercentOff or DiscountKindAmountOff.
	Kind string `json:"kind"`
	// PercentOff arrives as the console's numeric(5,2) — "50.00", "33.33".
	//
	// It is a json.Number, NOT a float64, and that is load-bearing.
	// promo_codes.discount_value holds basis points, so this has to be
	// multiplied by 100; doing that through a binary float eventually lands
	// on 3332 or 3334 for 33.33. json.Number keeps the literal digits so the
	// conversion can be done exactly, as decimal string arithmetic. Empty
	// means the key was absent or null.
	PercentOff json.Number `json:"percent_off"`
	// AmountOffMinor is minor units of Currency. Nil when the discount is
	// not an amount-off.
	AmountOffMinor *int64 `json:"amount_off_minor"`
	// Currency names AmountOffMinor's currency. promo_codes has no currency
	// column — discount_value for an amount code is read in the store's
	// billing currency (000060) — so this is carried for diagnostics and
	// deliberately not mapped. Widening the table to hold it is #726's open
	// question, not this ingest's.
	Currency string `json:"currency"`
	// Duration is DurationOnce, DurationRepeating or DurationForever.
	Duration string `json:"duration"`
	// DurationInMonths is set only when Duration is DurationRepeating.
	DurationInMonths *int `json:"duration_in_months"`
	// StripeCouponID is nil when the KEY IS ABSENT, which is how the console
	// reports "no coupon minted in this Stripe mode" — distinct from a
	// present-but-empty string, which is malformed and rejected by the
	// mapper. Absent maps to a NULL column (migration 000131 made it
	// nullable precisely for this).
	StripeCouponID *string `json:"stripe_coupon_id"`
}

// Discount kinds the console publishes.
const (
	DiscountKindPercentOff = "percent_off"
	DiscountKindAmountOff  = "amount_off"
)

// Discount durations the console publishes.
const (
	DurationOnce      = "once"
	DurationRepeating = "repeating"
	DurationForever   = "forever"
)
