package trial

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// NO BUILD TAG, NO DATABASE — deliberately. stripeIdempotencyKey is a pure
// function of its three arguments, and this file exercises it directly
// (package trial, not trial_test) precisely so these run on every `go test
// ./...` without a live TEST_DATABASE_URL or a Stripe test-mode account
// (#358 N2).
//
// The fixed instant used throughout has a 10-digit Unix second
// (1798675200 = 2026-12-31T00:00:00Z), matching what production actually
// appends until the year 2286 — the boundary math below is only correct for
// that width.
var idemKeyFixtureEnd = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

// callerKeyOfLen returns a caller idempotency key of exactly n characters,
// built from a repeating alphabet so a human skimming a failure message
// can immediately see it is a fixture, not real operator input.
func callerKeyOfLen(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(alphabet)
	}
	return b.String()[:n]
}

// #358 N2. base := callerKey, key := base + ":" + unixSeconds. unixSeconds
// for idemKeyFixtureEnd is 10 digits, so the suffix is exactly 11 characters
// (":" + 10 digits). A callerKey of 244 characters therefore produces a key
// of EXACTLY 255 characters — Stripe's own documented limit — which must
// stay unhashed and human-readable. This fixture sits ON that boundary, not
// merely near it.
func TestStripeIdempotencyKey_AtLimit_StaysReadable(t *testing.T) {
	caller := callerKeyOfLen(244)
	storeID := uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111")

	got := stripeIdempotencyKey(caller, storeID, idemKeyFixtureEnd)

	require.Len(t, got, stripeIdempotencyKeyMaxLen,
		"fixture is constructed to land exactly on stripe's 255-character limit")
	require.True(t, strings.HasPrefix(got, caller+":"),
		"a key at or under the limit must stay human-readable, not be hashed")
	require.Contains(t, got, "1798675200", "the readable form still carries the target unix second")
}

// #358 N2. One character longer than the previous fixture pushes the
// derived key to 256 characters — one over Stripe's limit — which must be
// hashed down to something comfortably inside it, and the hash must be
// STABLE: the same caller key and target second computed twice must always
// produce the same Stripe key, or Stripe's own dedupe (the reason this
// function exists at all) breaks.
func TestStripeIdempotencyKey_OverLimit_HashedAndStable(t *testing.T) {
	caller := callerKeyOfLen(245)
	storeID := uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111")

	unhashedLen := len(caller) + 1 + len("1798675200")
	require.Equal(t, stripeIdempotencyKeyMaxLen+1, unhashedLen,
		"fixture is constructed to land exactly one character over stripe's limit")

	first := stripeIdempotencyKey(caller, storeID, idemKeyFixtureEnd)
	second := stripeIdempotencyKey(caller, storeID, idemKeyFixtureEnd)

	require.LessOrEqual(t, len(first), stripeIdempotencyKeyMaxLen,
		"an over-limit caller key must be collapsed to fit stripe's 255-character limit")
	require.Equal(t, first, second,
		"the SAME caller key and target second must ALWAYS hash to the SAME stripe key — "+
			"anything time- or random-derived here would silently break stripe's dedupe")
	require.NotEqual(t, caller+":1798675200", first,
		"a key over the limit must actually be hashed, not passed through unhashed")
}

// #358 N2. Two DIFFERENT long caller keys must still land on two DIFFERENT
// Stripe keys after hashing — otherwise two distinct operator requests
// would collide at Stripe exactly the way this function was introduced
// (#358 F1) to prevent.
func TestStripeIdempotencyKey_OverLimit_DifferentCallersDifferentHashes(t *testing.T) {
	callerA := callerKeyOfLen(300)
	callerB := "z" + callerKeyOfLen(299)
	storeID := uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111")

	gotA := stripeIdempotencyKey(callerA, storeID, idemKeyFixtureEnd)
	gotB := stripeIdempotencyKey(callerB, storeID, idemKeyFixtureEnd)

	require.LessOrEqual(t, len(gotA), stripeIdempotencyKeyMaxLen)
	require.LessOrEqual(t, len(gotB), stripeIdempotencyKeyMaxLen)
	require.NotEqual(t, gotA, gotB, "two different caller keys must never collide after hashing")
}

// #358 N2. A short, ordinary key (the common case) must remain completely
// untouched — no hashing overhead, no prefix change — so an operator
// reading Stripe's dashboard still sees a recognisable key.
func TestStripeIdempotencyKey_ShortKey_Unchanged(t *testing.T) {
	storeID := uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111")
	got := stripeIdempotencyKey("trial_extend:store-x:op-123", storeID, idemKeyFixtureEnd)
	require.Equal(t, "trial_extend:store-x:op-123:1798675200", got)
}
