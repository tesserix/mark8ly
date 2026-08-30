// Package admin — unit coverage for pickupEmailOrBuyerFallback (#483).
//
// A pure function test, deliberately in-package with no //go:build tag and
// no database: pickupEmailOrBuyerFallback has no I/O, so it doesn't need
// testdb or an integration run to exercise both branches.
package admin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPickupEmailOrBuyerFallback_PrefersTheWarehouseEmail pins the #483
// behavior change: once a pickup address carries its own email (collected
// via the admin shipping settings form and stored on the warehouses row),
// the label path must use it instead of the buyer's order email.
func TestPickupEmailOrBuyerFallback_PrefersTheWarehouseEmail(t *testing.T) {
	pickup := pickupAddress{Email: "warehouse@example.com"}

	got := pickupEmailOrBuyerFallback(pickup, "buyer@example.com")

	require.Equal(t, "warehouse@example.com", got, "the warehouse's own email must win, ignoring the buyer email entirely")
}

// TestPickupEmailOrBuyerFallback_FallsBackToTheBuyerEmailWhenBlank covers
// the case that still matters after #483: a pickup resolved from the
// legacy warehouse_* columns (see resolvePickupAddress) has no email
// column at all, so the buyer's order email is the only usable contact.
func TestPickupEmailOrBuyerFallback_FallsBackToTheBuyerEmailWhenBlank(t *testing.T) {
	pickup := pickupAddress{Email: ""}

	got := pickupEmailOrBuyerFallback(pickup, "buyer@example.com")

	require.Equal(t, "buyer@example.com", got)
}
