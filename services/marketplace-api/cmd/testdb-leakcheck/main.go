// Command testdb-leakcheck reports tables holding rows after an integration
// run, so a test that leaves state behind is caught at the moment it does so
// rather than as someone else's mysterious failure later.
//
// Usage:
//
//	TEST_DATABASE_URL=postgres://... testdb-leakcheck
//
// Exit 0 when the database is clean, 1 when it is not, 2 on a usage or
// connection error. `make test-int` runs it as its final step.
//
// # Why this exists
//
// pkg/testdb's NewDB truncates only the table list the caller passes it
// (testdb.go:60). A test that raw-INSERTs into a table it did not name leaves
// those rows behind for whatever package runs next in the same database, and
// nothing anywhere notices. #401 measured the result: a full run reporting
// ~46 failures that did not reproduce in isolation, with packages flipping
// between pass and fail purely on run order.
//
// Two concrete instances have since been found and fixed. #446's promo
// fixture seeded a hardcoded code into promo_codes — a table absent from its
// truncate list and, unlike its neighbours, carrying no ON DELETE CASCADE
// from stores — so the row committed permanently. That test passed exactly
// once, in August 2026, and its own success is what broke every run after.
// #436 was the same shape in the catalog: per-store sequences that TRUNCATE
// cannot reach, accumulating to 27,990 until pg_dump could no longer run.
//
// Both were found by accident. This makes the next one loud.
//
// # Why a command rather than a test
//
// The invariant is about the state left after EVERY package has run, so it
// cannot be a Go test: package test binaries are independent, and one that
// asserted an empty database would fail depending on which package happened
// to run before it.
//
// # Why not simply truncate everything in NewDB
//
// #401 suggests widening NewDB's truncation to all tenant-scoped tables. That
// would fix pollution and break something worse: this database is shared, and
// a fixture that truncates products and stores destroys a concurrent run's
// data mid-flight. Tests that need real commits should name their tables;
// tests that do not should use testdb.NewTx, whose rollback leaks nothing and
// is safe under concurrency. This command enforces the outcome without
// dictating which of the two a test picks.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// allowed are tables that legitimately hold rows in a clean database. Each
// value is why — an entry without one is not a decision, it is a silenced
// alarm, and this list is the only way to quiet the check.
//
// Migrations also INSERT into vendors, warehouses and shipments, but all of
// those are backfills (INSERT ... SELECT over existing rows) that match
// nothing on an empty database, so they are correctly absent here.
var allowed = map[string]string{
	"supported_countries":              "reference data seeded by migration 000030; global, not tenant-scoped, and no purge or fixture should remove it",
	"shipping_zones":                   "reference data seeded by migration; same class as supported_countries",
	"marketplace_db_schema_migrations": "golang-migrate's own bookkeeping — the record of which migrations have run",
}

func main() {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		fail("TEST_DATABASE_URL (or DATABASE_URL) is required")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fail("open: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dirty, err := nonEmptyTables(ctx, db)
	if err != nil {
		fail("scan: %v", err)
	}

	var leaked []string
	for table, n := range dirty {
		if _, ok := allowed[table]; !ok {
			leaked = append(leaked, fmt.Sprintf("%s (%d rows)", table, n))
		}
	}
	sort.Strings(leaked)

	if len(leaked) > 0 {
		fmt.Fprintf(os.Stderr, "testdb-leakcheck: %d table(s) hold rows after the run:\n", len(leaked))
		for _, l := range leaked {
			fmt.Fprintf(os.Stderr, "  %s\n", l)
		}
		fmt.Fprintf(os.Stderr, `
A test committed rows and did not clean them up. The next package to run
against this database inherits them, which is how #401's phantom failures
happened — packages passing or failing on run order alone.

Fix the fixture, not this list:
  - testdb.NewDB(t, ...): pass EVERY table the test writes, so the truncate
    reaches them. Watch for tables with no ON DELETE CASCADE from stores —
    those are not swept for you (that was #446).
  - testdb.NewTx(t): if the test does not need real commits, the rollback
    leaks nothing at all and is safe when runs overlap.

Add to the allowlist in cmd/testdb-leakcheck ONLY for data that SHOULD
survive, such as migration-seeded reference tables, and say why.
`)
		os.Exit(1)
	}

	fmt.Println("testdb-leakcheck: clean")
}

// nonEmptyTables returns an exact row count for every public table that has
// any, keyed by table name.
//
// Exact counts, deliberately, not pg_stat_user_tables.n_live_tup: that is an
// estimate maintained by autovacuum and it under-reports on a database this
// quiet. Measured while building this check — n_live_tup reported two
// non-empty tables where there were three, missing shipping_zones entirely.
// A leak detector that silently misses rows is worse than none.
func nonEmptyTables(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname,
		       (xpath('/row/c/text()',
		           query_to_xml(format('SELECT count(*) AS c FROM %I.%I', n.nspname, c.relname),
		                        false, true, '')))[1]::text::bigint AS n
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE c.relkind = 'r'
		   AND n.nspname = 'public'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int64{}
	for rows.Next() {
		var name string
		var n int64
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		if n > 0 {
			out[name] = n
		}
	}
	return out, rows.Err()
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "testdb-leakcheck: "+format+"\n", args...)
	os.Exit(2)
}
