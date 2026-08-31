//go:build integration

package stockhold_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #231. These are integration tests by necessity, not preference: the
// property that matters is what happens when two connections contend for the
// same unit, and that is invisible to a single-transaction test.

const testLocation = "00000000-0000-0000-0000-000000000001"

// seedVariant creates the minimum product/variant/stock rows a hold needs.
func seedVariant(t *testing.T, db *gorm.DB, qty int) string {
	t.Helper()
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone,
		                     storefront_customer_portal_secret)
		 VALUES (?, ?, 'Stock Test', ?, 'active', 'AU', 'AUD', 'Australia/Sydney', ?)`,
		storeID, tenantID, "stock-"+uuid.NewString()[:8], uuid.NewString()).Error)

	productID := uuid.NewString()
	require.NoError(t, db.Exec(
		// status='active' requires published_at (products_published_requires_active).
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'Stock Test Product', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "stock-"+uuid.NewString()[:8], uuid.NewString()).Error)

	variantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 10.00, 'AUD')`,
		variantID, productID, storeID, "SKU-"+uuid.NewString()[:8]).Error)

	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, ?, now())`, variantID, testLocation, qty).Error)
	return variantID
}

func TestHold_ReducesAvailabilityAndRefusesWhenShort(t *testing.T) {
	db := testdb.NewDB(t, "stock_holds")
	repo := stockhold.NewRepository()
	ctx := context.Background()
	variantID := seedVariant(t, db, 3)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, uuid.NewString(), variantID, testLocation, 2, time.Minute)
	}))

	// 1 unit left; asking for 2 must fail rather than oversell.
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, uuid.NewString(), variantID, testLocation, 2, time.Minute)
	})
	require.True(t, errors.Is(err, stockhold.ErrInsufficientStock), "got %v", err)

	// The remaining 1 is still obtainable.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, uuid.NewString(), variantID, testLocation, 1, time.Minute)
	}))
}

// A repeat Hold for the same cart must REFRESH, not stack a second
// reservation — otherwise a customer revisiting checkout consumes their own
// stock twice and locks themselves out.
func TestHold_IsIdempotentPerCartAndRefreshesExpiry(t *testing.T) {
	db := testdb.NewDB(t, "stock_holds")
	repo := stockhold.NewRepository()
	ctx := context.Background()
	variantID := seedVariant(t, db, 1)
	cart := uuid.NewString()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, cart, variantID, testLocation, 1, time.Second)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, cart, variantID, testLocation, 1, time.Hour)
	}))

	var rows int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM stock_holds WHERE cart_token = ?`, cart).Scan(&rows).Error)
	require.Equal(t, int64(1), rows, "a repeat hold must refresh the existing row, not add one")

	var expires time.Time
	require.NoError(t, db.Raw(`SELECT expires_at FROM stock_holds WHERE cart_token = ?`, cart).Scan(&expires).Error)
	require.True(t, expires.After(time.Now().Add(30*time.Minute)), "expiry must be pushed out")
}

// The clock, not the sweeper, is what frees stock. Nothing is running here.
func TestHold_ExpiredHoldDoesNotReduceAvailability(t *testing.T) {
	db := testdb.NewDB(t, "stock_holds")
	repo := stockhold.NewRepository()
	ctx := context.Background()
	variantID := seedVariant(t, db, 1)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, uuid.NewString(), variantID, testLocation, 1, -time.Minute)
	}))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, uuid.NewString(), variantID, testLocation, 1, time.Minute)
	}), "an expired hold must not reserve anything, with no sweeper running")
}

func TestCommit_DecrementsStockAndFlipsHolds(t *testing.T) {
	db := testdb.NewDB(t, "stock_holds")
	repo := stockhold.NewRepository()
	ctx := context.Background()
	variantID := seedVariant(t, db, 5)
	cart := uuid.NewString()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, cart, variantID, testLocation, 2, time.Minute)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Commit(ctx, tx, cart)
	}))

	var qty int
	require.NoError(t, db.Raw(`SELECT quantity FROM variant_stock WHERE variant_id = ?`, variantID).Scan(&qty).Error)
	require.Equal(t, 3, qty, "a committed sale must decrement the stock row")

	var state string
	require.NoError(t, db.Raw(`SELECT state FROM stock_holds WHERE cart_token = ?`, cart).Scan(&state).Error)
	require.Equal(t, "committed", state)

	// The committed hold must no longer reduce availability — the stock row
	// already reflects it, so counting both would double-charge the variant.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, uuid.NewString(), variantID, testLocation, 3, time.Minute)
	}))
}

func TestRelease_ReturnsStockToThePool(t *testing.T) {
	db := testdb.NewDB(t, "stock_holds")
	repo := stockhold.NewRepository()
	ctx := context.Background()
	variantID := seedVariant(t, db, 1)
	cart := uuid.NewString()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, cart, variantID, testLocation, 1, time.Hour)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Release(ctx, tx, cart)
	}))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, uuid.NewString(), variantID, testLocation, 1, time.Minute)
	}), "a released hold must return its unit to the pool")
}

