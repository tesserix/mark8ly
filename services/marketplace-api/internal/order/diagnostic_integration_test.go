//go:build integration

// Diagnostic tests for M1 Task 12 exit gate failure. These tests isolate
// individual latency contributors so we can decide whether the sequencing
// strategy needs rework, or whether the bottleneck is elsewhere.
//
// DELETE THIS FILE once the diagnosis is resolved and the gate passes.

package order_test

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

type percentiles struct{ p50, p90, p99, max time.Duration }

func percentilesFrom(latencies []float64) percentiles {
	sort.Float64s(latencies)
	return percentiles{
		p50: time.Duration(latencies[len(latencies)/2]),
		p90: time.Duration(latencies[int(float64(len(latencies))*0.90)]),
		p99: time.Duration(latencies[int(float64(len(latencies))*0.99)]),
		max: time.Duration(latencies[len(latencies)-1]),
	}
}

func logPercentiles(t *testing.T, name string, latencies []float64) {
	t.Helper()
	p := percentilesFrom(latencies)
	t.Logf("%s: p50=%v p90=%v p99=%v max=%v (n=%d)", name, p.p50, p.p90, p.p99, p.max, len(latencies))
}

// D1: baseline single-connection latency for NextDocumentNumber alone.
// Tells us the raw cost of one atomic upsert round-trip without any contention.
func TestDiagnostic_D1_SequenceAlone_SingleGoroutine(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t, "document_number_seq")
	storeID := uuid.New()
	day := time.Now()

	const ops = 100
	latencies := make([]float64, 0, ops)
	for i := 0; i < ops; i++ {
		start := time.Now()
		err := db.Transaction(func(tx *gorm.DB) error {
			_, err := order.NextDocumentNumber(ctx, tx, storeID, "order", day)
			return err
		})
		require.NoError(t, err)
		latencies = append(latencies, float64(time.Since(start)))
	}
	logPercentiles(t, "D1 seq-only single-goroutine", latencies)
}

// D2: concurrent sequence-only (no surrounding order graph).
// If this stays fast, the atomic upsert is not the bottleneck and we should
// NOT switch to Redis/per-store sequences. If this matches the full-create
// p99, sequence contention is real.
func TestDiagnostic_D2_SequenceAlone_50Concurrent(t *testing.T) {
	const goroutines = 50
	const perG = 20

	ctx := context.Background()
	db := testdb.NewDB(t, "document_number_seq")
	storeID := uuid.New()
	day := time.Now()

	ch := make(chan time.Duration, goroutines*perG)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				start := time.Now()
				_ = db.Transaction(func(tx *gorm.DB) error {
					_, err := order.NextDocumentNumber(ctx, tx, storeID, "order", day)
					return err
				})
				ch <- time.Since(start)
			}
		}()
	}
	wg.Wait()
	close(ch)

	latencies := make([]float64, 0, goroutines*perG)
	for d := range ch {
		latencies = append(latencies, float64(d))
	}
	logPercentiles(t, "D2 seq-only 50-concurrent", latencies)
}

// D3: bare single INSERT latency (no sequence, no multi-statement tx).
// This measures the raw network + fsync cost per INSERT round-trip to Docker
// Postgres. If this is already ~10ms on a dev machine, 6 INSERTs per tx is
// ~60ms best-case, explaining the full-create benchmark without ANY sequence
// contention.
func TestDiagnostic_D3_BareInsert_50Concurrent(t *testing.T) {
	const goroutines = 50
	const perG = 20

	// Create a throwaway table for this diagnostic
	db := testdb.NewDB(t)
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS diag_insert_test (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			payload text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		)
	`).Error)
	t.Cleanup(func() {
		db.Exec("DROP TABLE IF EXISTS diag_insert_test")
	})

	ch := make(chan time.Duration, goroutines*perG)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				start := time.Now()
				_ = db.Transaction(func(tx *gorm.DB) error {
					return tx.Exec(
						"INSERT INTO diag_insert_test (payload) VALUES (?)",
						fmt.Sprintf("worker=%d i=%d", id, i),
					).Error
				})
				ch <- time.Since(start)
			}
		}(g)
	}
	wg.Wait()
	close(ch)

	latencies := make([]float64, 0, goroutines*perG)
	for d := range ch {
		latencies = append(latencies, float64(d))
	}
	logPercentiles(t, "D3 bare-INSERT 50-concurrent", latencies)
}

// D4: pool stats and connection count at the end.
// If max open is > 50, pool is not the limit. If it's pegged at some lower
// number, we have a bottleneck via database/sql defaults.
func TestDiagnostic_D4_PoolStats(t *testing.T) {
	db := testdb.NewDB(t)
	// Hit it with some load first so stats are non-zero
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				var one int
				db.Raw("SELECT 1").Scan(&one)
			}
		}()
	}
	wg.Wait()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	stats := sqlDB.Stats()
	t.Logf("pool stats: max_open=%d open=%d in_use=%d idle=%d wait_count=%d wait_duration=%v",
		stats.MaxOpenConnections, stats.OpenConnections, stats.InUse, stats.Idle,
		stats.WaitCount, stats.WaitDuration)

	// Also print Postgres-side view
	var pgConnInfo []struct {
		PID    int
		State  sql.NullString
		Query  string
	}
	db.Raw(`
		SELECT pid, state, query
		FROM pg_stat_activity
		WHERE datname = current_database()
		  AND pid <> pg_backend_pid()
	`).Scan(&pgConnInfo)
	t.Logf("postgres-side: %d connections", len(pgConnInfo))
}
