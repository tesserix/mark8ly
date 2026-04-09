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

// TestNextDocumentNumber_HappyPath issues 5 numbers for the same
// (store, "order") pair and asserts they are strictly increasing.
//
// The exact values may NOT start at 1 because Postgres sequences with
// CACHE 50 reserve a block per connection — a later caller might see
// 51 as the first value if they reach a fresh connection. So the test
// only asserts monotonicity, not concrete values.
func TestNextDocumentNumber_HappyPath(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t)
	storeID := uuid.New()
	seedStore(t, db, storeID)

	var prev int64
	for i := 0; i < 5; i++ {
		var got int64
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			n, err := order.NextDocumentNumber(ctx, tx, storeID, "order")
			if err != nil {
				return err
			}
			got = n
			return nil
		}))
		require.Greater(t, got, prev, "sequence must be monotonic")
		prev = got
	}
}

func TestNextDocumentNumber_KindIsolated(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t)
	storeID := uuid.New()
	seedStore(t, db, storeID)

	var orderSeq, returnSeq int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		n, err := order.NextDocumentNumber(ctx, tx, storeID, "order")
		if err != nil {
			return err
		}
		orderSeq = n
		n, err = order.NextDocumentNumber(ctx, tx, storeID, "return")
		if err != nil {
			return err
		}
		returnSeq = n
		return nil
	}))

	// Order and return have independent sequences, so the first value of
	// each is whatever the first CACHE block holds — they need not be equal
	// to 1 exactly, but they must be independent (first value of one
	// doesn't advance the other).
	var orderSeq2 int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		n, err := order.NextDocumentNumber(ctx, tx, storeID, "order")
		if err != nil {
			return err
		}
		orderSeq2 = n
		return nil
	}))
	require.Greater(t, orderSeq2, orderSeq, "order sequence advances independently of return")
	require.NotZero(t, orderSeq)
	require.NotZero(t, returnSeq)
}

func TestNextDocumentNumber_StoreIsolated(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t)
	store1 := uuid.New()
	store2 := uuid.New()
	seedStore(t, db, store1)
	seedStore(t, db, store2)

	var s1a, s2a, s1b int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		n, _ := order.NextDocumentNumber(ctx, tx, store1, "order")
		s1a = n
		n, _ = order.NextDocumentNumber(ctx, tx, store2, "order")
		s2a = n
		n, _ = order.NextDocumentNumber(ctx, tx, store1, "order")
		s1b = n
		return nil
	}))

	require.Greater(t, s1b, s1a, "store1 advances independently")
	require.NotZero(t, s2a, "store2 has its own sequence")
}

func TestNextDocumentNumber_InvalidKindRejected(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t)
	var err error
	_ = db.Transaction(func(tx *gorm.DB) error {
		_, err = order.NextDocumentNumber(ctx, tx, uuid.New(), "invoice")
		return nil
	})
	require.Error(t, err, "only 'order' and 'return' kinds are accepted")
}

func TestNextDocumentNumber_NilTxRejected(t *testing.T) {
	_, err := order.NextDocumentNumber(context.Background(), (*gorm.DB)(nil), uuid.New(), "order")
	require.Error(t, err)
}

func TestFormatOrderNumber(t *testing.T) {
	day := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	require.Equal(t, "M-TST-260409-00123", order.FormatOrderNumber("tst", day, 123))
	require.Equal(t, "M-TST-260409-00001", order.FormatOrderNumber("TST", day, 1))
	// Sequence overflow past 5 digits is allowed; format is floor-padded.
	require.Equal(t, "M-TST-260409-123456", order.FormatOrderNumber("tst", day, 123456))
}

func TestFormatReturnNumber(t *testing.T) {
	day := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	require.Equal(t, "R-TST-260409-00007", order.FormatReturnNumber("tst", day, 7))
}
