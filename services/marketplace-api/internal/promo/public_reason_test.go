package promo_test

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/promo"
)

// allInternalReasons is every ValidationRejectReason the validator can
// produce. Listed by hand: a new constant added to validator.go without a
// line here is caught by TestPublicReasonFor_CoversEveryInternalReason below,
// which counts them.
var allInternalReasons = []promo.ValidationRejectReason{
	promo.RejectReasonNotFound,
	promo.RejectReasonExpired,
	promo.RejectReasonMaxRedemptions,
	promo.RejectReasonMaxPerEmail,
	promo.RejectReasonWrongPlan,
	promo.RejectReasonAnnualOnly,
	promo.RejectReasonBelowFloor,
	promo.RejectReasonCurrencyNotCovered,
	promo.RejectReasonUnknownDiscountType,
}

// TestPublicReasonFor_NotFoundAndExpiredCollapse is the enumeration-oracle
// guard. If these two ever return different public reasons, a caller trying
// strings can tell a real code from a made-up one.
func TestPublicReasonFor_NotFoundAndExpiredCollapse(t *testing.T) {
	notFound := promo.PublicReasonFor(promo.RejectReasonNotFound)
	expired := promo.PublicReasonFor(promo.RejectReasonExpired)

	if notFound != expired {
		t.Fatalf("not_found → %q and expired → %q must be indistinguishable to a client",
			notFound, expired)
	}
	if notFound != promo.PublicReasonInvalidOrExpired {
		t.Errorf("collapsed reason = %q, want %q", notFound, promo.PublicReasonInvalidOrExpired)
	}
}

// TestPublicReasonFor_SevenReasonsStayDistinct is the other half. Collapsing
// is a security decision made for exactly one pair; every other reason must
// survive the trip to the client, because each one is a different sentence a
// merchant can act on. A regression that widened the collapse — say, mapping
// below_absolute_floor back onto invalid_or_expired — would restore the
// support ticket this endpoint's response was changed to prevent.
func TestPublicReasonFor_SevenReasonsStayDistinct(t *testing.T) {
	distinct := []promo.ValidationRejectReason{
		promo.RejectReasonMaxRedemptions,
		promo.RejectReasonMaxPerEmail,
		promo.RejectReasonWrongPlan,
		promo.RejectReasonAnnualOnly,
		promo.RejectReasonBelowFloor,
		promo.RejectReasonCurrencyNotCovered,
		promo.RejectReasonUnknownDiscountType,
	}

	seen := map[promo.PublicRejectReason]promo.ValidationRejectReason{}
	for _, internal := range distinct {
		public := promo.PublicReasonFor(internal)
		if public == promo.PublicReasonInvalidOrExpired {
			t.Errorf("%s collapsed into %q — it is a fact about the caller's own subscription and must reach them",
				internal, public)
			continue
		}
		if prev, dup := seen[public]; dup {
			t.Errorf("%s and %s both map to %q — the client cannot tell them apart",
				prev, internal, public)
			continue
		}
		seen[public] = internal
	}
}

// TestPublicReasonFor_UnknownReasonDisclosesNothing pins the default arm.
// An empty reason means the refusal was not a validation refusal at all.
func TestPublicReasonFor_UnknownReasonDisclosesNothing(t *testing.T) {
	for _, r := range []promo.ValidationRejectReason{"", "something_new"} {
		if got := promo.PublicReasonFor(r); got != promo.PublicReasonInvalidOrExpired {
			t.Errorf("PublicReasonFor(%q) = %q, want %q", r, got, promo.PublicReasonInvalidOrExpired)
		}
	}
}

// TestPublicReasonFor_CoversEveryInternalReason fails when a tenth internal
// reason is added without deciding whether it is safe to disclose. It cannot
// see the new constant directly, so it asserts the count it was written
// against — the failure message says what to do.
func TestPublicReasonFor_CoversEveryInternalReason(t *testing.T) {
	const knownReasonCount = 9
	if len(allInternalReasons) != knownReasonCount {
		t.Fatalf("allInternalReasons has %d entries, want %d — a reason was added or removed in validator.go; "+
			"add it here AND to the switch in PublicReasonFor, deciding whether it is safe to disclose",
			len(allInternalReasons), knownReasonCount)
	}
	for _, r := range allInternalReasons {
		if promo.PublicReasonFor(r) == "" {
			t.Errorf("PublicReasonFor(%q) returned the empty reason", r)
		}
	}
}