// second location a store's stock holds may also sit at, for tests exercising
// more than one (variant, location) pair for the same cart.
const testLocation2 = "00000000-0000-0000-0000-000000000002"

// ReleaseExcept must release every hold NOT in keep, and leave the ones in
// keep untouched — the shape checkout needs before Commit runs, since
// Commit decrements every live hold the cart owns.
func TestReleaseExcept_ReleasesEverythingNotKept(t *testing.T) {
	db := testdb.NewDB(t, "stock_holds")
	repo := stockhold.NewRepository()
	ctx := context.Background()
	variantID := seedVariant(t, db, 5)
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 5, now())`, variantID, testLocation2).Error)
	cart := uuid.NewString()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := repo.Hold(ctx, tx, cart, variantID, testLocation, 1, time.Hour); err != nil {
			return err
		}
		return repo.Hold(ctx, tx, cart, variantID, testLocation2, 1, time.Hour)
	}))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.ReleaseExcept(ctx, tx, cart, []stockhold.VariantLocation{
			{VariantID: variantID, LocationID: testLocation2},
		})
	}))

	var states []string
	require.NoError(t, db.Raw(
		`SELECT state FROM stock_holds WHERE cart_token = ? ORDER BY location_id`, cart).
		Scan(&states).Error)
	require.Equal(t, []string{"released", "held"}, states,
		"testLocation sorts before testLocation2 — the kept hold must stay 'held', the other must be released")

	// The released unit at testLocation must be obtainable by someone else.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, uuid.NewString(), variantID, testLocation, 1, time.Minute)
	}))
}

// An empty keep list must behave exactly like Release — release everything.
func TestReleaseExcept_EmptyKeepReleasesEverything(t *testing.T) {
	db := testdb.NewDB(t, "stock_holds")
	repo := stockhold.NewRepository()
	ctx := context.Background()
	variantID := seedVariant(t, db, 1)
	cart := uuid.NewString()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, cart, variantID, testLocation, 1, time.Hour)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.ReleaseExcept(ctx, tx, cart, nil)
	}))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, uuid.NewString(), variantID, testLocation, 1, time.Minute)
	}), "an empty keep list must release everything, same as Release")
}

// THE TEST THIS PACKAGE EXISTS FOR.
//
// N goroutines on separate connections contend for M units. Exactly M may
// win. This is the oversell #230 reports, and it cannot be reproduced with
// testdb.NewTx — a single transaction cannot see another connection's
// uncommitted rows, so the contention never happens.
func TestHold_ConcurrentContendersCannotOversell(t *testing.T) {
	db := testdb.NewDB(t, "stock_holds")
	repo := stockhold.NewRepository()
	ctx := context.Background()

	const units, contenders = 3, 12
	variantID := seedVariant(t, db, units)

	var wg sync.WaitGroup
	results := make(chan error, contenders)
	start := make(chan struct{})
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- db.Transaction(func(tx *gorm.DB) error {
				return repo.Hold(ctx, tx, uuid.NewString(), variantID, testLocation, 1, time.Minute)
			})
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var ok, short, other int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, stockhold.ErrInsufficientStock):
			short++
		default:
			other++
			t.Logf("unexpected error: %v", err)
		}
	}
	require.Equal(t, 0, other, "no contender may fail for an unexpected reason")
	require.Equal(t, units, ok, "exactly the available units may be held — more is an oversell")
	require.Equal(t, contenders-units, short)
}

// The sweeper deletes dead rows only. It must never touch a live hold, and
// availability must not change when it runs.
func TestSweep_DeletesOnlyLongExpiredHolds(t *testing.T) {
	db := testdb.NewDB(t, "stock_holds")
	repo := stockhold.NewRepository()
	ctx := context.Background()
	variantID := seedVariant(t, db, 10)

	live := uuid.NewString()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, live, variantID, testLocation, 1, time.Hour)
	}))
	recentlyExpired := uuid.NewString()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, recentlyExpired, variantID, testLocation, 1, -time.Minute)
	}))
	longExpired := uuid.NewString()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return repo.Hold(ctx, tx, longExpired, variantID, testLocation, 1, -2*time.Hour)
	}))

	n, err := repo.Sweep(ctx, db, 100)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "only the hold expired beyond the grace window is dead")

	require.Equal(t, int64(1), countHolds(t, db, live))
	require.Equal(t, int64(1), countHolds(t, db, recentlyExpired),
		"a recently expired hold is kept: it is evidence of what happened, and it "+
			"already reduces availability by nothing")
	require.Equal(t, int64(0), countHolds(t, db, longExpired))
}

func countHolds(t *testing.T, db *gorm.DB, cart string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM stock_holds WHERE cart_token = ?`, cart).Scan(&n).Error)
	return n
}
