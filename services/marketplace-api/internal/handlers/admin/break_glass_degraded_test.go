package admin

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
)

// #468: when the lockout store cannot be read, the login path used to fail
// OPEN unconditionally — the error was logged at Warn and `locked` stayed
// false, so an existing lockout went unenforced.
//
// #457 proved that is not hypothetical: marketplace_api held no privileges
// on break_glass_lockouts, so every read failed, and the only outward sign
// was a Warn line nobody was reading.
//
// Failing CLOSED unconditionally is not the answer either. It would turn a
// database blip into "nobody can break-glass in" — precisely when
// break-glass is most likely to be needed, since this endpoint exists to
// survive states other paths cannot.
//
// So the decision degrades to the in-memory limiter, which is per-pod and
// resets on deploy but is the one signal still available: an IP already at
// the failure threshold is refused, and an IP with no recent failures is
// still let through to try its credentials.

func TestDegradedLockDecision_RefusesAnIPAlreadyAtTheThreshold(t *testing.T) {
	require.True(t, degradedLockDecision(breakglass.LoginMaxFailures),
		"an IP that has already hit the threshold must not gain access just "+
			"because the lockout store is unreachable")
	require.True(t, degradedLockDecision(breakglass.LoginMaxFailures+5))
}

func TestDegradedLockDecision_AllowsAnIPWithNoRecentFailures(t *testing.T) {
	require.False(t, degradedLockDecision(0),
		"a database blip must not lock out an operator who has done nothing wrong — "+
			"break-glass exists to work when other things do not")
	require.False(t, degradedLockDecision(breakglass.LoginMaxFailures-1),
		"below the threshold behaves exactly as it would with the store readable")
}

// The boundary is the same one RecordFailure uses to decide whether to
// persist a lockout, so degraded and normal operation agree on what
// "too many" means rather than drifting apart.
func TestDegradedLockDecision_SharesTheThresholdWithNormalOperation(t *testing.T) {
	require.False(t, degradedLockDecision(breakglass.LoginMaxFailures-1))
	require.True(t, degradedLockDecision(breakglass.LoginMaxFailures))
}
