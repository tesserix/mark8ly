//go:build integration

package order_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestNextDocumentNumber_HappyPath(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewTx(t)
	storeID := uuid.New()
	day := time.Now()

	for i := 1; i <= 5; i++ {
		n, err := order.NextDocumentNumber(ctx, db, storeID, "order", day)
		require.NoError(t, err)
		require.Equal(t, i, n, "sequence should advance monotonically")
	}
}

func TestNextDocumentNumber_KindIsolated(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewTx(t)
	storeID := uuid.New()
	day := time.Now()

	o1, err := order.NextDocumentNumber(ctx, db, storeID, "order", day)
	require.NoError(t, err)
	r1, err := order.NextDocumentNumber(ctx, db, storeID, "return", day)
	require.NoError(t, err)
	o2, err := order.NextDocumentNumber(ctx, db, storeID, "order", day)
	require.NoError(t, err)

	require.Equal(t, 1, o1, "order starts at 1")
	require.Equal(t, 1, r1, "return starts at 1 independently of order")
	require.Equal(t, 2, o2, "order continues independently of return")
}

func TestNextDocumentNumber_DayRollover(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewTx(t)
	storeID := uuid.New()
	day1 := time.Now()
	day2 := day1.AddDate(0, 0, 1)

	a, err := order.NextDocumentNumber(ctx, db, storeID, "order", day1)
	require.NoError(t, err)
	b, err := order.NextDocumentNumber(ctx, db, storeID, "order", day2)
	require.NoError(t, err)

	require.Equal(t, 1, a)
	require.Equal(t, 1, b, "new day resets sequence to 1")
}

func TestNextDocumentNumber_StoreIsolated(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewTx(t)
	store1 := uuid.New()
	store2 := uuid.New()
	day := time.Now()

	a, _ := order.NextDocumentNumber(ctx, db, store1, "order", day)
	b, _ := order.NextDocumentNumber(ctx, db, store2, "order", day)
	c, _ := order.NextDocumentNumber(ctx, db, store1, "order", day)

	require.Equal(t, 1, a)
	require.Equal(t, 1, b, "store2 starts independently")
	require.Equal(t, 2, c, "store1 continues independently")
}

func TestNextDocumentNumber_InvalidKindRejected(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewTx(t)

	_, err := order.NextDocumentNumber(ctx, db, uuid.New(), "invoice", time.Now())
	require.Error(t, err, "only 'order' and 'return' kinds are accepted")
}

func TestNextDocumentNumber_NilTxRejected(t *testing.T) {
	// Defensive check: calling without a transaction handle is a programmer error.
	_, err := order.NextDocumentNumber(context.Background(), (*gorm.DB)(nil), uuid.New(), "order", time.Now())
	require.Error(t, err)
}
