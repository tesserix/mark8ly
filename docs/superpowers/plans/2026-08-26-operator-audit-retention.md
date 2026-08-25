# Operator audit retention (#365) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give operator audit rows a stated 7-year retention and stop an erasure purge's free-text `reason` from outliving the tenant it erased.

**Architecture:** A second, additive prune path in `internal/audit` with no join (the existing one joins `store_subscriptions` on `store_id`, which operator rows never have), plus a write-side carve-out in the purge handler that omits `reason` entirely when `reason_code = "erasure_request"`.

**Tech Stack:** Go 1.26, GORM, Postgres 15, gin, testify, prometheus.

**Spec:** `docs/superpowers/specs/2026-08-26-operator-audit-retention-design.md` — read it before Task 1. It records the four decisions and why each alternative was rejected.

## Global Constraints

- Run every command from `services/marketplace-api`. Never path-scope `go test ./...`.
- `go vet -tags=integration ./...` is the ONLY command that compiles `//go:build integration` files. Part of the verification set for every task. `go vet -tags=stripelive ./internal/billing/trial/` must also stay clean.
- `set -o pipefail` whenever a command's exit code is evidence you report. A past session reported `exit=0` over a plainly FAILED suite because it read `tail`'s status.
- Integration runs: `-p 1 -count=1`, env `TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"` — the LAN IP, never `localhost`, and never `TEST_DB_DSN` (under that name tests SKIP silently while the package prints `ok`).
- Confirm from VERBOSE output that new tests RAN. `--- SKIP` and `--- PASS` are one character apart.
- Retention is **7 years from `created_at`**, matching `billing_archive` (migration `000046:24`, §23.2).
- Comments explain WHY and cite issue numbers. `prune_cron.go` and `tenant_purge.go` are heavily and carefully commented — match that density.
- **Do NOT** push, open a PR, merge, or deploy. **Do NOT** run kubectl, gcloud, argocd, or gh.

### Pre-existing failures — not yours, do not fix, do not let them mask yours

