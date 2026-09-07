//go:build integration

package promo_test

import (
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// These tests pin how allowed_plans crosses the Go/Postgres boundary (#795).
//
// The column is text[]. Declaring the field with GORM's `serializer:json`
// made every write of a non-empty scope fail outright:
//
//	ERROR: malformed array literal: "["pro","studio"]" (SQLSTATE 22P02)
//
// GORM marshalled the slice to a JSON document and handed Postgres a text
// literal, which is not array syntax. It never surfaced because nothing had
// ever written a scope — the column has only held NULL.
//
// The assertion that matters is the STORED REPRESENTATION, not that the write
// returned no error: a future change that reintroduces a serializer would pass
// a bare require.NoError just as happily. Only reading allowed_plans::text back
// and demanding `{pro,studio}` pins the behaviour that was broken.

// TestPromoCodes_AllowedPlansStoresAsPostgresArray is the regression guard.
func TestPromoCodes_AllowedPlansStoresAsPostgresArray(t *testing.T) {
	db := testdb.NewTx(t)

	// promo_codes_has_benefit refuses a row that neither discounts nor
	// extends, so the fixture must carry a benefit to reach the column
	// under test at all.
	pc := &promo.PromoCode{
		Code:               "SCOPEDPLANS1",
		TrialExtensionDays: ptr(14),
		AllowedPlans:       pq.StringArray{"pro", "studio"},
	}
	require.NoError(t, insertPromoCode(db, pc),
		"a plan-scoped code must insert; a JSON serializer makes this fail with 22P02")

	var stored string
	require.NoError(t, db.Raw(
		`SELECT allowed_plans::text FROM promo_codes WHERE id = ?`, pc.ID,
	).Scan(&stored).Error)
	require.Equal(t, "{pro,studio}", stored,
		"the column must hold a Postgres array literal, not a JSON document")

	var got promo.PromoCode
	require.NoError(t, db.Where("id = ?", pc.ID).First(&got).Error)
	require.Equal(t, pq.StringArray{"pro", "studio"}, got.AllowedPlans,
		"the scope must round-trip through GORM in order")
}

// TestPromoCodes_UnscopedCodeStoresNull pins the other half of the contract the
// redeemer depends on. validator.go guards on len(AllowedPlans) > 0, so NULL
// and '{}' are the same fact to it — but only one of them is written, and an
// accidental '{}' would read as "scoped" to anything inspecting the column.
func TestPromoCodes_UnscopedCodeStoresNull(t *testing.T) {
	db := testdb.NewTx(t)

	pc := &promo.PromoCode{Code: "UNSCOPEDCODE", TrialExtensionDays: ptr(7)}
	require.NoError(t, insertPromoCode(db, pc))

	var isNull bool
	require.NoError(t, db.Raw(
		`SELECT allowed_plans IS NULL FROM promo_codes WHERE id = ?`, pc.ID,
	).Scan(&isNull).Error)
	require.True(t, isNull, "an unscoped code must store NULL, never '{}'")

	var got promo.PromoCode
	require.NoError(t, db.Where("id = ?", pc.ID).First(&got).Error)
	require.Empty(t, got.AllowedPlans, "NULL must read back as an empty scope")
}
