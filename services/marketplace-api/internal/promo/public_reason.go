package promo

// public_reason.go — which rejection reasons may leave the server.
//
// ValidationRejectReason is an internal record. This file decides what a
// merchant's browser is told, which is a narrower question with a different
// threat model: the audit trail is read by us, the HTTP response is read by
// whoever is holding the keyboard.

// PublicRejectReason is the machine-readable rejection reason returned on the
// apply-promo response, for a client to map onto merchant-facing copy.
//
// It is a distinct type from ValidationRejectReason on purpose. The two sets
// are deliberately NOT the same size, and giving them one type would make the
// collapse below invisible at every call site.
type PublicRejectReason string

const (
	// PublicReasonInvalidOrExpired is the collapsed reason covering BOTH
	// RejectReasonNotFound and RejectReasonExpired.
	//
	// They stay merged because separating them is an enumeration oracle: a
	// caller trying strings could tell "this code does not exist" from "this
	// code exists but has expired", and the second answer confirms a real
	// code for free. Nothing a merchant can act on is lost — either way the
	// code they hold will not work and retyping it will not help.
	//
	// Every other reason below is safe to return because it is a fact about
	// the caller's OWN subscription (plan, billing period, currency, price
	// floor) or their own prior use of the code. None of them confirms
	// anything about a code the caller has not already been given.
	PublicReasonInvalidOrExpired PublicRejectReason = "invalid_or_expired"

	PublicReasonMaxRedemptions     PublicRejectReason = "max_redemptions_reached"
	PublicReasonMaxPerEmail        PublicRejectReason = "max_per_email_reached"
	PublicReasonWrongPlan          PublicRejectReason = "wrong_plan"
	PublicReasonAnnualOnly         PublicRejectReason = "annual_only"
	PublicReasonBelowFloor         PublicRejectReason = "below_absolute_floor"
	PublicReasonCurrencyNotCovered PublicRejectReason = "currency_not_covered"
	PublicReasonUnknownDiscount    PublicRejectReason = "unknown_discount_type"
)

// PublicReasonFor maps an internal reason onto the one the client is told.
//
// The switch is written out in full rather than as a passthrough with two
// special cases, so that adding a tenth ValidationRejectReason forces an
// explicit decision here about whether it is safe to disclose. The default
// arm is the safe answer, not the common one: an unrecognised reason — and
// the empty reason, which means the failure was not a validation failure at
// all — discloses nothing.
func PublicReasonFor(r ValidationRejectReason) PublicRejectReason {
	switch r {
	case RejectReasonNotFound, RejectReasonExpired:
		return PublicReasonInvalidOrExpired
	case RejectReasonMaxRedemptions:
		return PublicReasonMaxRedemptions
	case RejectReasonMaxPerEmail:
		return PublicReasonMaxPerEmail
	case RejectReasonWrongPlan:
		return PublicReasonWrongPlan
	case RejectReasonAnnualOnly:
		return PublicReasonAnnualOnly
	case RejectReasonBelowFloor:
		return PublicReasonBelowFloor
	case RejectReasonCurrencyNotCovered:
		return PublicReasonCurrencyNotCovered
	case RejectReasonUnknownDiscountType:
		return PublicReasonUnknownDiscount
	default:
		return PublicReasonInvalidOrExpired
	}
}