`internal/subscription/planchange` (9 FAIL, `store_subscriptions_store_id_fkey` fixture drift, #317) · `internal/whitelabel` nil panic · `internal/outbox` 2 FAIL · appaddon, dispatch, tax, tax/revalidation on the same FK drift.

### Three comments in the tree are FALSE and are part of this work

Verified 2026-08-26. #288 added `actor_type <> 'operator'` to the purge's audit delete, and three comments still describe the world before it:

1. **`internal/audit/prune_cron.go`** (in `pruneBucket`'s #311 block): *"internal/tenantpurge/purge.go:238 deletes audit_logs by tenant_id and still reaches these rows, so GDPR erasure and tenant deletion are unaffected by this guard."* **False.** The real statement is at `purge.go:370` and reads `DELETE FROM audit_logs WHERE tenant_id = ? AND actor_type <> 'operator'` — it explicitly does NOT reach operator rows. The line number is also wrong. A reader trusting this concludes erasure already handles these rows and closes #365 as invalid.
2. **`internal/handlers/platformadmin/tenant_purge.go:85`** — quotes `purgePlan contains DELETE FROM audit_logs WHERE tenant_id = ?`, omitting the `actor_type` half.
3. **`internal/handlers/platformadmin/tenant_purge.go:249`** — same quote, same omission. Note its *conclusion* (audit LAST and SYNCHRONOUS) remains correct for the reason stated immediately after it — the row records the OUTCOME — so correct the premise without disturbing the conclusion.

Nothing anywhere in the estate deletes operator audit rows today. That is the whole of #365.

## File Structure

| file | responsibility |
|---|---|
| `internal/audit/operator_prune.go` (create) | the 7-year, join-less prune for `actor_type='operator'` |
| `internal/audit/operator_prune_integration_test.go` (create) | boundary + negative-guard tests against real Postgres |
| `internal/audit/prune_cron.go` (modify) | call the new path from `Run`; correct the false #311 comment |
| `internal/audit/prune_cron_storeless_integration_test.go` (modify) | narrow its `#311` assertion message; add the discriminating 7-year pair |
| `internal/handlers/platformadmin/tenant_purge.go` (modify) | omit `reason` on `erasure_request`; disclose it; correct two stale comments |
| `internal/handlers/platformadmin/tenant_purge_test.go` (modify) | the carve-out's discriminating pair |
| `internal/metrics/registry.go` (modify) | document the new bucket label |
| `cmd/marketplace-api/main.go` (modify) | comment only — the cron call site already invokes `Run` |

---

### Task 1: the erasure free-text carve-out

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/tenant_purge.go`
- Test: `services/marketplace-api/internal/handlers/platformadmin/tenant_purge_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `purgeResponse` gains `ReasonNotRetained bool \`json:"reason_not_retained,omitempty"\``.

- [ ] **Step 1: Write the failing tests**

Append to `tenant_purge_test.go`. The helpers you need ALREADY EXIST in that file — use them, do not add a parallel set:

- `doPurge(t, td, pg, emit, body)` issues the POST and returns the recorder.
- `fakeTeardown` / `fakePurger` with a shared `&seq{}`.
- `emit` is a plain closure of type `func(*gin.Context, uuid.UUID, audit.Event) error`.

Two shapes to get right, both verified against the code:

- **There is no `confirm` field.** `purgeRequest` is `{StoreSlugs *[]string, ReasonCode string, Reason string}`, and the confirmation IS `store_slugs` — it must be present, with `[]` confirming a tenant that has no stores. A body carrying `"confirm"` is rejected with 400 for a missing `store_slugs`.
- **The response is enveloped**: the payload sits under `data`, so assert on `got["data"]`, not on the top level.

```go
// THE CARVE-OUT. On an erasure_request purge the free-text reason must never
// be written. Asserted on the METADATA the audit event carries, not on the
// response: the response is what we say, the metadata is what survives the
// tenant for seven years.
func TestPurge_ErasureRequest_DropsFreeTextFromAuditMetadata(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, TenantName: "The Bondi Store",
		StoreIDs: []string{"s-1"}, StoreSlugs: []string{"a"},
	}}
	pg := &fakePurger{seq: sq, rep: tenantpurge.Report{
		Tables: []tenantpurge.TableResult{{Table: "products", RowsDeleted: 3}}, TotalRows: 3,
	}}
	var got audit.Event
	emit := func(_ *gin.Context, _ uuid.UUID, ev audit.Event) error { got = ev; return nil }

	rec := doPurge(t, td, pg, emit,
		`{"store_slugs":["a"],"reason_code":"erasure_request","reason":"cust jane@example.com asked, ticket 4471"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	_, present := got.Metadata["reason"]
	require.False(t, present,
		"an erasure purge's free text must be ABSENT from the metadata, not empty — it outlives the tenant")
	require.Equal(t, "erasure_request", got.Metadata["reason_code"],
		"reason_code must survive: it is what tells a regulator a statutory clock applied")

	// The operator is told, so nobody believes they documented something
	// that silently vanished.
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "response is enveloped under data: %s", rec.Body.String())
	require.Equal(t, true, data["reason_not_retained"])
}

// THE DISCRIMINATING PAIR (trap 13). The SAME free text under a different
// reason code MUST persist. Without this, the test above passes against an
// implementation that drops `reason` unconditionally, and every purge loses
// its context.
func TestPurge_NonErasureReasonCodes_KeepFreeText(t *testing.T) {
	const text = "cust jane@example.com asked, ticket 4471"
	for _, code := range []string{"merchant_request", "fraud", "abandoned", "legal", "operator_error"} {
		t.Run(code, func(t *testing.T) {
			sq := &seq{}
			td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
				TenantID: tenantID, TenantName: "The Bondi Store",
				StoreIDs: []string{"s-1"}, StoreSlugs: []string{"a"},
			}}
			pg := &fakePurger{seq: sq, rep: tenantpurge.Report{TotalRows: 0}}
			var got audit.Event
			emit := func(_ *gin.Context, _ uuid.UUID, ev audit.Event) error { got = ev; return nil }

			rec := doPurge(t, td, pg, emit,
				`{"store_slugs":["a"],"reason_code":"`+code+`","reason":"`+text+`"}`)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			require.Equal(t, text, got.Metadata["reason"],
				"only erasure_request drops free text; %s must keep it", code)
			require.NotContains(t, rec.Body.String(), "reason_not_retained",
				"nothing was dropped, so the disclosure key must be absent (omitempty)")
		})
	}
}

