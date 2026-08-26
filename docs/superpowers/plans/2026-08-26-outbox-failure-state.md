# Outbox failure state (#336) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop the outbox publisher recording dropped events as successfully published, and start
persisting why a row was dropped, so a failed event is durable, terminal, and visible.

**Architecture:** Three derived states over the existing `outbox_events` columns — `pending`
(`published_at IS NULL AND error IS NULL`), `failed` (`published_at IS NULL AND error IS NOT
NULL`, terminal), `published` (`published_at IS NOT NULL`). The publisher marks a dropped row
failed instead of published, in the same transaction as the watermark bumps; the poll excludes
failed rows so they are not retried forever; `/admin/health` counts them separately from pending
and degrades on them. **No migration** — `outbox_events.error` already exists and is unused.

**Tech Stack:** Go 1.26, GORM, Gin, Postgres 15, `testify/require`, build-tagged integration tests
against a real Postgres via `pkg/testdb`.

**Spec:** `docs/superpowers/specs/2026-08-26-outbox-failure-state-design.md`

**Scope:** This plan is **#336 only** — the first of the spec's two PRs. #331
(`GET /admin/outbox`) is a separate plan on a separate branch, written after this ships and
deploys.

## Global Constraints

- Service root for every command: `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api`. Never run `go test` path-scoped for a full-suite run.
- Integration DSN — LAN IP, **never `localhost`**: `TEST_DATABASE_URL=postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable`
- Integration tests require `-tags=integration`. **`go test ./...` without the tag never compiles build-tagged files** and will report a false green (Trap 8).
- Use `-p 1` on full integration runs.
- `set -o pipefail` (or `${PIPESTATUS[0]}`) whenever a test exit code is reported as evidence — piping `go test` into `tail` reports *tail's* status, not the suite's (Trap 8 corollary).
- Module path: `github.com/mark8ly/marketplace-api`.
- **Do not push, open a PR, merge, or deploy.** This plan stops at local commits on the branch `fix/336-outbox-failure-state`. Deployment and the production verification exercise are handled separately, with explicit go-ahead at the moment of the write.
- Commit messages: conventional commits, **single line**, no signatures.
- `internal/outbox` is recorded at **2 FAIL** on `origin/main`. Do not fix them; do not accept them as pre-existing without the diff in Task 5.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `internal/outbox/models.go` | model + domain constants | **Modify** — add the two failure reason codes and the `Failure` type |
| `internal/outbox/repository.go` | data access for `outbox_events` | **Modify** — narrow the poll predicate; add `MarkFailedInTx` |
| `internal/outbox/publisher.go` | the polling loop and watermark bumps | **Modify** — build `ids` only from contributing rows; mark failures; rewrite the package doc comment |
| `internal/outbox/repository_integration_test.go` | repository integration tests | **Modify** — `MarkFailedInTx` and poll-exclusion tests |
| `internal/outbox/publisher_integration_test.go` | publisher integration tests | **Modify** — invert the test that enshrines the bug; add mixed-batch and unparseable-payload tests |
| `internal/handlers/platformadmin/health.go` | health contract and status rules | **Modify** — `OutboxHealth.Errored`, degrade rule, replace the now-false doc comment |
| `internal/handlers/platformadmin/health_checks.go` | the DB-backed health queries | **Modify** — split pending vs errored with `FILTER` |
| `internal/handlers/platformadmin/health_test.go` | health status unit tests | **Modify** — degrade-on-errored cases |
| `internal/handlers/platformadmin/health_checks_integration_test.go` | health query integration tests | **Modify** — errored rows excluded from pending and from the age |
| `internal/handlers/platformadmin/testdata/health_response.json` | the console's pinned golden response | **Modify** — the outbox entry gains `errored`; breaking this is the deliberate friction that says the contract changed |

---

## Task 1: Failure vocabulary and `MarkFailedInTx`

**Files:**
- Modify: `internal/outbox/models.go` (append after the `EventType` constants block)
- Modify: `internal/outbox/repository.go:10-21` (interface), and append the method
- Test: `internal/outbox/repository_integration_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `outbox.ReasonPayloadUnparseable` (`string` const, value `"payload_unparseable"`)
  - `outbox.ReasonPayloadMissingStoreID` (`string` const, value `"payload_missing_store_id"`)
  - `type outbox.Failure struct { ID string; Reason string }`
  - `Repository.MarkFailedInTx(tx *gorm.DB, failures []Failure) error` — used by Task 3.

- [ ] **Step 1: Write the failing test**

Append to `internal/outbox/repository_integration_test.go`:

```go
func TestIntegration_MarkFailedInTx_SetsReasonAndLeavesUnpublished(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	bad := makeEvent(tenantID)
	good := makeEvent(tenantID)
	enqueueCommitted(t, db, bad)
	enqueueCommitted(t, db, good)

	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: bad.ID, Reason: outbox.ReasonPayloadUnparseable},
		})
	})
	if err != nil {
		t.Fatalf("MarkFailedInTx: %v", err)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", bad.ID).Error; err != nil {
		t.Fatalf("reload failed row: %v", err)
	}
	if got.Error == nil {
		t.Fatalf("error is nil; want %q", outbox.ReasonPayloadUnparseable)
	}
	// Assert the EXACT code, not merely non-nil: a stub returns the zero
	// value for a field nobody set.
	if *got.Error != outbox.ReasonPayloadUnparseable {
		t.Fatalf("error = %q, want %q", *got.Error, outbox.ReasonPayloadUnparseable)
	}
	if got.PublishedAt != nil {
		t.Fatalf("a failed row must stay unpublished, got published_at=%v", got.PublishedAt)
	}

	var untouched outbox.OutboxEvent
	if err := db.First(&untouched, "id = ?", good.ID).Error; err != nil {
		t.Fatalf("reload untouched row: %v", err)
	}
	if untouched.Error != nil {
		t.Fatalf("unrelated row was marked failed: %q", *untouched.Error)
	}
}

