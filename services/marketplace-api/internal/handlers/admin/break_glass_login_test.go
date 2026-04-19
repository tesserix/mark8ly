package admin

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBreakGlassUserID_DeterministicPerTenant(t *testing.T) {
	tenantA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	tenantB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	a1 := BreakGlassUserID(tenantA)
	a2 := BreakGlassUserID(tenantA)
	b1 := BreakGlassUserID(tenantB)

	require.Equal(t, a1, a2, "must be stable per tenant")
	require.NotEqual(t, a1, b1, "must differ across tenants")
	require.NotEqual(t, a1, tenantA, "must not collide with tenant id")
}

func TestBreakGlassSessionTTL_IsTwoHours(t *testing.T) {
	// Pins the spec §12.4 invariant — short-lived emergency session.
	require.Equal(t, int64(7200), int64(BreakGlassSessionTTL.Seconds()))
}