// An erasure purge with NO free text must not claim text was dropped.
func TestPurge_ErasureRequest_NoText_DoesNotClaimADrop(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, TenantName: "The Bondi Store",
		StoreIDs: []string{"s-1"}, StoreSlugs: []string{"a"},
	}}
	pg := &fakePurger{seq: sq, rep: tenantpurge.Report{TotalRows: 0}}

	rec := doPurge(t, td, pg, noopEmit, `{"store_slugs":["a"],"reason_code":"erasure_request"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "reason_not_retained",
		"nothing was dropped, so nothing should be disclosed")
}
```

- [ ] **Step 2: Run them and verify they fail**

```bash
cd services/marketplace-api
set -o pipefail
go test -count=1 -run 'TestPurge_ErasureRequest|TestPurge_NonErasureReasonCodes' ./internal/handlers/platformadmin/ -v 2>&1 | tail -25
```

Expected: FAIL — `reason` is present in the metadata, and `reason_not_retained` is missing from the response.

- [ ] **Step 3: Implement the carve-out**

In `tenant_purge.go`, where the metadata map is built (around line 262, `"reason": reason`), replace the unconditional assignment:

```go
	// #365: on a statutory erasure the free text is NEVER written.
	//
	// This row is excluded from the purge's own audit_logs delete
	// (purge.go:370, actor_type <> 'operator') so that the outbox backstop
	// cannot destroy the record of the destruction (#288). It therefore
	// OUTLIVES the tenant — and free text on an erasure is exactly where an
	// operator writes what the erasure was for ("cust jane@…, ticket 4471"),
	// so the surviving row would carry the thing being erased.
	//
	// Never written rather than stripped later: CloudNativePG backs up to
	// GCS continuously (3-day PITR), so text that exists even briefly is
	// captured there. "We never recorded it" is both stronger and easier to
	// explain than a two-stage rule.
	//
	// reason_code SURVIVES. PurgeReasonCodes keeps merchant_request and
	// erasure_request distinct precisely because "only the second carries a
	// statutory clock, and an audit trail that cannot tell them apart cannot
	// answer the question a regulator asks" — dropping the category as well
	// would defeat the reason the category exists.
	metadata := map[string]any{
		"reason_code": req.ReasonCode,
		// … the other existing keys unchanged …
	}
	reasonNotRetained := false
	if req.ReasonCode == "erasure_request" {
		reasonNotRetained = reason != ""
	} else {
		metadata["reason"] = reason
	}
```

Keep every other metadata key exactly as it is. Add the field to `purgeResponse`:

```go
	// ReasonNotRetained is set only when free text was supplied on an
	// erasure_request purge and therefore discarded. omitempty: a purge that
	// dropped nothing carries no such key, so its presence always means
	// something was actually discarded (#365).
	ReasonNotRetained bool `json:"reason_not_retained,omitempty"`
```

and populate it where the response is assembled.

Leave the request-level `reason` handling otherwise untouched: the purge is NOT refused for carrying text — blocking an art.17 purge against a statutory deadline on a formatting objection is a worse outcome than a discarded sentence (see the spec).

- [ ] **Step 4: Correct the two stale comments in this file**

At `tenant_purge.go:85` and `:249`, both quote `purgePlan contains DELETE FROM audit_logs WHERE tenant_id = ?`. Since #288 the statement is:

```
DELETE FROM audit_logs WHERE tenant_id = ? AND actor_type <> 'operator'
```

Correct the quoted SQL in both places. At `:249`, keep the conclusion — audit LAST and SYNCHRONOUS — because the sentence immediately after it gives the reason that still holds (the row records the OUTCOME). Note in that comment that with the exclusion an operator row written *before* the purge would now survive it, so the ordering rests on the outcome argument rather than on the row being destroyed.

- [ ] **Step 5: Run the tests and verify they pass**

```bash
cd services/marketplace-api
set -o pipefail
go test -count=1 ./internal/handlers/platformadmin/... 2>&1 | tail -5
go vet -tags=integration ./... 2>&1 | tail -3
```

- [ ] **Step 6: Confirm the golden fixtures did not move**

```bash
cd services/marketplace-api
set -o pipefail
git diff --stat internal/handlers/platformadmin/testdata/
```

Expected: EMPTY. `tenant_purge.golden.json` is driven by a non-erasure fixture and `reason_not_retained` is `omitempty`, so nothing should change. **If a golden moves, do not regenerate it** — it means the new key is leaking onto purges that dropped nothing. Fix the code.

- [ ] **Step 7: Prove the carve-out by mutation**

Make the carve-out unconditional (drop `reason` for every reason code) and re-run. `TestPurge_NonErasureReasonCodes_KeepFreeText` MUST fail. Then make it never apply and confirm `TestPurge_ErasureRequest_DropsFreeTextFromAuditMetadata` fails. Revert both; confirm `git status --short` is clean. Quote both failures in the commit message.

- [ ] **Step 8: Commit**

```bash
git add internal/handlers/platformadmin/tenant_purge.go internal/handlers/platformadmin/tenant_purge_test.go
git commit -m "feat(platformadmin): never record free text on an erasure purge, and correct two stale purge comments (#365)"
```

---

### Task 2: the 7-year operator prune

**Files:**
- Create: `services/marketplace-api/internal/audit/operator_prune.go`
- Create: `services/marketplace-api/internal/audit/operator_prune_integration_test.go`
- Modify: `services/marketplace-api/internal/audit/prune_cron.go`
- Modify: `services/marketplace-api/internal/audit/prune_cron_storeless_integration_test.go`

**Interfaces:**
- Consumes: `PruneCron` (fields `db`, `logger`, `clock`, `batchSize`, `counter`), `PruneBatchSize`, `CounterFn`.
- Produces:
  - `const OperatorRetentionYears = 7`
  - `const OperatorMetricLabel = "operator_7y"`
  - `func (c *PruneCron) pruneOperatorRows(ctx context.Context, cutoff time.Time) (int64, int, error)`
  - `PruneStats` gains `OperatorRowsDeleted int64`

- [ ] **Step 1: Write the failing tests**

Create `internal/audit/operator_prune_integration_test.go`:

```go
//go:build integration

package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// insertOperatorRow seeds one store-less operator audit row at a given age.
func insertOperatorRow(t *testing.T, db *gorm.DB, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO audit_logs (id, tenant_id, store_id, actor_type, action, resource_type, status, severity, created_at)
		 VALUES (?, ?, NULL, 'operator', 'tenant.purged', 'tenant', 'success', 'warning', ?)`,
		id, uuid.New(), createdAt,
	).Error)
	return id
}