func TestIntegration_MarkFailedInTx_GroupsDistinctReasons(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	unparseable := makeEvent(tenantID)
	missingStore := makeEvent(tenantID)
	enqueueCommitted(t, db, unparseable)
	enqueueCommitted(t, db, missingStore)

	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: unparseable.ID, Reason: outbox.ReasonPayloadUnparseable},
			{ID: missingStore.ID, Reason: outbox.ReasonPayloadMissingStoreID},
		})
	})
	if err != nil {
		t.Fatalf("MarkFailedInTx: %v", err)
	}

	for _, tc := range []struct {
		id   string
		want string
	}{
		{unparseable.ID, outbox.ReasonPayloadUnparseable},
		{missingStore.ID, outbox.ReasonPayloadMissingStoreID},
	} {
		var got outbox.OutboxEvent
		if err := db.First(&got, "id = ?", tc.id).Error; err != nil {
			t.Fatalf("reload %s: %v", tc.id, err)
		}
		if got.Error == nil || *got.Error != tc.want {
			t.Fatalf("row %s error = %v, want %q", tc.id, got.Error, tc.want)
		}
	}
}

func TestIntegration_MarkFailedInTx_EmptyIsNoOp(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, nil)
	})
	if err != nil {
		t.Fatalf("empty MarkFailedInTx must be a no-op, got: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -run 'TestIntegration_MarkFailedInTx' ./internal/outbox/ -v
```

Expected: **compile failure** — `undefined: outbox.Failure`, `undefined: outbox.ReasonPayloadUnparseable`, `repo.MarkFailedInTx undefined`.

- [ ] **Step 3: Add the vocabulary and the type**

Append to `internal/outbox/models.go`, after the `EventType` constants block and before
`IsOrderAggregate`:

```go
// Failure reason codes written to outbox_events.error when the publisher
// cannot process a row. This vocabulary is CLOSED and the values are
// STABLE: #331 serves this column cross-tenant to the platform console,
// and a stable code is what lets the console render it.
//
// A raw error string must NEVER be stored here. encoding/json quotes the
// offending input in its unmarshal errors, so persisting err.Error() would
// copy fragments of an arbitrary customer-data JSONB payload into a column
// that leaves this service — defeating the same reasoning that keeps
// `payload` out of #331's response, through a field nobody would audit.
const (
	ReasonPayloadUnparseable    = "payload_unparseable"
	ReasonPayloadMissingStoreID = "payload_missing_store_id"
)

// Failure is one row the publisher could not process, paired with the
// reason code to persist. See MarkFailedInTx.
type Failure struct {
	ID     string
	Reason string
}
```

- [ ] **Step 4: Add `MarkFailedInTx` to the interface**

In `internal/outbox/repository.go`, add to the `Repository` interface immediately below
`MarkPublishedInTx`:

```go
	// MarkFailedInTx records why the publisher could not process each row,
	// leaving published_at NULL. A row with error set is TERMINAL: the poll
	// in ProcessBatch excludes it, so it is never retried. Requeueing is an
	// operator action — clear error and the row re-enters the poll.
	//
	// Reason must be one of the Reason* constants in models.go, never a raw
	// error string. See the comment on those constants.
	MarkFailedInTx(tx *gorm.DB, failures []Failure) error
```

- [ ] **Step 5: Implement it**

Append to `internal/outbox/repository.go`:

```go
func (r *gormRepository) MarkFailedInTx(tx *gorm.DB, failures []Failure) error {
	if len(failures) == 0 {
		return nil
	}
	// Grouped by reason so this is one statement per DISTINCT reason (two,
	// today) rather than one per row. Map iteration order is unspecified
	// and irrelevant: the id sets are disjoint, so the statements commute.
	byReason := make(map[string][]string, len(failures))
	for _, f := range failures {
		byReason[f.Reason] = append(byReason[f.Reason], f.ID)
	}
	for reason, ids := range byReason {
		if err := tx.Exec(`UPDATE outbox_events SET error = ? WHERE id IN ?`,
			reason, ids).Error; err != nil {
			return fmt.Errorf("outbox: mark failed: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -run 'TestIntegration_MarkFailedInTx' ./internal/outbox/ -v
```

Expected: PASS, 3 tests.

- [ ] **Step 7: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/outbox/models.go \
        services/marketplace-api/internal/outbox/repository.go \
        services/marketplace-api/internal/outbox/repository_integration_test.go
git commit -m "feat(outbox): persist a closed-vocabulary failure reason on a dropped event"
```

---

## Task 2: The poll excludes failed rows

Without this, a failed row left at `published_at IS NULL` is re-selected every 2s forever — a
poison pill that sorts to the head of `ORDER BY tenant_id, created_at` and eventually consumes the
whole batch window. This is what makes "terminal" true rather than aspirational.

**Files:**
- Modify: `internal/outbox/repository.go:41-51` (the `SELECT` inside `ProcessBatch`)
- Test: `internal/outbox/repository_integration_test.go`

**Interfaces:**
- Consumes: `outbox.ReasonPayloadUnparseable`, `outbox.Failure`, `Repository.MarkFailedInTx` from Task 1.
- Produces: no new symbols. Changes the behaviour of `ProcessBatch`.

- [ ] **Step 1: Write the failing test**

Append to `internal/outbox/repository_integration_test.go`:

```go
// A row with error set is TERMINAL. This is the poison-pill proof: nothing
// else in the suite exercises the poll's error IS NULL term, which exists
// precisely so a permanently-failing row cannot be re-selected forever and
// starve real events out of the batch window.
func TestIntegration_ProcessBatch_SkipsFailedRows(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	failed := makeEvent(tenantID)
	fresh := makeEvent(tenantID)
	enqueueCommitted(t, db, failed)
	enqueueCommitted(t, db, fresh)

	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: failed.ID, Reason: outbox.ReasonPayloadUnparseable},
		})
	}); err != nil {
		t.Fatalf("seed failed row: %v", err)
	}

	var seen []string
	count, err := repo.ProcessBatch(context.Background(), 10,
		func(tx *gorm.DB, rows []outbox.OutboxEvent) error {
			for _, r := range rows {
				seen = append(seen, r.ID)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if count != 1 {
		t.Fatalf("ProcessBatch saw %d rows, want 1 (the failed row must be skipped)", count)
	}
	if len(seen) != 1 || seen[0] != fresh.ID {
		t.Fatalf("ProcessBatch saw %v, want only the un-failed row %s", seen, fresh.ID)
	}
}

// Clearing error is the documented requeue path for an operator. It must
// actually work, or "terminal" means "lost".
func TestIntegration_ProcessBatch_ClearingErrorRequeues(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: evt.ID, Reason: outbox.ReasonPayloadMissingStoreID},
		})
	}); err != nil {
		t.Fatalf("seed failed row: %v", err)
	}

	if err := db.Exec(`UPDATE outbox_events SET error = NULL WHERE id = ?`, evt.ID).Error; err != nil {
		t.Fatalf("clear error: %v", err)
	}

	count, err := repo.ProcessBatch(context.Background(), 10,
		func(tx *gorm.DB, rows []outbox.OutboxEvent) error { return nil })
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if count != 1 {
		t.Fatalf("ProcessBatch saw %d rows, want 1 after error was cleared", count)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -run 'TestIntegration_ProcessBatch_(SkipsFailedRows|ClearingErrorRequeues)' ./internal/outbox/ -v
```

Expected: `SkipsFailedRows` **FAILS** with `ProcessBatch saw 2 rows, want 1`.
`ClearingErrorRequeues` passes already — it is a regression guard for the requeue path, and it must
keep passing after Step 3.

- [ ] **Step 3: Narrow the poll predicate**

In `internal/outbox/repository.go`, inside `ProcessBatch`, replace the `WHERE` line of the raw
`SELECT`:

```go
			WHERE published_at IS NULL
```

with:

```go
			WHERE published_at IS NULL AND error IS NULL
```

and replace the `ProcessBatch` doc comment on the `Repository` interface
(`internal/outbox/repository.go:13-20`) with:

```go
	// ProcessBatch opens its own transaction, locks up to `limit` PENDING
	// rows via FOR UPDATE SKIP LOCKED, and calls fn with the rows and the
	// same tx. If fn returns nil the tx commits (the caller is expected to
	// have called MarkPublishedInTx inside fn); if fn returns an error the
	// tx rolls back and the rows become visible to the next poll. Returns
	// the number of rows the callback saw.
	//
	// PENDING means published_at IS NULL *and* error IS NULL. A row with
	// error set is terminal and is never re-selected — see MarkFailedInTx.
	// The partial index outbox_unpublished_idx (migration 000001) is on
	// published_at IS NULL; the error term is a filter on top of it, which
	// is fine while failed rows are ~0. If they ever become common, that
	// index is the thing to revisit.
	ProcessBatch(ctx context.Context, limit int,
		fn func(tx *gorm.DB, rows []OutboxEvent) error) (int, error)
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -run 'TestIntegration_' ./internal/outbox/ -v
```

Expected: both new tests PASS, and every pre-existing `internal/outbox` integration test still
behaves as it did (note: `TestIntegration_Publisher_MissingStoreID_DropFloor` still passes here —
Task 3 is what inverts it).

- [ ] **Step 5: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/outbox/repository.go \
        services/marketplace-api/internal/outbox/repository_integration_test.go
git commit -m "fix(outbox): exclude terminally-failed rows from the publisher poll"
```

---

## Task 3: The publisher stops marking dropped events published

This is #336's core. Note Step 1 **inverts an existing test that currently enshrines the bug** —
`TestIntegration_Publisher_MissingStoreID_DropFloor` asserts *"expected malformed event to still be
marked published"*.

**Files:**
- Modify: `internal/outbox/publisher.go:1-19` (package doc), `:13-19` (type doc), `:85-145` (`Tick`)
- Test: `internal/outbox/publisher_integration_test.go:168-217` (invert), plus two new tests

**Interfaces:**
- Consumes: `outbox.Failure`, `outbox.ReasonPayloadUnparseable`, `outbox.ReasonPayloadMissingStoreID`, `Repository.MarkFailedInTx` (Task 1); the narrowed poll (Task 2).
- Produces: no new symbols. Changes `Publisher.Tick` behaviour.

- [ ] **Step 1: Invert the test that enshrines the bug**

In `internal/outbox/publisher_integration_test.go`, in
`TestIntegration_Publisher_MissingStoreID_DropFloor`, replace this block:

```go
	if got.PublishedAt == nil {
		t.Fatalf("expected malformed event to still be marked published")
	}
```

with:

```go
	if got.PublishedAt != nil {
		t.Fatalf("a dropped event must NOT be marked published, got published_at=%v", got.PublishedAt)
	}
	if got.Error == nil {
		t.Fatalf("error is nil; want %q", outbox.ReasonPayloadMissingStoreID)
	}
	if *got.Error != outbox.ReasonPayloadMissingStoreID {
		t.Fatalf("error = %q, want %q", *got.Error, outbox.ReasonPayloadMissingStoreID)
	}
```

and rename the test to state the new property:

```go
func TestIntegration_Publisher_MissingStoreID_MarksFailedNotPublished(t *testing.T) {
```

- [ ] **Step 2: Add the unparseable-payload and mixed-batch tests**

Append to `internal/outbox/publisher_integration_test.go`:

```go
func TestIntegration_Publisher_UnparseablePayload_MarksFailedNotPublished(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	insertStore(t, db, storeID, tenantID)

	// jsonb rejects malformed JSON at insert, so the unparseable-to-Go
	// value is a well-formed JSON scalar: valid jsonb, but not the object
	// json.Unmarshal into map[string]any requires.
	evt := &outbox.OutboxEvent{
		TenantID:    tenantID,
		Aggregate:   outbox.AggregateProduct,
		AggregateID: uuid.NewString(),
		EventType:   outbox.EventProductCreated,
		Payload:     datatypes.JSON([]byte(`"not-an-object"`)),
	}
	if err := db.Create(evt).Error; err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	pub := outbox.New(outbox.Config{
		Repo: repo, DB: db, Logger: quietLogger(),
		Interval: 1 * time.Second, BatchSize: 100,
	})
	if _, err := pub.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", evt.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.PublishedAt != nil {
		t.Fatalf("a dropped event must NOT be marked published, got published_at=%v", got.PublishedAt)
	}
	if got.Error == nil || *got.Error != outbox.ReasonPayloadUnparseable {
		t.Fatalf("error = %v, want %q", got.Error, outbox.ReasonPayloadUnparseable)
	}
}

// The mixed batch is the composition no single-purpose test constructs, and
// it is the whole point of the fix: one good row and one bad row in the SAME
// tick. The good row must publish AND bump its watermark; the bad row must
// be failed AND left unpublished. A regression that reverted the id-building
// inversion would still pass every single-row test in this file.
func TestIntegration_Publisher_MixedBatch_PublishesGoodFailsBad(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	insertStore(t, db, storeID, tenantID)

	good := enqueueForStore(t, db, tenantID, storeID)

	bad := &outbox.OutboxEvent{
		TenantID:    tenantID,
		Aggregate:   outbox.AggregateProduct,
		AggregateID: uuid.NewString(),
		EventType:   outbox.EventProductUpdated,
		Payload:     datatypes.JSON([]byte(`{"unrelated":"value"}`)),
	}
	if err := db.Create(bad).Error; err != nil {
		t.Fatalf("enqueue bad: %v", err)
	}

	pub := outbox.New(outbox.Config{
		Repo: repo, DB: db, Logger: quietLogger(),
		Interval: 1 * time.Second, BatchSize: 100,
	})
	count, err := pub.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if count != 2 {
		t.Fatalf("tick saw %d rows, want 2", count)
	}

	var gotGood outbox.OutboxEvent
	if err := db.First(&gotGood, "id = ?", good.ID).Error; err != nil {
		t.Fatalf("reload good: %v", err)
	}
	if gotGood.PublishedAt == nil {
		t.Fatalf("the good event must be published even when a bad one shares its batch")
	}
	if gotGood.Error != nil {
		t.Fatalf("the good event must not be marked failed, got %q", *gotGood.Error)
	}

	var gotBad outbox.OutboxEvent
	if err := db.First(&gotBad, "id = ?", bad.ID).Error; err != nil {
		t.Fatalf("reload bad: %v", err)
	}
	if gotBad.PublishedAt != nil {
		t.Fatalf("the bad event must NOT be published, got published_at=%v", gotBad.PublishedAt)
	}
	if gotBad.Error == nil || *gotBad.Error != outbox.ReasonPayloadMissingStoreID {
		t.Fatalf("bad event error = %v, want %q", gotBad.Error, outbox.ReasonPayloadMissingStoreID)
	}

	// The good row's watermark landed: the drop must not have suppressed it.
	var n int64
	if err := db.Raw(`SELECT count(*) FROM store_watermarks WHERE store_id = ?`, storeID).
		Scan(&n).Error; err != nil {
		t.Fatalf("count watermarks: %v", err)
	}
	if n != 1 {
		t.Fatalf("watermark rows = %d, want 1", n)
	}

	// And the failed row is not retried on the next tick.
	count, err = pub.Tick(context.Background())
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if count != 0 {
		t.Fatalf("second tick saw %d rows, want 0 (failed rows are terminal)", count)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -run 'TestIntegration_Publisher_(MissingStoreID_MarksFailedNotPublished|UnparseablePayload_MarksFailedNotPublished|MixedBatch_PublishesGoodFailsBad)' ./internal/outbox/ -v
```

Expected: all three **FAIL** with `a dropped event must NOT be marked published, got
published_at=…`.

- [ ] **Step 4: Fix `Tick`**

In `internal/outbox/publisher.go`, inside the `Tick` callback, replace the loop body's opening and
the two drop branches so that `ids` is appended **after** both checks, and collect failures.
Replace this:

```go
		byBucket := map[key]time.Time{}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
			var payload map[string]any
			if err := json.Unmarshal(r.Payload, &payload); err != nil {
				if p.logger != nil {
					p.logger.Warn("outbox publisher: unparseable payload; dropping",
						"event_id", r.ID, "err", err)
				}
				continue
			}
			sid, _ := payload["store_id"].(string)
			if sid == "" {
				if p.logger != nil {
					p.logger.Warn("outbox publisher: payload missing store_id; dropping",
						"event_id", r.ID, "event_type", r.EventType)
				}
				continue
			}
			axis := "products"
```

with this:

```go
		byBucket := map[key]time.Time{}
		ids := make([]string, 0, len(rows))
		failures := make([]Failure, 0)
		for _, r := range rows {
			var payload map[string]any
			if err := json.Unmarshal(r.Payload, &payload); err != nil {
				if p.logger != nil {
					p.logger.Warn("outbox publisher: unparseable payload; failing",
						"event_id", r.ID, "err", err)
				}
				failures = append(failures, Failure{ID: r.ID, Reason: ReasonPayloadUnparseable})
				continue
			}
			sid, _ := payload["store_id"].(string)
			if sid == "" {
				if p.logger != nil {
					p.logger.Warn("outbox publisher: payload missing store_id; failing",
						"event_id", r.ID, "event_type", r.EventType)
				}
				failures = append(failures, Failure{ID: r.ID, Reason: ReasonPayloadMissingStoreID})
				continue
			}
			// Appended only now: a row reaches this line exactly when it is
			// going to contribute a watermark bump. Appending before the
			// checks above is what made a dropped event indistinguishable
			// from a published one (#336).
			ids = append(ids, r.ID)
			axis := "products"
```

Then replace the final line of the callback:

```go
		return p.repo.MarkPublishedInTx(tx, ids)
```

with:

```go
		// Both marks run in the SAME transaction as the watermark bumps
		// above, so a batch's outcome commits whole or not at all.
		if err := p.repo.MarkFailedInTx(tx, failures); err != nil {
			return err
		}
		return p.repo.MarkPublishedInTx(tx, ids)
```

- [ ] **Step 5: Rewrite the doc comments that now assert something false**

Replace `internal/outbox/publisher.go:13-19` (the `Publisher` type doc):

```go
// Publisher polls outbox_events and bumps store_watermarks asynchronously.
// See spec §14.1 (watermark separation) and §14.6 (publisher semantics).
//
// Payload invariant: every outbox row in slice 1 carries a "store_id" key
// at the top level of its JSON payload. A row without it, or with a payload
// that will not unmarshal, is logged and marked FAILED — error is set, and
// published_at is left NULL. It never blocks the publisher (the original
// reason for dropping it) and it is never retried (both causes are
// deterministic properties of the row), but it is no longer recorded as a
// successful publish. See #336; the failed state is served by #331.
```

And replace `internal/outbox/models.go:1-4` (the package doc), which describes only the two-state
world:

```go
// Package outbox holds the OutboxEvent model. Events are written in the
// same transaction as the mutation that produces them (see spec §13.2.7).
// Slice 1's publisher reads these rows, upserts store_watermarks, and marks
// them published. Slice 2 adds real Pub/Sub delivery.
//
// A row is in one of three states, all derived from existing columns:
//
//	pending    published_at IS NULL AND error IS NULL
//	failed     published_at IS NULL AND error IS NOT NULL   (terminal)
//	published  published_at IS NOT NULL
//
// failed is terminal: ProcessBatch's poll excludes it, so it is never
// retried. Clearing error requeues the row. See #336.
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/outbox/ -v
```

Expected: PASS for the whole `internal/outbox` package, including the previously-passing
watermark and shutdown tests.

- [ ] **Step 7: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/outbox/publisher.go \
        services/marketplace-api/internal/outbox/models.go \
        services/marketplace-api/internal/outbox/publisher_integration_test.go
git commit -m "fix(outbox): mark a dropped event failed instead of published"
```

---

## Task 4: `/admin/health` separates pending from errored, and degrades on errored

Without this, the first failed row makes `/admin/health` permanently degraded on
`oldest_pending_age_seconds` — a false alarm shipped as a bug fix.

**Files:**
- Modify: `internal/handlers/platformadmin/health.go:62-70` (type + doc), `:151-162` (status rule)
- Modify: `internal/handlers/platformadmin/health_checks.go:29-48` (the query)
- Test: `internal/handlers/platformadmin/health_test.go`, `internal/handlers/platformadmin/health_checks_integration_test.go`

**Interfaces:**
- Consumes: the `error` column now being written (Tasks 1 and 3).
- Produces: `platformadmin.OutboxHealth.Errored int64`, and an `"errored"` key in the `outbox` dependency's `metrics` map.

- [ ] **Step 1: Write the failing integration test**

Append to `internal/handlers/platformadmin/health_checks_integration_test.go`:

```go
// A terminally-failed row is NOT pending. If it counted as pending, the
// first one would put /admin/health into a degraded state that never
// clears, because oldest_pending_age_seconds would grow forever.
func TestOutboxHealthExcludesFailedRowsFromPendingAndAge(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	src := platformadmin.NewDBHealthSource(db)
	tenant := uuid.NewString()

	// Failed and very old: must not count as pending, and must not drive
	// the age.
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at, error)
		VALUES (?, 'product', ?, 'product.created', '{}'::jsonb, ?, 'payload_unparseable')`,
		tenant, uuid.NewString(), healthAsOf.Add(-72*time.Hour)).Error)

	// Genuinely pending, 5 minutes old.
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at)
		VALUES (?, 'product', ?, 'product.created', '{}'::jsonb, ?)`,
		tenant, uuid.NewString(), healthAsOf.Add(-5*time.Minute)).Error)

	got, err := src.Outbox(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Pending, "a failed row must not count as pending")
	require.Equal(t, int64(1), got.Errored, "the failed row must be counted as errored")
	require.Equal(t, int64(300), got.OldestPendingAgeSeconds,
		"age must ignore failed rows, or the alarm never clears")
}

// Errored counts only unpublished failures. A published row is settled
// whatever its error column happens to hold.
func TestOutboxHealthErroredIsZeroWhenNoFailedRows(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	src := platformadmin.NewDBHealthSource(db)
	tenant := uuid.NewString()

	published := healthAsOf.Add(-time.Hour)
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at, published_at)
		VALUES (?, 'product', ?, 'product.created', '{}'::jsonb, ?, ?)`,
		tenant, uuid.NewString(), published, published).Error)

	got, err := src.Outbox(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got.Pending)
	require.Equal(t, int64(0), got.Errored)
	require.Equal(t, int64(0), got.OldestPendingAgeSeconds)
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -run 'TestOutboxHealth' ./internal/handlers/platformadmin/ -v
```

Expected: **compile failure** — `got.Errored undefined (type platformadmin.OutboxHealth has no field or method Errored)`.

- [ ] **Step 3: Add the field and replace the false doc comment**

In `internal/handlers/platformadmin/health.go`, replace the `OutboxHealth` block at `:62-70`:

```go
// OutboxHealth is the measured state of outbox_events.
//
// Pending counts rows the publisher will still attempt: published_at IS
// NULL AND error IS NULL. Errored counts rows it has given up on:
// published_at IS NULL AND error IS NOT NULL, which #336 made a real state
// by teaching the publisher to write outbox_events.error instead of marking
// a dropped event published.
//
// The two are separate because they need different reactions. A pending
// backlog drains on its own and is measured by age; an errored row never
// drains — only an operator clears it — so counting it as pending would
// make OldestPendingAgeSeconds grow forever and leave this surface
// permanently degraded on a condition draining cannot fix.
type OutboxHealth struct {
	Pending                 int64
	OldestPendingAgeSeconds int64
	Errored                 int64
}
```

- [ ] **Step 4: Split the query**

In `internal/handlers/platformadmin/health_checks.go`, replace the body of the `Outbox` query —
keep the outer `WHERE published_at IS NULL` so the partial index still applies, and split with
`FILTER`:

```go
	// The published_at condition stays a WHERE, not a FILTER: outbox_events
	// is never pruned, and only a WHERE lets Postgres use
	// outbox_unpublished_idx (a partial index on published_at IS NULL,
	// migration 000001) instead of scanning every row ever written on a
	// shared db-f1-micro. The pending/errored split is done with FILTER
	// *inside* that index-selected set, so the split costs nothing.
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE error IS NULL)                       AS pending,
			COALESCE(EXTRACT(EPOCH FROM (
				? - MIN(created_at) FILTER (WHERE error IS NULL)))::bigint, 0)
			                                                            AS oldest_pending_age_seconds,
			COUNT(*) FILTER (WHERE error IS NOT NULL)                   AS errored
		FROM outbox_events
		WHERE published_at IS NULL`, asOf).Scan(&out).Error
```

- [ ] **Step 5: Run the integration tests to verify they pass**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -run 'TestOutboxHealth' ./internal/handlers/platformadmin/ -v
```

Expected: PASS, including the pre-existing
`TestOutboxHealthCountsPendingAndMeasuresAgeFromAsOf`.

- [ ] **Step 6: Write the failing unit tests for the degrade rule**

Append to `internal/handlers/platformadmin/health_test.go`. This file already provides
`stubHealthSource` (a struct with an `outbox platformadmin.OutboxHealth` field, used as a pointer),
`healthFixture()`, and `getHealth(t, src) (*httptest.ResponseRecorder, healthBody)` — use those, do
not add duplicates. `outbox` is index 0 of `body.Data.Dependencies` because the registry order is
contract-pinned by `TestHealthReportsEveryRegistryEntryInOrder`. Status comes back as a plain JSON
string, so compare against the literals `"degraded"` / `"ok"`, matching the golden fixture.

```go
// An errored row degrades regardless of age. This alarm does not clear by
// draining — only by an operator resolving the row — which is the correct
// shape for a condition that requires a human, and the same shape
// csv_import_jobs already uses for RunningStaleHeartbeat.
func TestOutboxDegradesOnErroredEvenWhenNothingIsPending(t *testing.T) {
	_, body := getHealth(t, &stubHealthSource{
		outbox: platformadmin.OutboxHealth{Pending: 0, OldestPendingAgeSeconds: 0, Errored: 1},
	})
	dep := body.Data.Dependencies[0]
	require.Equal(t, "outbox", dep.Name)
	require.Equal(t, "degraded", dep.Status,
		"a terminally-failed event must degrade even with an empty pending queue")
	require.Equal(t, int64(1), dep.Metrics["errored"])
	require.Equal(t, int64(0), dep.Metrics["pending"])
}

func TestOutboxIsOKWhenNothingErroredAndBacklogIsYoung(t *testing.T) {
	_, body := getHealth(t, &stubHealthSource{
		outbox: platformadmin.OutboxHealth{
			Pending:                 3,
			OldestPendingAgeSeconds: int64(platformadmin.OutboxPendingThreshold/time.Second) - 1,
			Errored:                 0,
		},
	})
	dep := body.Data.Dependencies[0]
	require.Equal(t, "outbox", dep.Name)
	require.Equal(t, "ok", dep.Status)
	require.Equal(t, int64(0), dep.Metrics["errored"])
}
```

- [ ] **Step 7: Run it to verify it fails**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
go test -run 'TestOutbox(DegradesOnErrored|IsOKWhenNothingErrored)' ./internal/handlers/platformadmin/ -v
```

Expected: `TestOutboxDegradesOnErroredEvenWhenNothingIsPending` FAILS — status is `"ok"` and
`dep.Metrics["errored"]` is absent (zero value).

- [ ] **Step 8: Add the degrade rule and the metric**

In `internal/handlers/platformadmin/health.go`, replace the outbox block in `health()`
(`:151-162`):

```go
	if v, err := h.src.Outbox(ctx, asOf); err != nil {
		h.logCheckFailed("outbox", err)
	} else {
		status := StatusOK
		// Errored degrades regardless of age: a terminally-failed event is
		// a silent divergence between this service and whatever consumes
		// the watermark, and it does not resolve on its own.
		if v.Errored > 0 ||
			time.Duration(v.OldestPendingAgeSeconds)*time.Second >= OutboxPendingThreshold {
			status = StatusDegraded
		}
		measured["outbox"] = dependencyRow{Status: status, Metrics: map[string]int64{
			"pending":                    v.Pending,
			"oldest_pending_age_seconds": v.OldestPendingAgeSeconds,
			"errored":                    v.Errored,
		}}
	}
```

- [ ] **Step 9: Update the shared fixture and the golden response**

Adding a key to the `outbox` metrics map **breaks the golden fixture on purpose** — that file is
the console's pinned contract, and `TestHealthReportsEveryRegistryEntryInOrder`'s comment calls
this friction deliberate. Two edits, made together.

First, `healthFixture()` in `internal/handlers/platformadmin/health_test.go:50`. That file's own
convention (`health_test.go:19-25`) is that every asserted value is **distinct and non-zero**, so a
missing map key cannot masquerade as a passing zero. `Errored` must therefore be non-zero and
unlike any other number in the fixture. `outbox` is already `degraded` (400 ≥ 300), so this does
not change any status:

```go
		outbox:   platformadmin.OutboxHealth{Pending: 7, OldestPendingAgeSeconds: 400, Errored: 3},
```

Then `internal/handlers/platformadmin/testdata/health_response.json` — the outbox entry only:

```json
      { "name": "outbox", "status": "degraded",
        "metrics": { "pending": 7, "oldest_pending_age_seconds": 400, "errored": 3 } },
```

- [ ] **Step 10: Run the whole package's unit tests**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
go test ./internal/handlers/platformadmin/ 2>&1 | tail -40; echo "exit=${PIPESTATUS[0]}"
```

Expected: PASS, `exit=0`. If the golden-fixture test fails here, the JSON in Step 9 does not match
what the handler now emits — fix the fixture to match the handler, **never** the handler to match
the fixture.

- [ ] **Step 11: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/handlers/platformadmin/health.go \
        services/marketplace-api/internal/handlers/platformadmin/health_checks.go \
        services/marketplace-api/internal/handlers/platformadmin/health_test.go \
        services/marketplace-api/internal/handlers/platformadmin/health_checks_integration_test.go \
        services/marketplace-api/internal/handlers/platformadmin/testdata/health_response.json
git commit -m "feat(health): count terminally-failed outbox events and degrade on them"
```

---

## Task 5: Whole-branch verification and the pre-existing-failure diff

The point of this task is to find out whether this branch **added** a failure, which cannot be
answered by looking at this branch alone.

**Files:** none modified — this task produces evidence.

- [ ] **Step 1: Verify build-tagged files actually compile**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
set -o pipefail
go vet -tags=integration ./... 2>&1 | tail -20; echo "exit=${PIPESTATUS[0]}"
```

Expected: `exit=0`. `go vet -tags=integration` is the only thing that compiles build-tagged files.

- [ ] **Step 2: Capture this branch's failing set**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 ./... > /tmp/branch-tests.txt 2>&1
echo "go test exit=$?"
grep -E '^FAIL' /tmp/branch-tests.txt | sort > /tmp/branch-fails.txt
wc -l < /tmp/branch-fails.txt
```

No pipe on the `go test` line: its exit code is being reported as evidence, and piping would report
the *last* command's status instead. A non-zero exit here is expected — the point of Steps 3 and 4
is finding out whether it is the *same* non-zero as `origin/main`.

- [ ] **Step 3: Capture `origin/main`'s failing set in a throwaway worktree**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git worktree add /tmp/m8-main-baseline origin/main
cd /tmp/m8-main-baseline/services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 ./... > /tmp/main-tests.txt 2>&1
echo "go test exit=$?"
grep -E '^FAIL' /tmp/main-tests.txt | sort > /tmp/main-fails.txt
wc -l < /tmp/main-fails.txt
```

Note `> file 2>&1`, not `2>&1 > file` — the latter sends stderr to the terminal and only stdout to
the file, which would silently drop half the output this comparison depends on.

- [ ] **Step 4: Diff the two failing sets**

```bash
diff /tmp/main-fails.txt /tmp/branch-fails.txt && echo "IDENTICAL — no failure added"
```

Expected: identical, or the only difference is a package that this branch **fixed**. **A package
failing on the branch but not on `origin/main` is a defect this work introduced** — stop and fix it
before proceeding. Do not report "pre-existing" without this diff; that claim has been wrong twice
in this milestone.

- [ ] **Step 5: Clean up the worktree**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git worktree remove /tmp/m8-main-baseline --force
git worktree list
```

- [ ] **Step 6: Record the evidence**

Append the two failing-set counts and the diff result to the plan's own file as a short
"Verification record" section, then commit:

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add docs/superpowers/plans/2026-08-26-outbox-failure-state.md
git commit -m "docs: record the origin/main failing-set diff for the outbox failure state branch"
```

---

## Verification record (Task 5, 2026-08-26)

**`go vet -tags=integration ./...`**: `exit=0` (clean — build-tagged files compile).

**Branch (`fix/336-outbox-failure-state`)** — full integration suite, `TEST_DATABASE_URL` LAN
Postgres, `-tags=integration -p 1`:
- `go test` exit=1
- Failing packages: **22** (`/tmp/branch-fails-pkgs.txt`)
- Failing individual tests: **191** (`/tmp/branch-fails-tests.txt`)

**`origin/main`** (5413e20f, throwaway worktree at `/tmp/m8-main-baseline`) — identical command:
- `go test` exit=1
- Failing packages: **23** (`/tmp/main-fails-pkgs.txt`)
- Failing individual tests: **194** (`/tmp/main-fails-tests.txt`)

**Both-directions package diff:**
- Only on `origin/main`, not on branch (fixed by this branch): `internal/outbox` — the one package
  this plan targeted.
- Only on branch, not on `origin/main` (would be a regression): **none**.

**Both-directions individual-test diff** (within the 22 packages that fail on both):
- Only on `origin/main`, not on branch: `TestIntegration_Publisher_BumpsWatermark`,
  `TestIntegration_Publisher_BatchMultipleEvents_MaxCreatedAt`,
  `TestIntegration_Publisher_MissingStoreID_DropFloor` — the three tests broken by the
  `insertStore` helper's missing `stores.storefront_customer_portal_secret` column, fixed in Task
  3. The third was also renamed on the branch to
  `TestIntegration_Publisher_MissingStoreID_MarksFailedNotPublished`, which now passes (does not
  appear in either failing-test list).
- Only on branch, not on `origin/main`: **none**.

**Verdict:** This branch added zero test failures. It strictly reduced the pre-existing failing set
by fixing `internal/outbox` (23 → 22 failing packages, 194 → 191 failing tests), all via the
authorised `insertStore` test-helper fix from Task 3. The remaining 22 failing packages / 191
failing tests are identical, by package and by individual test name, between `origin/main` and this
branch — pre-existing failures unrelated to this work, unmodified by it.

---

## Not in this plan

- **`GET /admin/outbox` (#331).** Separate branch, separate plan, written after this ships. The
  spec's §6 explains why they are not bundled.
- **The production provoke-and-clean exercise** (spec §7). It runs after deploy, with explicit
  go-ahead at the moment of the write. It is not a step an implementing agent performs.
- **`metrics.OutboxEventsPending` / `OutboxEventsPublishedTotal`**, registered at
  `internal/metrics/registry.go:165-166` and written by nothing. Same dead-declaration family as
  `outbox_events.error` was. To be filed as its own issue — see spec §8.
- **Producer-side validation** that would reject a malformed row at enqueue. A different defence at
  a different layer, and it cannot help rows already in the table.
