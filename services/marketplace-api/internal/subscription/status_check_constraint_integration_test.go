//go:build integration

package subscription_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// quotedLiteralRE matches single-quoted string literals inside a
// pg_get_constraintdef() rendering of a CHECK ((status)::text = ANY
// (ARRAY['a', 'b', ...]::text[])) constraint, e.g. 'past_due'.
var quotedLiteralRE = regexp.MustCompile(`'([^']*)'`)

// TestAllStatuses_MatchesDatabaseCheckConstraint is the test that makes
// subscription.AllStatuses() trustworthy.
//
// Go cannot enumerate the constants of a type at runtime, so nothing in the
// language can mechanically verify that AllStatuses() — a hand-written slice
// — actually lists every SubscriptionStatus declared in the const block
// above it. A twelfth status added to the const block alone compiles clean,
// every existing test still passes, and the value is silently unfilterable
// anywhere that derives from AllStatuses() (see
// platformadmin/billing_subscriptions.go's validSubscriptionStatuses).
//
// The one thing in this system that DOES mechanically enumerate "every
// valid status" is the database: store_subscriptions.status carries a CHECK
// constraint listing the complete accepted set, enforced by Postgres on
// every write. That constraint is the authoritative source of truth this
// test verifies AllStatuses() against, in both directions — a value the
// database accepts but Go doesn't know about, or a value Go claims but the
// database would reject, both fail this test.
func TestAllStatuses_MatchesDatabaseCheckConstraint(t *testing.T) {
	db := testdb.NewTx(t)

	var def string
	err := db.Raw(`
		SELECT pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE conrelid = 'store_subscriptions'::regclass
		  AND contype = 'c'
		  AND pg_get_constraintdef(oid) LIKE '%status%'
	`).Scan(&def).Error
	require.NoError(t, err)
	require.NotEmpty(t, def, "expected a CHECK constraint on store_subscriptions mentioning status")

	matches := quotedLiteralRE.FindAllStringSubmatch(def, -1)
	require.NotEmpty(t, matches, "could not parse any quoted values out of constraint definition: %s", def)

	dbStatuses := make([]string, 0, len(matches))
	for _, m := range matches {
		dbStatuses = append(dbStatuses, m[1])
	}

	goStatuses := make([]string, 0, len(subscription.AllStatuses()))
	for _, s := range subscription.AllStatuses() {
		goStatuses = append(goStatuses, string(s))
	}

	require.ElementsMatch(t, dbStatuses, goStatuses,
		"subscription.AllStatuses() must match the store_subscriptions.status CHECK constraint exactly; "+
			"constraint def was: %s", def)
}