// THE BOUNDARY, ON THE BOUNDARY. Seven years minus a second survives;
// seven years plus a second is deleted. "Close to the edge" is not the edge.
func TestOperatorPrune_SevenYearBoundary(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	ctx := context.Background()

	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(-audit.OperatorRetentionYears, 0, 0)

	survives := insertOperatorRow(t, db, cutoff.Add(time.Second))
	deleted := insertOperatorRow(t, db, cutoff.Add(-time.Second))

	cron := audit.NewPruneCron(db, nil, func() time.Time { return now }, 0)
	stats, err := cron.Run(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.OperatorRowsDeleted, int64(1))

	require.Equal(t, int64(1), countRows(t, db, survives),
		"a row one second inside seven years must survive")
	require.Equal(t, int64(0), countRows(t, db, deleted),
		"a row one second past seven years must be deleted")
}

// THE NEGATIVE GUARD. #311 says store-less rows are never pruned; #365
// narrows that for actor_type='operator' ONLY. A store-less row of any other
// actor_type must still survive, however old — otherwise the narrowing is
// wider than it was written to be.
func TestOperatorPrune_LeavesNonOperatorStoreLessRows(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	ctx := context.Background()

	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	ancient := now.AddDate(-20, 0, 0)

	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO audit_logs (id, tenant_id, store_id, actor_type, action, resource_type, status, severity, created_at)
		 VALUES (?, ?, NULL, 'system', 'tenant.something', 'tenant', 'success', 'info', ?)`,
		id, uuid.New(), ancient,
	).Error)

	cron := audit.NewPruneCron(db, nil, func() time.Time { return now }, 0)
	_, err := cron.Run(ctx)
	require.NoError(t, err)

	require.Equal(t, int64(1), countRows(t, db, id),
		"only actor_type='operator' is pruned by the #365 path; #311 still covers the rest")
}

// The batch loop must terminate and delete everything eligible, not just one
// batch's worth.
func TestOperatorPrune_DeletesBeyondOneBatch(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	ctx := context.Background()

	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(-audit.OperatorRetentionYears, 0, -1)
	for i := 0; i < 5; i++ {
		insertOperatorRow(t, db, old)
	}

	cron := audit.NewPruneCron(db, nil, func() time.Time { return now }, 2) // batchSize 2
	stats, err := cron.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(5), stats.OperatorRowsDeleted,
		"the loop must continue past the first batch")
}

func countRows(t *testing.T, db *gorm.DB, id uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM audit_logs WHERE id = ?`, id).Scan(&n).Error)
	return n
}
```

Add `"gorm.io/gorm"` to the imports.

Then AMEND `prune_cron_storeless_integration_test.go`. Its store-less operator row is seeded at `longAgo.AddDate(0, 0, -800)` ≈ 2.7 years old, so it still survives — but its assertion message is now misleading. Change it to:

```go
	require.Equal(t, int64(1), storelessCount,
		"a store-less operator row must survive the PLAN-BASED prune (#311); it is under the 7-year operator window (#365)")
