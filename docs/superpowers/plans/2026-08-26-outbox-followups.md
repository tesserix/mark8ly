# Outbox follow-ups (#374, #375, #376) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the three follow-ups found during #336's whole-branch review — the last poison-pill
cause, the dead outbox metrics, and a comment that contradicts its own file.

**Architecture:** #336 established that a deterministic, permanent property of an outbox row becomes
a terminal `failed` state carrying a reason from a closed vocabulary. #374 extends that shape to a
third cause (a `store_id` with no matching store) by checking store existence *before* the watermark
upsert, rather than letting an FK violation abort the whole transaction. #375 wires the publish/fail
counters and deletes a redundant gauge. #376 is a comment.

**Tech Stack:** Go 1.26, GORM, Postgres 15, Prometheus client, build-tagged integration tests via
`pkg/testdb`.

**Spec:** `docs/superpowers/specs/2026-08-26-outbox-failure-state-design.md` — this plan extends its
§2 state model with a fourth reason code. The three-state model itself is unchanged.

## Global Constraints

- Service root for every command: `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api`
- Integration DSN — LAN IP, **never `localhost`**: `TEST_DATABASE_URL=postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable`
- Integration tests require `-tags=integration` and `-count=1`. **Without the tag, build-tagged files are never compiled and the run is a false green.**
- **Run whole packages, never a `-run`-scoped subset, for final GREEN evidence.**
- Never pipe the measured `go test` — capture with `> file 2>&1` and report `$?` separately.
- Module path: `github.com/mark8ly/marketplace-api`.
- Commit messages: conventional commits, **single line**, no signatures.
- **Do not push, open a PR, merge, or deploy.** Local commits on `fix/374-outbox-followups` only.
- Branch base: `main` at `0fc2763f` (the #336 merge).

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `internal/outbox/models.go` | model + vocabulary | **Modify** — add `ReasonStoreNotFound` |
| `internal/outbox/repository.go` | data access | **Modify** — add the new code to `sanitizeReason`'s allowlist |
| `internal/outbox/publisher.go` | polling loop | **Modify** — track ids per store; store-existence pre-check; counters; document `Tick`'s return |
| `internal/outbox/publisher_integration_test.go` | publisher tests | **Modify** — FK-row tests |
| `internal/outbox/repository_integration_test.go` | repository tests | **Modify** — allowlist guard for the new code |
| `internal/metrics/registry.go` | Prometheus definitions | **Modify** — delete the gauge, add a failed counter |
| `cmd/marketplace-api/main.go` | wiring | **Modify** — one comment (#376) |
| `docs/superpowers/specs/2026-08-26-outbox-failure-state-design.md` | the design doc | **Modify** — document the fourth reason code |

---

## Task 1: #374 — store-existence pre-check

**Files:**
- Modify: `internal/outbox/models.go`, `internal/outbox/repository.go`, `internal/outbox/publisher.go`
- Modify: `docs/superpowers/specs/2026-08-26-outbox-failure-state-design.md`
- Test: `internal/outbox/publisher_integration_test.go`, `internal/outbox/repository_integration_test.go`

**Interfaces:**
- Consumes: `Failure`, `MarkFailedInTx`, `sanitizeReason` (all from #336).
- Produces: `outbox.ReasonStoreNotFound` (`string` const, `"store_not_found"`).

**The trap in this task.** `sanitizeReason` (`repository.go`) coerces anything outside its allowlist
to `ReasonUnknown`. **Adding the constant without adding it to that switch means every store-not-found
row silently records `"unknown"`** — and the tests below are written to catch exactly that.

- [ ] **Step 1: Write the failing tests**

Append to `internal/outbox/publisher_integration_test.go`:

```go
// A store_id that is well-formed but absent from `stores` used to raise an FK
// violation on the watermark upsert, which ABORTS the whole Postgres
// transaction — taking the good rows and the failure marks with it and
// leaving the entire batch pending forever. #374.
func TestIntegration_Publisher_StoreNotFound_MarksFailedAndCommits(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	ghostStoreID := uuid.NewString() // deliberately never inserted

	evt := enqueueForStore(t, db, tenantID, ghostStoreID)

	pub := outbox.New(outbox.Config{
		Repo: repo, DB: db, Logger: quietLogger(),
		Interval: 1 * time.Second, BatchSize: 100,
	})
	if _, err := pub.Tick(context.Background()); err != nil {
		t.Fatalf("tick must not error on a missing store: %v", err)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", evt.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.PublishedAt != nil {
		t.Fatalf("a row for a missing store must not be published, got %v", got.PublishedAt)
	}
	if got.Error == nil {
		t.Fatalf("error is nil; want %q", outbox.ReasonStoreNotFound)
	}
	// Exact code. If the constant was added but not allowlisted in
	// sanitizeReason, this reads "unknown" and this line is what catches it.
	if *got.Error != outbox.ReasonStoreNotFound {
		t.Fatalf("error = %q, want %q", *got.Error, outbox.ReasonStoreNotFound)
	}

	var n int64
	if err := db.Raw(`SELECT count(*) FROM store_watermarks`).Scan(&n).Error; err != nil {
		t.Fatalf("count watermarks: %v", err)
	}
	if n != 0 {
		t.Fatalf("watermark rows = %d, want 0", n)
	}
}

// The composition that matters: one good row and one ghost-store row in the
// SAME batch. Before #374 the FK violation rolled back BOTH.
func TestIntegration_Publisher_StoreNotFound_DoesNotRollBackGoodRows(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	realStoreID := uuid.NewString()
	insertStore(t, db, realStoreID, tenantID)
	ghostStoreID := uuid.NewString()

	good := enqueueForStore(t, db, tenantID, realStoreID)
	ghost := enqueueForStore(t, db, tenantID, ghostStoreID)

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
		t.Fatalf("the good row must publish even when a ghost-store row shares its batch")
	}
	if gotGood.Error != nil {
		t.Fatalf("the good row must not be failed, got %q", *gotGood.Error)
	}

	var gotGhost outbox.OutboxEvent
	if err := db.First(&gotGhost, "id = ?", ghost.ID).Error; err != nil {
		t.Fatalf("reload ghost: %v", err)
	}
	if gotGhost.PublishedAt != nil {
		t.Fatalf("the ghost row must not be published, got %v", gotGhost.PublishedAt)
	}
	if gotGhost.Error == nil || *gotGhost.Error != outbox.ReasonStoreNotFound {
		t.Fatalf("ghost row error = %v, want %q", gotGhost.Error, outbox.ReasonStoreNotFound)
	}

	// The real store's watermark landed — the FK row did not suppress it.
	var n int64
	if err := db.Raw(`SELECT count(*) FROM store_watermarks WHERE store_id = ?`, realStoreID).
		Scan(&n).Error; err != nil {
		t.Fatalf("count watermarks: %v", err)
	}
	if n != 1 {
		t.Fatalf("watermark rows for the real store = %d, want 1", n)
	}

	// And nothing is retried.
	count, err = pub.Tick(context.Background())
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if count != 0 {
		t.Fatalf("second tick saw %d rows, want 0", count)
	}
}
```

Append to `internal/outbox/repository_integration_test.go`:

```go
// Guard against adding a reason constant without allowlisting it in
// sanitizeReason, which would silently record "unknown" instead.
func TestIntegration_MarkFailedInTx_StoreNotFoundIsInTheVocabulary(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: evt.ID, Reason: outbox.ReasonStoreNotFound},
		})
	}); err != nil {
		t.Fatalf("MarkFailedInTx: %v", err)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", evt.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Error == nil || *got.Error != outbox.ReasonStoreNotFound {
		t.Fatalf("error = %v, want %q (is it in sanitizeReason's switch?)", got.Error, outbox.ReasonStoreNotFound)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -count=1 -run 'StoreNotFound' ./internal/outbox/ -v
```

Expected: **compile failure** — `undefined: outbox.ReasonStoreNotFound`.

- [ ] **Step 3: Add the constant**

In `internal/outbox/models.go`, add to the existing Reason const block, before `ReasonUnknown`:

```go
	// ReasonStoreNotFound is written when a payload's store_id is
	// well-formed but has no matching row in `stores`. The watermark upsert
	// would raise an FK violation (store_watermarks.store_id REFERENCES
	// stores(id)), which ABORTS the whole transaction rather than failing
	// one row — see #374. Permanent in practice: stores are removed only by
	// tenant purge and hard-delete, both of which sweep this tenant's
	// outbox_events too.
	ReasonStoreNotFound = "store_not_found"
```

- [ ] **Step 4: Allowlist it — the step that is easy to miss**

In `internal/outbox/repository.go`, add it to `sanitizeReason`'s switch:

```go
	case ReasonPayloadUnparseable, ReasonPayloadMissingStoreID, ReasonStoreNotFound:
		return reason
```

- [ ] **Step 5: Track ids per store in `Tick`**

In `internal/outbox/publisher.go`, replace the flat `ids` accumulation with a per-store map so a
store's rows can be moved to `failures` wholesale once it is known to be missing.

Replace:

```go
		ids := make([]string, 0, len(rows))
		failures := make([]Failure, 0)
```

with:

```go
		idsByStore := make(map[string][]string, len(rows))
		failures := make([]Failure, 0)
```

and replace the append line (the one whose comment begins "Appended only now"):

```go
			ids = append(ids, r.ID)
```

with:

```go
			idsByStore[sid] = append(idsByStore[sid], r.ID)
```

- [ ] **Step 6: Add the pre-check and rebuild `ids`**

Still in `Tick`, insert this **between** the row loop and the `for k, ts := range byBucket` upsert
loop:

```go
		// Store-existence pre-check (#374). store_watermarks.store_id is
		// REFERENCES stores(id), so upserting a watermark for a store that
		// does not exist raises an FK violation — and an FK violation ABORTS
		// the Postgres transaction, so it does not fail one row, it takes the
		// whole batch: the good rows, the failure marks, everything. Those
		// rows then stay pending and are re-selected forever.
		//
		// Checking first turns that into a per-row terminal failure, the same
		// shape as the other two causes. One extra SELECT per tick, and only
		// when at least one row survived validation.
		if len(idsByStore) > 0 {
			storeIDs := make([]string, 0, len(idsByStore))
			for sid := range idsByStore {
				storeIDs = append(storeIDs, sid)
			}
			var found []struct{ ID string }
			if err := tx.Raw(`SELECT id FROM stores WHERE id IN ?`, storeIDs).
				Scan(&found).Error; err != nil {
				return err
			}
			present := make(map[string]struct{}, len(found))
			for _, f := range found {
				present[f.ID] = struct{}{}
			}
			for sid, rowIDs := range idsByStore {
				if _, ok := present[sid]; ok {
					continue
				}
				if p.logger != nil {
					p.logger.Warn("outbox publisher: store not found; failing",
						"store_id", sid, "events", len(rowIDs))
				}
				for _, id := range rowIDs {
					failures = append(failures, Failure{ID: id, Reason: ReasonStoreNotFound})
				}
				delete(idsByStore, sid)
			}
			// Drop every bucket whose store is gone, both axes.
			for k := range byBucket {
				if _, ok := present[k.storeID]; !ok {
					delete(byBucket, k)
				}
			}
		}

		ids := make([]string, 0, len(rows))
		for _, rowIDs := range idsByStore {
			ids = append(ids, rowIDs...)
		}
```

Deleting from a map while ranging over it is defined behaviour in Go — entries removed during
iteration are simply not produced.

- [ ] **Step 7: Run the tests to verify they pass**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -count=1 ./internal/outbox/ > /tmp/t374.txt 2>&1
echo "exit=$?"
tail -5 /tmp/t374.txt
```

Expected: whole package passes, including every #336 test.

- [ ] **Step 8: Document the fourth code in the design doc**

In `docs/superpowers/specs/2026-08-26-outbox-failure-state-design.md`, in the section listing the
closed vocabulary (§3, "The failure reason is a closed vocabulary, never a raw error"), add
`ReasonStoreNotFound = "store_not_found"` to the code block and one sentence of prose in the
document's voice explaining that it covers the FK case #374 describes, and that it is terminal for
the same reason the other two are — the condition is a permanent property of the row.

- [ ] **Step 9: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/outbox docs/superpowers/specs/2026-08-26-outbox-failure-state-design.md
git commit -m "fix(outbox): fail rows whose store is missing instead of aborting the batch"
```

---

## Task 2: #375 metrics and #376 comment

Two independent small changes, batched because both are trivial and neither needs its own review
surface. **Two separate commits.**

**Files:**
- Modify: `internal/metrics/registry.go`, `internal/outbox/publisher.go` (#375)
- Modify: `cmd/marketplace-api/main.go` (#376)

**Interfaces:**
- Consumes: `ids` and `failures` from Task 1's `Tick`.
- Produces: `metrics.OutboxEventsFailedTotal`. Removes `metrics.OutboxEventsPending`.

**The correctness point in this task.** The counters must be incremented **after `ProcessBatch`
returns nil**, never inside the callback. The callback runs inside a transaction that can roll back,
and incrementing there would count work that never committed. A Prometheus counter cannot be
decremented, so an over-count is permanent.

- [ ] **Step 1: Delete the dead gauge, add a failed counter**

In `internal/metrics/registry.go`, delete the whole `OutboxEventsPending` block:

```go
	// OutboxEventsPending tracks the current outbox queue depth.
	OutboxEventsPending = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "outbox_events_pending",
			Help: "Number of pending outbox events.",
		},
	)
```

and delete its line from the `prometheus.MustRegister(...)` list:

```go
		OutboxEventsPending,
```

Then add, immediately after the `OutboxEventsPublishedTotal` block:

```go
	// OutboxEventsFailedTotal counts outbox events the publisher gave up on.
	// There is deliberately no pending GAUGE beside these two counters:
	// /admin/health reports pending depth, oldest-pending age and errored
	// count from a DB query, authoritatively, whereas a gauge set by the
	// publisher would be reported identically by every replica running in
	// admin or both mode — so any dashboard summing it would multiply.
	OutboxEventsFailedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "outbox_events_failed_total",
			Help: "Total outbox events marked terminally failed by the publisher.",
		},
	)
```

and add `OutboxEventsFailedTotal,` to the `MustRegister` list where `OutboxEventsPending` was.

- [ ] **Step 2: Wire the counters and document `Tick`'s return**

In `internal/outbox/publisher.go`, add the metrics import
(`"github.com/mark8ly/marketplace-api/internal/metrics"`) and restructure `Tick` so the counters are
incremented only after a successful commit:

```go
func (p *Publisher) Tick(ctx context.Context) (int, error) {
	var published, failed int
	seen, err := p.repo.ProcessBatch(ctx, p.batch, func(tx *gorm.DB, rows []OutboxEvent) error {
		// Reset per attempt: ProcessBatch's callback can run again, and a
		// counter that survived a rolled-back attempt would over-count.
		published, failed = 0, 0

		// ... existing body unchanged, up to and including the two marks ...

		published, failed = len(ids), len(failures)
		if err := p.repo.MarkFailedInTx(tx, failures); err != nil {
			return err
		}
		return p.repo.MarkPublishedInTx(tx, ids)
	})
	if err != nil {
		return seen, err
	}
	// AFTER the commit, never inside the callback: the transaction can roll
	// back, and a Prometheus counter cannot be decremented, so an over-count
	// is permanent.
	metrics.OutboxEventsPublishedTotal.Add(float64(published))
	metrics.OutboxEventsFailedTotal.Add(float64(failed))
	return seen, nil
}
```

Keep the existing body of the callback exactly as Task 1 left it; only the wrapper changes.

Then extend `Tick`'s doc comment with:

```go
// The returned int is the number of rows the poll SAW — not the number
// published. Since #336 a batch can be entirely failed, so a non-zero return
// says work was examined, not that anything was delivered. The
// outbox_events_published_total and outbox_events_failed_total counters are
// what separate those two outcomes.
```

- [ ] **Step 3: Verify**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
go build ./... && echo "build ok"
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -count=1 ./internal/outbox/ > /tmp/t375.txt 2>&1
echo "exit=$?"
tail -3 /tmp/t375.txt
grep -rn "OutboxEventsPending" --include=*.go . || echo "gauge fully removed"
```

Expected: build ok, package green, and no remaining reference to the deleted gauge (a leftover in
`MustRegister` is a compile error, which is the point).

- [ ] **Step 4: Commit #375**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/metrics/registry.go services/marketplace-api/internal/outbox/publisher.go
git commit -m "feat(outbox): count published and failed events, drop the dead pending gauge"
```

- [ ] **Step 5: Fix the #376 comment**

In `services/marketplace-api/cmd/marketplace-api/main.go`, replace:

```go
	// Outbox publisher — runs in admin and both modes; the storefront
	// process does not produce events, so running it there would just poll
	// an always-empty table and waste a connection.
```

with:

```go
	// Outbox publisher — runs in admin and both modes because admin owns
	// draining, not because the storefront produces nothing. It does: public
	// checkout in storefront mode writes outbox_events rows through
	// orderSvcSF (see the Orders M5 wiring above), and this replica drains
	// them. Running a second publisher there would duplicate the poll, not
	// find an empty table.
```

- [ ] **Step 6: Verify and commit #376**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
go build ./... && echo "build ok"
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "docs: correct the outbox publisher comment about storefront event production"
```

---

## Task 3: Whole-branch verification

**Files:** none modified — this task produces evidence.

- [ ] **Step 1: Build-tagged compile check**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
go vet -tags=integration ./... > /tmp/vet374.txt 2>&1
echo "exit=$?"
tail -10 /tmp/vet374.txt
```

Expected: `exit=0`.

- [ ] **Step 2: Capture the branch failing set**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 ./... > /tmp/fu-branch.txt 2>&1
echo "go test exit=$?"
grep -E '^FAIL\s+github' /tmp/fu-branch.txt | awk '{print $2}' | sort -u > /tmp/fu-branch-pkgs.txt
wc -l < /tmp/fu-branch-pkgs.txt
```

- [ ] **Step 3: Compare against the merge base**

The baseline is `main` at `0fc2763f`, which already contains #336. Its failing set was measured
during that work at **22 packages / 191 tests**. Re-measure rather than trusting that number:

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git worktree remove /tmp/m8-fu-baseline --force 2>/dev/null
git worktree add /tmp/m8-fu-baseline 0fc2763f
cd /tmp/m8-fu-baseline/services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 ./... > /tmp/fu-main.txt 2>&1
echo "go test exit=$?"
grep -E '^FAIL\s+github' /tmp/fu-main.txt | awk '{print $2}' | sort -u > /tmp/fu-main-pkgs.txt
wc -l < /tmp/fu-main-pkgs.txt
```

**Run the two sequentially, never concurrently — they share one database** and would corrupt each
other's fixtures, making both results meaningless.

- [ ] **Step 4: Diff both directions**

```bash
echo ">>> failing on BRANCH but not on 0fc2763f (must be empty):"
comm -13 /tmp/fu-main-pkgs.txt /tmp/fu-branch-pkgs.txt
echo ">>> failing on 0fc2763f but not on BRANCH:"
comm -23 /tmp/fu-main-pkgs.txt /tmp/fu-branch-pkgs.txt
```

Expected: the first list EMPTY. Unlike the #336 branch, no reduction is expected here either — this
work fixes no pre-existing test. A package in the first list is a defect this branch introduced;
**stop and report it, do not fix it silently.**

- [ ] **Step 5: Clean up**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git worktree remove /tmp/m8-fu-baseline --force
git worktree list
```

- [ ] **Step 6: Record the evidence**

Append a "Verification record" section to this plan file with both counts, the both-directions
comparison, and the verdict. Commit:

```bash
git add docs/superpowers/plans/2026-08-26-outbox-followups.md
git commit -m "docs: record the verification diff for the outbox follow-ups branch"
```

---

## Verification record

Executed 2026-08-26. Branch `fix/374-outbox-followups` at `cf20ea47f7f1ba27c5cc4fceebdd0bc8b7f996c9`,
compared against baseline `main` at `0fc2763f6cdc3b3fdbaf04aa1cfe226d8e8bdace` (contains #336).

- **`go vet -tags=integration ./...`** — `exit=0`, no output.
- **Branch full-suite integration run** — `go test -tags=integration -p 1 ./...`, exit=1 (expected —
  pre-existing failures). Failing set: **22 packages / 191 tests**.
- **Baseline full-suite integration run** (`/tmp/m8-fu-baseline` worktree at `0fc2763f`) — same
  command, exit=1. Failing set: **22 packages / 191 tests**.
- **Both-directions package diff**:
  - Failing on BRANCH but not on `0fc2763f`: empty.
  - Failing on `0fc2763f` but not on BRANCH: empty.
  - The two package lists are byte-identical.
- **Test-name diff** (to rule out same-package-different-test drift): the 191 `--- FAIL:` test names
  on each side are identical after stripping per-run timing suffixes; only timing values differed.

**Verdict:** the branch introduced no new test failures and fixed none — the failing set is
identical in both directions, as expected for a branch whose scope (#374, #375, #376) touches no
code path covered by the pre-existing failures.

---

## Not in this plan

- **The #336 production verification exercise.** Still outstanding, separate, and requires explicit
  go-ahead at the moment of the write.
- **A `CHECK` constraint on `outbox_events.error`.** It would make the vocabulary a database
  guarantee rather than a Go one, but it reopens the no-migration decision and would break the
  manual-`UPDATE` requeue path. The design doc §5 already records that #331 must treat the column as
  opaque, which is the mitigation.
