//go:build integration

package order_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestNextDocumentNumber_Concurrent_NoDuplicates_And_P99Gate is the M1 EXIT GATE.
//
// It spawns 50 concurrent goroutines, each running 20 full-create-transaction
// cycles that use NextDocumentNumber, insert an orders row, 2 order_items,
// shipping + billing addresses, and an initial order_events row. Every returned
// sequence number must be unique and the p99 latency of the full create-tx
// must stay under 50ms.
//
// If this test fails or the p99 exceeds the gate, DO NOT fix the test — the
// sequencing strategy needs to be reworked per §11 of the spec (per-store
// Postgres sequence or Redis counter). Any pivot must be surfaced to a human.
//
// Uses testdb.NewDB (not NewTx) because the concurrent workers need real
// commits — a single enclosing transaction would serialize them. The cleanup
// TRUNCATE removes all rows from the tables listed after the test completes.
func TestNextDocumentNumber_Concurrent_NoDuplicates_And_P99Gate(t *testing.T) {
	const (
		goroutines = 50
		perG       = 20
		p99Gate    = 50 * time.Millisecond
	)

	ctx := context.Background()
	db := testdb.NewDB(t,
		"document_number_seq",
		"order_events",
		"order_addresses",
		"order_items",
		"orders",
	)
	storeID := uuid.New()
	tenantID := uuid.New()
	day := time.Now()

	type result struct {
		seq     int
		latency time.Duration
		err     error
	}
	ch := make(chan result, goroutines*perG)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				start := time.Now()
				var seq int
				err := db.Transaction(func(tx *gorm.DB) error {
					var err error
					seq, err = order.NextDocumentNumber(ctx, tx, storeID, "order", day)
					if err != nil {
						return err
					}
					o := order.Order{
						TenantID:       tenantID,
						StoreID:        storeID,
						OrderNumber:    fmt.Sprintf("M-BCH-%06d-%04d", workerID, seq),
						IdempotencyKey: fmt.Sprintf("bench-%d-%d-%s", workerID, i, uuid.NewString()),
						CustomerEmail:  "bench@example.com",
						Subtotal:       decimal.NewFromInt(100),
						GrandTotal:     decimal.NewFromInt(100),
						CurrencyCode:   "EUR",
						PlacedAt:       time.Now(),
					}
					if err := tx.Create(&o).Error; err != nil {
						return err
					}
					for j := 0; j < 2; j++ {
						if err := tx.Create(&order.OrderItem{
							OrderID:       o.ID,
							TitleSnapshot: "x",
							SKUSnapshot:   "x",
							UnitPrice:     decimal.NewFromInt(50),
							Quantity:      1,
							LineTotal:     decimal.NewFromInt(50),
							CurrencyCode:  "EUR",
						}).Error; err != nil {
							return err
						}
					}
					for _, kind := range []string{"shipping", "billing"} {
						if err := tx.Create(&order.OrderAddress{
							OrderID:     o.ID,
							Kind:        kind,
							Name:        "A",
							Line1:       "1",
							City:        "C",
							CountryCode: "IE",
						}).Error; err != nil {
							return err
						}
					}
					return tx.Create(&order.OrderEvent{
						OrderID: o.ID,
						Kind:    "status_changed",
						Payload: datatypes.JSON([]byte(`{"to":"pending"}`)),
					}).Error
				})
				ch <- result{seq: seq, latency: time.Since(start), err: err}
			}
		}(g)
	}

	wg.Wait()
	close(ch)

	seen := make(map[int]bool, goroutines*perG)
	latencies := make([]float64, 0, goroutines*perG)
	for r := range ch {
		require.NoError(t, r.err)
		require.False(t, seen[r.seq], "duplicate sequence: %d", r.seq)
		seen[r.seq] = true
		latencies = append(latencies, float64(r.latency))
	}
	require.Len(t, seen, goroutines*perG, "expected every sequence number to be unique")

	sort.Float64s(latencies)
	p50 := time.Duration(latencies[len(latencies)/2])
	p90 := time.Duration(latencies[int(float64(len(latencies))*0.90)])
	p99 := time.Duration(latencies[int(float64(len(latencies))*0.99)])
	t.Logf("p50=%v p90=%v p99=%v (gate=<%v)", p50, p90, p99, p99Gate)

	require.Less(t, p99, p99Gate,
		"M1 EXIT GATE FAILED: p99 create-tx latency %v exceeds %v. "+
			"Do NOT fix the test — rework the sequencing strategy per spec §11 risks.", p99, p99Gate)
}