```

and add, in the same test, the discriminating sibling so the file states both halves of the rule:

```go
	// #365's other half: the same kind of row, past seven years, IS pruned.
	// Without this the file asserts only "operator rows survive", which is
	// no longer the whole rule.
	ancientOperatorID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO audit_logs (id, tenant_id, store_id, actor_type, action, resource_type, status, severity, created_at)
		 VALUES (?, ?, NULL, 'operator', 'tenant.purged', 'tenant', 'success', 'warning', ?)`,
		ancientOperatorID, tenantID, time.Now().UTC().AddDate(-8, 0, 0),
	).Error)
```

seeded BEFORE `cron.Run(ctx)` is called, with an assertion after it that its count is 0.

- [ ] **Step 2: Run and verify they fail**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -tags=integration -count=1 -p 1 -run 'TestOperatorPrune' ./internal/audit/ -v 2>&1 | tail -20
```

Expected: compile failure — `audit.OperatorRetentionYears` undefined, `stats.OperatorRowsDeleted` undefined.

Confirm from the verbose output that the tests RUN. A `--- SKIP` means `TEST_DATABASE_URL` is unset and you are proving nothing.

- [ ] **Step 3: Implement the operator prune**

Create `internal/audit/operator_prune.go`:

```go
package audit

import (
	"context"
	"fmt"
	"time"
)

// OperatorRetentionYears is how long an operator audit row is kept, counted
// from created_at.
//
// Seven years is not a new number: billing_archive is documented as
// "retained 7 years after hard-delete under legal-obligation basis"
// (migration 000046_billing_archive.up.sql:24, §23.2). An operator
// governance record about a destruction is the same class of artefact under
// the same basis, and reusing the number leaves the estate ONE retention
// story to defend rather than two that have to be reconciled (#365).
const OperatorRetentionYears = 7

// OperatorMetricLabel is the bucket label this path reports under, so
// operator pruning and plan-based pruning are distinguishable in monitoring.
const OperatorMetricLabel = "operator_7y"

