//go:build integration

package consolepromo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/consolepromo"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// These tests are about the RE-SCOPE path, and that is deliberate.
//
// Covering first ingest alone would have passed against the bug (#795): the
// upsert assigns only the columns in upsertColumns, and allowed_plans and
// annual_only were absent from it, so a NEW row got the console's scope from
// the INSERT while an EXISTING row kept whatever it already held. An operator
// narrowing a live code got silence. Only publish -> ingest -> re-scope ->
// re-ingest -> assert the stored row CHANGED can tell the two apart.

var ingestedAt = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

// publish builds one console definition carrying a benefit, so
// promo_codes_has_benefit is satisfied and every assertion is about the scope.
func publish(code string, plans []string, annualOnly bool) consolepromo.Code {
	days := 14
	return consolepromo.Code{
		Code:               code,
		TrialExtensionDays: &days,
		AllowedPlans:       plans,
		AnnualOnly:         annualOnly,
	}
}

// ingest maps one definition and upserts it, i.e. does what one sync does to
// one code.
func ingest(t *testing.T, store consolepromo.Store, in consolepromo.Code) {
	t.Helper()
	row, err := consolepromo.MapCode(in, ingestedAt)
	require.NoError(t, err, "the definition must map")
	require.NoError(t, store.UpsertCodes(context.Background(),
		[]promo.PromoCode{row}), "the upsert must succeed")
}

// storedScope reads the two columns back as the DATABASE holds them.
// allowed_plans is read as ::text rather than through the model so an accidental
// '{}' cannot be mistaken for NULL — both read as an empty scope in Go.
func storedScope(t *testing.T, db *gorm.DB, code string) (plans *string, annualOnly bool) {
	t.Helper()
	var got struct {
		AllowedPlans *string
		AnnualOnly   bool
	}
	require.NoError(t, db.Raw(
		`SELECT allowed_plans::text AS allowed_plans, annual_only FROM promo_codes WHERE code = ?`,
		code,
	).Scan(&got).Error)
	return got.AllowedPlans, got.AnnualOnly
}

// TestUpsertCodes_RescopingAnIngestedCodeChangesTheRow is THE test for #795.
func TestUpsertCodes_RescopingAnIngestedCodeChangesTheRow(t *testing.T) {
	db := testdb.NewTx(t)
	store := consolepromo.NewStore(db)

	ingest(t, store, publish("RESCOPE50", []string{"pro"}, true))
	plans, annualOnly := storedScope(t, db, "RESCOPE50")
	require.NotNil(t, plans)
	require.Equal(t, "{pro}", *plans, "the first publication's scope must land")
	require.True(t, annualOnly)

	// The operator re-scopes the same code in the console and it publishes
	// again. The row already exists, so this is the upsert's UPDATE branch —
	// the branch that used to preserve both columns.
	ingest(t, store, publish("RESCOPE50", []string{"starter", "studio"}, false))

	plans, annualOnly = storedScope(t, db, "RESCOPE50")
	require.NotNil(t, plans)
	require.Equal(t, "{starter,studio}", *plans,
		"re-scoping an already-ingested code must overwrite allowed_plans, not preserve it")
	require.False(t, annualOnly,
		"clearing annual_only must land too; preserving it leaves the code annual-only forever")

	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM promo_codes WHERE code = ?`, "RESCOPE50").Scan(&n).Error)
	require.EqualValues(t, 1, n, "a re-sync must update in place, not duplicate the code")
}

// TestUpsertCodes_WideningAScopeBackToEveryPlanStoresNull is the other
// direction, and the one an "is it non-empty?" assertion would miss entirely:
// an operator who removes the restriction must get NULL back, not the old scope
// and not '{}'.
func TestUpsertCodes_WideningAScopeBackToEveryPlanStoresNull(t *testing.T) {
	db := testdb.NewTx(t)
	store := consolepromo.NewStore(db)

	ingest(t, store, publish("WIDENME50", []string{"pro"}, false))
	ingest(t, store, publish("WIDENME50", nil, false))

	plans, _ := storedScope(t, db, "WIDENME50")
	require.Nil(t, plans,
		"an unscoped publication must store NULL — '{}' means the same to the redeemer but reads as scoped")
}

// TestUpsertCodes_PreservesMark8lyOnlyPolicyColumns is decision 2 of the plan,
// and the guard on the change made for decision 1. max_per_email and
// min_effective_price_per_currency are abuse controls the console cannot
// express (#726); adding two console-owned columns to upsertColumns must not
// have quietly turned these into console-owned columns too.
func TestUpsertCodes_PreservesMark8lyOnlyPolicyColumns(t *testing.T) {
	db := testdb.NewTx(t)
	store := consolepromo.NewStore(db)

	ingest(t, store, publish("POLICY5050", []string{"pro"}, false))

	// mark8ly tightens its own controls on the ingested row.
	require.NoError(t, db.Exec(
		`UPDATE promo_codes
		    SET max_per_email = 5,
		        min_effective_price_per_currency = '{"usd": 1000}'::jsonb
		  WHERE code = ?`, "POLICY5050").Error)

	ingest(t, store, publish("POLICY5050", []string{"starter"}, true))

	var got struct {
		MaxPerEmail int
		MinPrice    string
	}
	require.NoError(t, db.Raw(
		`SELECT max_per_email, min_effective_price_per_currency::text AS min_price
		   FROM promo_codes WHERE code = ?`, "POLICY5050",
	).Scan(&got).Error)
	require.Equal(t, 5, got.MaxPerEmail,
		"max_per_email is mark8ly policy; a console publication must not reset it")
	require.JSONEq(t, `{"usd": 1000}`, got.MinPrice,
		"min_effective_price_per_currency is mark8ly policy; a console publication must not clear it")

	// …while the scope on the same row did move, so this test is not passing
	// merely because the second upsert did nothing at all.
	plans, annualOnly := storedScope(t, db, "POLICY5050")
	require.NotNil(t, plans)
	require.Equal(t, "{starter}", *plans)
	require.True(t, annualOnly)
}
