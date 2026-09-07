package audit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// drainBudget is the deadline every test in this package hands to
// Emitter.Stop before reading back the row it just emitted.
//
// It is DERIVED from audit.WriteTimeout rather than being a number of its
// own, because the two are not independent choices. Stop's drain is
// best-effort and bounded by the caller's context, while the worker's
// insert runs on its own WriteTimeout-bounded context; a budget shorter
// than WriteTimeout lets Stop return on its deadline while a healthy
// insert is still in flight, and the read that follows finds nothing.
// That is mark8ly#804: the tests used a flat 2s against a 5s write and
// passed only because the database was usually fast enough. Writing it
// as a derivation means the budget cannot silently fall below the write
// again if WriteTimeout is ever changed.
//
// The extra second is head-room, not superstition: Stop's clock starts
// when Stop is called, whereas the write it is waiting on may only have
// begun a moment earlier and may legitimately consume its full
// WriteTimeout. Exactly WriteTimeout would leave zero margin for that
// offset plus the queue handoff.
const drainBudget = audit.WriteTimeout + time.Second

// Compile-time floor. Constant expressions are evaluated by the compiler,
// and converting a negative constant to uint is a compile error, so this
// line stops the package building at all if drainBudget is ever edited
// below audit.WriteTimeout — including via a change to WriteTimeout
// itself. A failing build is a louder signal than a flaky test, which is
// the failure mode this whole file exists to remove.
const _ = uint(drainBudget - audit.WriteTimeout)

// TestDrainBudgetCoversTheWriteTimeout states the same invariant in a
// form a reader will actually see when it is violated. The compile-time
// guard above is the real enforcement — it fires before this test can
// run — but a bare `const _ = uint(...)` explains nothing on its own, and
// this failure message does.
func TestDrainBudgetCoversTheWriteTimeout(t *testing.T) {
	require.GreaterOrEqual(t, drainBudget, audit.WriteTimeout,
		"a Stop deadline shorter than audit.WriteTimeout can expire while a healthy "+
			"insert is still in flight, so any test that Stops and then reads the row "+
			"back would be racing the write rather than testing it (mark8ly#804)")
}