// pruneOperatorRows deletes audit_logs rows with actor_type='operator' older
// than cutoff, in batches, and returns (rowsDeleted, batches, error).
//
// Deliberately NOT a fourth retentionBucket. The plan-based path derives its
// window from the row's store's plan and JOINs store_subscriptions on
// store_id; operator rows carry store_id = NULL by design (a store-scoped
// operator row would surface a platform action inside the MERCHANT's own
// audit view), and after a purge that tenant's store_subscriptions rows are
// gone anyway. So the join can never match, and a bucket would mean
// special-casing the very join pruneBucket is built around. The two rules
// have different shapes: one is plan-derived and store-scoped, this one is
// flat and store-less.
//
// This NARROWS #311, which decided store-less audit rows are never pruned.
// That decision still stands for every actor_type EXCEPT 'operator'.
func (c *PruneCron) pruneOperatorRows(ctx context.Context, cutoff time.Time) (int64, int, error) {
	var totalDeleted int64
	batchCount := 0

	for {
		select {
		case <-ctx.Done():
			return totalDeleted, batchCount, ctx.Err()
		default:
		}

		res := c.db.WithContext(ctx).Exec(`
			DELETE FROM audit_logs
			WHERE id IN (
				SELECT id
				FROM audit_logs
				WHERE actor_type = 'operator'
				  AND created_at < ?
				LIMIT ?
			)`,
			cutoff, c.batchSize,
		)
		if res.Error != nil {
			return totalDeleted, batchCount, fmt.Errorf("audit operator prune delete: %w", res.Error)
		}
		batchCount++
		totalDeleted += res.RowsAffected
		if res.RowsAffected == 0 {
			return totalDeleted, batchCount, nil
		}
	}
}
```

In `prune_cron.go`, add the field to `PruneStats`:

```go
	// OperatorRowsDeleted counts rows removed by the 7-year operator path
	// (#365), kept separate from RowsDeleted so the two retention rules stay
	// distinguishable in logs and telemetry.
	OperatorRowsDeleted int64
```

and call it at the end of `Run`, before `return stats, nil`:

```go
	// #365 — operator rows, 7 years, no join. Failure is logged and
	// swallowed like a bucket failure: a lock conflict here must not fail
	// the whole pass.
	opCutoff := now.AddDate(-OperatorRetentionYears, 0, 0)
	opDeleted, opBatches, err := c.pruneOperatorRows(ctx, opCutoff)
	stats.OperatorRowsDeleted = opDeleted
	stats.BatchesRun += opBatches
	if c.counter != nil && opDeleted > 0 {
		c.counter(OperatorMetricLabel, opDeleted)
	}
	if err != nil {
		stats.ErrorsByPlan["operator (7 year retention)"]++
		c.logger.Error("audit prune: operator path failed",
			"deleted_so_far", opDeleted, "err", err.Error())
	} else {
		c.logger.Info("audit prune: operator path complete",
			"cutoff", opCutoff.Format(time.RFC3339),
			"rows_deleted", opDeleted, "batches", opBatches)
	}
```

- [ ] **Step 4: Correct the false comment in `pruneBucket`**

Its #311 block currently ends with a claim that is FALSE since #288:

> "internal/tenantpurge/purge.go:238 deletes audit_logs by tenant_id and still reaches these rows, so GDPR erasure and tenant deletion are unaffected by this guard."

The real statement is at `purge.go:370` and is `DELETE FROM audit_logs WHERE tenant_id = ? AND actor_type <> 'operator'` — it does NOT reach operator rows. Replace that paragraph with an accurate one: purge deliberately spares operator rows (#288, so the outbox backstop cannot destroy the record of the destruction), which is exactly why they need their own retention, and that retention is `pruneOperatorRows` in `operator_prune.go` at seven years (#365). Keep the rest of the block — the `store_id IS NOT NULL` guard and the instruction not to remove it — intact and still accurate.

- [ ] **Step 5: Run and verify they pass**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -tags=integration -count=1 -p 1 ./internal/audit/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
go vet -tags=integration ./... 2>&1 | tail -3
```

Expected: every `TestOperatorPrune_*` PASSes, and every pre-existing test in the package still passes — including `TestPruneCronSkipsStoreLessRows` with its amended assertions.

- [ ] **Step 6: Prove the boundary and the guard by mutation**

Three mutations, each reverted:
1. Change `created_at < ?` to `created_at <= ?`. `TestOperatorPrune_SevenYearBoundary` MUST fail on the survivor.
2. Drop `AND actor_type = 'operator'`. `TestOperatorPrune_LeavesNonOperatorStoreLessRows` MUST fail.
3. Return after the first batch instead of looping. `TestOperatorPrune_DeletesBeyondOneBatch` MUST fail.

