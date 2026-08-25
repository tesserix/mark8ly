package audit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// No build tag deliberately: this test needs no database, so it should run
// under plain `go test ./...` as well as `-tags=integration`, not just the
// latter (#365).

// TestOperatorRetentionIsSevenYears pins the retention WINDOW, not merely
// the mechanism. Every other test in operator_prune_integration_test.go
// derives its cutoff from OperatorRetentionYears, so all of them stay green
// if the constant changes — they pin the behaviour relative to the value,
// never the value itself. Seven years is a compliance decision matching
// billing_archive's "retained 7 years after hard-delete under
// legal-obligation basis" (migration 000046:24, §23.2), and shortening it
// destroys governance records irreversibly. Changing it should require
// changing a test that says so out loud (#365).
func TestOperatorRetentionIsSevenYears(t *testing.T) {
	require.Equal(t, 7, OperatorRetentionYears,
		"operator audit retention is a compliance window, not a tunable: it matches billing_archive (§23.2)")
}