Quote each observed failure in the commit message. Confirm `git status --short` is clean afterwards.

- [ ] **Step 7: Commit**

```bash
git add internal/audit/
git commit -m "feat(audit): prune operator audit rows at seven years, and correct a false claim about GDPR reach (#365)"
```

---

### Task 3: telemetry and the wiring comment

**Files:**
- Modify: `services/marketplace-api/internal/metrics/registry.go:105-114`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go:1705-1720`

**Interfaces:**
- Consumes: `audit.OperatorMetricLabel` (Task 2).
- Produces: nothing.

The counter itself needs no code change — `WithCounter` is already wired at the call site and the new path calls it with its own label. What must change is the prose, in both places, because both currently describe a two-bucket world.

- [ ] **Step 1: Correct the metric's help text**

`AuditPruneRowsDeletedTotal`'s comment says *"labeled by retention bucket (trial_starter_90d, studio_365d). Pro is unlimited and never pruned."* Add `operator_7y` to that list and say what it means: operator audit rows, retained seven years from `created_at` under the same legal-obligation basis as `billing_archive` (#365). Keep the Pro sentence — still true.

- [ ] **Step 2: Correct the cron registration comment**

`main.go` says the windows are *"Per-plan windows (Trial/Starter 90d, Studio 365d, Pro unlimited) … Multi-tenant safe: each DELETE joins audit_logs to store_subscriptions on store_id"*. That is now only half the pass. Add that the same run also prunes `actor_type='operator'` rows at seven years via a join-less path, because those rows carry no `store_id` and are unreachable by the plan-based DELETE (#365).

- [ ] **Step 3: Verify nothing else claims a two-bucket world**

```bash
cd services/marketplace-api
set -o pipefail
grep -rn "trial_starter_90d\|studio_365d\|retentionBuckets" --include='*.go' . | grep -v _test
```

Read each hit. Any comment that enumerates the buckets as the complete set must now include the operator path. Fix each one you find, and list them in your report — a partial correction leaves exactly the kind of stale claim this task exists to remove.

- [ ] **Step 4: Full verification**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go build ./... && go vet ./... && go vet -tags=integration ./... && go vet -tags=stripelive ./internal/billing/trial/ && echo "ALL CLEAN"
go test -count=1 ./... 2>&1 | grep -Ev '^ok|no test files' | head -10
go test -tags=integration -count=1 -p 1 ./internal/audit/ ./internal/handlers/platformadmin/ 2>&1 | grep -E '^(ok|FAIL|--- FAIL)'
```

Then confirm the pre-existing planchange set is still exactly its 9 names:

```bash
go test -tags=integration -count=1 -p 1 ./internal/subscription/planchange/... 2>&1 \
  | grep '^--- FAIL' | sed 's/ *(.*//' | sort
```

Expected: the 9 documented `store_subscriptions_store_id_fkey` failures, never a tenth.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/registry.go cmd/marketplace-api/main.go
git commit -m "docs(audit): describe the operator retention bucket at both telemetry sites (#365)"
```

---

## After the last task

1. **Whole-branch review on the most capable model.** The composition questions here: does the operator prune interact with the purge's `actor_type <> 'operator'` exclusion in any order that loses a row early; can `pruneOperatorRows` and a concurrent purge deadlock on `audit_logs`; and is the erasure carve-out reachable from any other write path that also emits operator rows (suspend, unsuspend, trial extend all do — none takes a `reason_code` of `erasure_request`, but confirm rather than assume).
2. **Mutation, not reading.** Every finding that mattered on the last two branches came from a mutation failing to fail.
3. **Update #311 on GitHub**, not only in code. It records the decision that store-less audit rows are never pruned; that now holds for every `actor_type` except `operator`. Two documents quietly disagreeing is how the false claim in `prune_cron.go` survived in the first place. (Controller does this — implementers must not run `gh`.)
4. **Do not push, open a PR, merge, or deploy.** Ask first.
