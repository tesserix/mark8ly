# #397 — Blocked-downgrade audit row survives the refusal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `downgrade_blocked_over_quota` row in `subscription_plan_change_audit` persist when an interactive downgrade is refused over quota, instead of being rolled back with the transaction that refuses it.

**Architecture:** `executeDowngradeSchedule` currently writes the blocked audit row on the same `tx` it then aborts by returning `ErrStoreCountOverQuota`. We hand the row *out* of the advisory-lock closure instead of writing it inside, and `Execute` writes it on the pooled handle (`o.deps.DB`) **after** `WithAdvisoryLock` has returned and the lock is released. The store-count *read* stays on `tx` (it must remain consistent with uncommitted rows under the lock); only the *write* moves.

**Tech Stack:** Go 1.26, GORM, Postgres, testify. Integration tests are `//go:build integration`, gated on `TEST_DATABASE_URL`.

**Spec:** GitHub issue tesserix/mark8ly#397, plus the verification findings recorded below.

## Global Constraints

- Run all Go commands from `services/marketplace-api`, never path-scoped, always `-count=1`.
- Required command set: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`. `go vet -tags=integration ./...` is the only command that compiles build-tagged files — include it.
- Integration tests must be run with `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'` and `-p 1`. **Without that variable every integration test skips silently and a skip reads as a pass.** Any claim about an integration test must name the DSN it ran with.
- Commits: conventional, single line, no signature, no `Co-Authored-By` trailer, no emoji.
- Stage with explicit paths (`git add <path>`). Never `git add -A`.
- Do **not** change the `Action` string `"downgrade_blocked_over_quota"` — it is constrained by the `spca_action_check` DB check constraint and keyed on by two integration tests.
- `subscription_plan_change_audit` is append-only at the DB level: migration 050 does `REVOKE UPDATE, DELETE ... FROM PUBLIC`. Insert only; never update or delete a row.
- Work only inside this worktree: `.claude/worktrees/397-blocked-downgrade-audit`.

## Verified findings (established before planning — do not re-litigate)

Each of these was checked at the source, not taken from the issue text.

1. **The mechanism is real.** `internal/subscription/planchange/downgrade.go:41` writes the blocked row via `WritePlanChangeAuditRowTx(ctx, tx, ...)`; `downgrade.go:70` then returns `ErrStoreCountOverQuota`. That error reaches the closure passed to `subscription.WithAdvisoryLock` (`planchange.go:155`, returned at `planchange.go:197`), whose body is `db.WithContext(ctx).Transaction(...)` (`internal/subscription/advisory_lock.go:16`). GORM rolls back on a non-nil callback return, discarding the row.

2. **Reproduced by execution, not inferred.** `TestExecute_Downgrade_StudioToStarter_OverQuota_Rejected` (`planchange_integration_test.go:232-281`) already asserts the row persists, and **fails today**:
   ```
   planchange_integration_test.go:280: Not equal: expected: 1, actual: 0
   ```
   Run with `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`, `go test -tags=integration -p 1 -count=1 ./internal/subscription/planchange/`.

3. **That is the package's ONLY failing test.** The full package run produces exactly one `--- FAIL`, and it is this one. The handoff's "known pre-existing failure" entry for `internal/subscription/planchange` *is* this bug — not an unrelated failure. So fixing it makes the package green, which is what makes Task 3 (wiring it into `make test-int`) safe.

4. **The error return is discarded** — `downgrade.go:41` is `_ = WritePlanChangeAuditRowTx(...)`. Its sibling at `downgrade.go:92` checks the error. Even with the rollback fixed, an insert failure would stay silent. In scope for this fix.

5. **The trail is not totally blank today, and the commit message must say so accurately.** `downgrade.go:55-67` also calls `o.deps.Emitter.EmitPlanChange`, and `audit.Emitter.Emit` queues to a background worker writing on the emitter's own handle with a fresh context (`internal/audit/emitter.go:235-243`), so an `audit_logs` row *does* survive. Only the `subscription_plan_change_audit` row is lost. That emitter path is best-effort — `Emit` drops on a full queue (`emitter.go:152-158`) — so it is not a substitute.

6. **Scope audit — this is the only affected `WritePlanChangeAuditRowTx` call site.** All four non-test call sites were enumerated and traced: `downgrade.go:41` (rolled back — the defect), `downgrade.go:92` (success path, returns nil), `planchange.go:358` (success path), `cron.go:220` (returns nil), `cron.go:279` (the cron's *blocked* path — survives, because `blockAndNotify` returns nil at `cron.go:327`). The cron's blocked path is the correct pattern and proves the interactive path is the outlier. This is genuinely a one-site defect.

7. **One adjacent site with the same shape, deliberately OUT of scope:** `internal/arbitrage/recorder.go:107` inserts an arbitrage audit row inside a transaction that can return non-nil at `recorder.go:115` and `recorder.go:118`, rolling the row back. Same mechanism, different package and a different judgement call (the row FK-references the subscription). File it as a separate issue; do not fix it here.

8. **No non-Tx sibling helper exists.** `WritePlanChangeAuditRowTx` is the only writer and takes any `*gorm.DB`, so passing `o.deps.DB` writes outside the transaction. `auditlog_test.go:43` already calls it with a plain `db`, so that usage is established.

9. **Constraint on the fix shape.** The store-count read `CountActiveOrSoftDeletedRestorableTx(ctx, tx, ...)` (`downgrade.go:33`) must stay on `tx` — it is deliberately tx-scoped so the count sees uncommitted rows under the lock (documented at `downgrade.go:18-22`). Writing on `o.deps.DB` *while the tx is still open* would take a second pooled connection under the advisory lock, against a 5-max-open pool. Therefore the write must happen **after** `WithAdvisoryLock` returns, not inline. All data the row needs (`sub`, `in`, `auditCurrency`) is already plain in-memory Go values, so deferring is mechanically safe.

10. **Accepted trade-off, to be stated in the PR:** a crash between the rollback and the deferred write leaves no row. Since the operation is a refusal that mutated nothing, that is strictly better than today's *guaranteed* loss.

## File Structure

- `internal/subscription/planchange/downgrade.go` — stop writing the blocked row inside `tx`; return it to the caller instead.
- `internal/subscription/planchange/planchange.go` — in `Execute`, write the deferred row after `WithAdvisoryLock` returns, and surface a write failure.
- `internal/subscription/planchange/planchange_integration_test.go` — extend the existing test to pin field content, not just row count.
- `Makefile` (repo root) — add `./internal/subscription/planchange/...` to `test-int` so the guard actually runs.

---

### Task 1: Move the blocked-audit write outside the advisory-lock transaction

**Files:**
- Modify: `services/marketplace-api/internal/subscription/planchange/downgrade.go:24-71`
- Modify: `services/marketplace-api/internal/subscription/planchange/planchange.go:144-204`
- Test: `services/marketplace-api/internal/subscription/planchange/planchange_integration_test.go:232-281` (existing, currently failing)

**Interfaces:**
- Consumes: `WritePlanChangeAuditRowTx(ctx context.Context, db *gorm.DB, row PlanChangeAuditRow) error` (`auditlog.go:46`), `PlanChangeAuditRow` (`auditlog.go:17-38`), `buildAuditCurrency(sub *subscription.StoreSubscription) string` (`downgrade.go:130`).
- Produces: `executeDowngradeSchedule` gains a third return value `deferredAudit *PlanChangeAuditRow` — a row the caller must write **after** the advisory-lock transaction unwinds. Nil means nothing deferred. Signature becomes:
  ```go
  func (o *Orchestrator) executeDowngradeSchedule(
      ctx context.Context, tx *gorm.DB, in Input, sub *subscription.StoreSubscription,
  ) (Output, *PlanChangeAuditRow, error)
  ```

- [ ] **Step 1: Confirm the existing test fails for the right reason (this is the RED step — the test already exists)**

Run, from `services/marketplace-api`:

```bash
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  -run 'TestExecute_Downgrade_StudioToStarter_OverQuota_Rejected' \
  ./internal/subscription/planchange/
```

Expected: `FAIL`, with `planchange_integration_test.go:280: Not equal: expected: 1, actual: 0`.

If it instead reports `ok` in under ~1s with no assertions, `TEST_DATABASE_URL` did not take effect and the test skipped — stop and escalate. A skip is not a pass.

- [ ] **Step 2: Change `executeDowngradeSchedule` to return the blocked row instead of writing it**

In `downgrade.go`, change the signature to return `(Output, *PlanChangeAuditRow, error)`.

Replace the blocked-path write (currently `downgrade.go:41-53`, the `_ = WritePlanChangeAuditRowTx(ctx, tx, PlanChangeAuditRow{...})` call) with construction of the row and returning it. The blocked branch becomes:

```go
if storeLimit != plangate.Unlimited && count > storeLimit {
    // The audit row must OUTLIVE this transaction: we are about to return
    // ErrStoreCountOverQuota, and WithAdvisoryLock rolls the tx back on any
    // non-nil return — which would discard a row written on tx here (#397).
    // Hand it to Execute, which writes it on the pooled handle after the
    // lock is released.
    auditCurrency := buildAuditCurrency(sub)
    blocked := &PlanChangeAuditRow{
        TenantID:        in.TenantID,
        StoreID:         in.StoreID,
        FromPlan:        sub.Plan,
        ToPlan:          in.TargetPlan,
        FromPeriod:      sub.SubscriptionPeriod,
        ToPeriod:        in.TargetPeriod,
        Action:          "downgrade_blocked_over_quota",
        BillingCurrency: auditCurrency,
        Actor:           in.Actor,
        Reason:          in.Reason,
        EffectiveAt:     in.Now,
    }

    if o.deps.Emitter != nil {
        o.deps.Emitter.EmitPlanChange(in.GinCtx, audit.PlanChange{
            TenantID:    in.TenantID,
            StoreID:     in.StoreID,
            FromPlan:    string(sub.Plan),
            ToPlan:      string(in.TargetPlan),
            FromPeriod:  string(sub.SubscriptionPeriod),
            ToPeriod:    string(in.TargetPeriod),
            Subaction:   "downgrade_blocked_over_quota",
            Actor:       in.Actor,
            Reason:      in.Reason,
            EffectiveAt: in.Now,
        })
    }

    return Output{}, blocked, ErrStoreCountOverQuota
}
```

Then update every other `return` in the function to carry a `nil` in the new middle position — the count-error return near the top, the `SetPendingDowngrade` error return, the `downgrade_scheduled` audit-write error return, and the final success return. Do **not** move the `downgrade_scheduled` write at `downgrade.go:92`: it is on the success path, commits with the tx, and must stay transactional.

- [ ] **Step 3: Write the deferred row in `Execute`, after the lock is released**

In `planchange.go`, declare the carrier alongside `out` before the closure and populate it at the downgrade call site:

```go
var out Output
var deferredAudit *PlanChangeAuditRow
err := subscription.WithAdvisoryLock(ctx, o.deps.DB, in.StoreID, func(tx *gorm.DB) error {
```

At the downgrade branch (currently `planchange.go:194-201`):

```go
        // Downgrade or period downgrade.
        o2, blocked, err := o.executeDowngradeSchedule(ctx, tx, in, sub)
        if blocked != nil {
            deferredAudit = blocked
        }
        if err != nil {
            return err
        }
        out = o2
        return nil
    })
```

Then, after `WithAdvisoryLock` returns and **before** the existing `if err != nil` return, write the deferred row on the pooled handle:

```go
    // Written outside WithAdvisoryLock deliberately: this row records a
    // REFUSAL, and the transaction that refuses is rolled back, so a row
    // written inside it would never persist (#397). The lock is released by
    // now, so this does not hold a second pooled connection under it.
    if deferredAudit != nil {
        if auditErr := WritePlanChangeAuditRowTx(ctx, o.deps.DB, *deferredAudit); auditErr != nil {
            return Output{}, fmt.Errorf(
                "planchange: write blocked downgrade audit row: %w (original refusal: %v)",
                auditErr, err)
        }
    }
    if err != nil {
        return Output{}, err
    }
    return out, nil
```

Note the error return no longer discards the failure — finding 4. The original refusal is preserved in the message so a failed audit write cannot mask why the downgrade was refused.

- [ ] **Step 4: Build and vet**

Run, from `services/marketplace-api`:

```bash
go build ./... && go vet ./... && go vet -tags=integration ./...
```

Expected: all three clean, exit 0. `go vet -tags=integration ./...` is what compiles the integration test files — if the signature change broke a test file, this is where it surfaces.

- [ ] **Step 5: Run the test to verify it now passes**

```bash
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  -run 'TestExecute_Downgrade_StudioToStarter_OverQuota_Rejected' \
  ./internal/subscription/planchange/
```

Expected: `PASS`. Report the elapsed time — a sub-second `ok` with no test run means it skipped.

- [ ] **Step 6: MUTATION TEST — prove the test actually pins the fix**

A green suite proves nothing on its own; the question is whether it stays green with the fix deleted.

Temporarily revert Step 3's deferred write by commenting out the `if deferredAudit != nil { ... }` block in `planchange.go`. Re-run the command from Step 5.

Expected: **FAIL** with `expected: 1, actual: 0`.

If it PASSES, the test is decoration — stop and escalate rather than continuing.

Then restore the block and re-run Step 5 to confirm `PASS` again. Record both outcomes in your report.

- [ ] **Step 7: Run the whole package to confirm no regression**

```bash
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/subscription/planchange/
```

Expected: `ok`, zero `--- FAIL` lines. The pre-change baseline for this package was exactly one failure (the test above), so anything else failing is yours and must not be dismissed as pre-existing.

Also run the unit suite:

```bash
go test ./... -count=1
```

Expected: no new failures versus `main`.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/downgrade.go \
        services/marketplace-api/internal/subscription/planchange/planchange.go
git commit -m "fix(planchange): write the blocked-downgrade audit row outside the transaction that refuses the downgrade (#397)"
```

---

### Task 2: Pin the row's CONTENT, not just its existence

**Files:**
- Modify: `services/marketplace-api/internal/subscription/planchange/planchange_integration_test.go:232-281`

**Interfaces:**
- Consumes: `PlanChangeAuditRow` and the `subscription_plan_change_audit` table from Task 1.
- Produces: nothing later tasks depend on.

Rationale: the existing assertion is `Count(&count)` and `require.Equal(t, int64(1), count)`. That pins existence only. A fix that wrote a row with the wrong plan, actor, or currency would still pass — and the value of this row to ops is entirely in its fields.

- [ ] **Step 1: Extend the assertion to read the row back**

In `TestExecute_Downgrade_StudioToStarter_OverQuota_Rejected`, after the existing count assertion, add:

```go
	// The row's VALUE is the point — ops reads these fields to see why the
	// downgrade was refused. Pinning only the count would let a row with the
	// wrong plan or actor pass (#397).
	var blocked struct {
		FromPlan        string
		ToPlan          string
		Actor           string
		BillingCurrency string
	}
	require.NoError(t, db.Table("subscription_plan_change_audit").
		Select("from_plan, to_plan, actor, billing_currency").
		Where("store_id = ? AND action = ?", storeID, "downgrade_blocked_over_quota").
		Scan(&blocked).Error)

	require.Equal(t, "studio", blocked.FromPlan)
	require.Equal(t, "starter", blocked.ToPlan)
	require.NotEmpty(t, blocked.Actor, "the blocked row must record who attempted the downgrade")
```

Before writing the literals, read the test's own setup block (`planchange_integration_test.go:232-260`) and use the exact plan and actor values it seeds. If the seeded actor is empty, keep the `NotEmpty` assertion off and assert the plans only — do not invent a value to make an assertion pass.

- [ ] **Step 2: Run the test**

```bash
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  -run 'TestExecute_Downgrade_StudioToStarter_OverQuota_Rejected' \
  ./internal/subscription/planchange/
```

Expected: `PASS`.

- [ ] **Step 3: MUTATION TEST the new assertions**

Temporarily change `Action: "downgrade_blocked_over_quota"` in `downgrade.go`'s blocked row to `ToPlan: "enterprise"` (i.e. corrupt one asserted field, leaving the action string alone so the `WHERE` still matches). Re-run Step 2.

Expected: **FAIL** on the `blocked.ToPlan` assertion.

Restore, re-run, confirm `PASS`. If corrupting the field did not fail the test, the assertion is not wired to what it claims to check — escalate.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/planchange_integration_test.go
git commit -m "test(planchange): pin the blocked-downgrade audit row's fields, not just its existence (#397)"
```

---

### Task 3: Wire the package into `make test-int` so the guard cannot silently stop running

**Files:**
- Modify: `Makefile` (repo root), the `test-int` package list at lines 86-107

**Interfaces:**
- Consumes: a fully green `./internal/subscription/planchange/` from Tasks 1-2.
- Produces: nothing later tasks depend on.

Rationale: this defect survived because the package that tests it is not in the runner. A correct test existed and asserted the right thing for as long as the bug has existed; it never ran. Fixing the code without fixing that leaves the next regression equally invisible.

- [ ] **Step 1: Confirm the package is currently absent and green**

```bash
grep -n "planchange" Makefile
```

Expected: no match in the `test-int` list. (`./internal/subscription` is listed non-recursively and `./internal/subscription/cancel/...` etc. are listed individually; `planchange` is not among them.)

Then confirm it is green after Tasks 1-2:

```bash
cd services/marketplace-api && \
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/subscription/planchange/
```

Expected: `ok`, zero failures. **If anything fails, stop — do not add a red package to the target.** Report what failed instead.

- [ ] **Step 2: Add the package to the list**

In `Makefile`, in the `test-int` target's package list, add the line immediately after `./internal/subscription/lifecycle/... \`:

```
	    ./internal/subscription/planchange/... \
```

Keep the existing indentation (a tab, then four spaces) and the trailing backslash exactly as the neighbouring lines have them.

- [ ] **Step 3: Verify the target still parses and the package now runs**

```bash
make -n test-int | grep planchange
```

Expected: the new path appears in the expanded command.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "test: run internal/subscription/planchange in test-int so the #397 guard cannot stop running"
```

---

### Task 1b: Remediate the two review findings on Task 1

Raised by the scoped review of `045ec8dc`. Both are in the same 8-line block at `planchange.go:205-215`. Neither undermines Task 1's core change; both are cheap and verified.

**Files:**
- Modify: `services/marketplace-api/internal/subscription/planchange/planchange.go` (the `if deferredAudit != nil { ... }` block)
- Test: `services/marketplace-api/internal/handlers/admin/subscription_change_plan_test.go`

**Interfaces:**
- Consumes: `ErrStoreCountOverQuota` (`planchange` package), `mapChangePlanErr` (`internal/handlers/admin/subscription_change_plan.go:130-147`).
- Produces: nothing later tasks depend on.

**Finding 1 (should-fix) — a failed audit write turns a 422 into a 500.** Verified: `subscription_change_plan.go:136` maps `errors.Is(err, planchange.ErrStoreCountOverQuota)` to **HTTP 422** `store_count_over_quota`; `subscription_change_plan.go:141-146` maps everything else to **HTTP 500** `internal`. Task 1's new return wraps `auditErr` with `%w` and demotes the refusal to `%v`, so `ErrStoreCountOverQuota` leaves the chain and a legitimately-over-quota merchant gets an opaque 500. Worse, the conditions that make the audit insert fail (pool pressure, cancelled context) are exactly when a merchant is likely to hit this. The frontend keys on the `store_count_over_quota` slug, not the status alone.

**Finding 2 (should-fix) — the deferred write uses the request context.** `WritePlanChangeAuditRowTx` does `tx.WithContext(ctx).Create(&row)` (`auditlog.go:52`) and `ctx` is `c.Request.Context()` (`subscription_change_plan.go:79`), cancelled by `net/http` on client disconnect. No timeout middleware wraps it. So a disconnect between the rollback and the insert loses the row — the very outcome this fix exists to prevent, in a narrower window. The codebase already documents the remedy for this exact problem at `internal/audit/emitter.go:186-190` (`EmitSync`): *"A client disconnecting mid-purge must not cancel the record of what was destroyed."*

- [ ] **Step 1: Apply both fixes**

Replace the `if deferredAudit != nil { ... }` block in `planchange.go` with:

```go
	if deferredAudit != nil {
		// Fresh context: the refusal has already happened, and the client
		// disconnecting must not cancel the record of it. Matches
		// audit.Emitter.EmitSync (internal/audit/emitter.go:186). WithoutCancel
		// keeps tracing values while dropping cancellation.
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if auditErr := WritePlanChangeAuditRowTx(auditCtx, o.deps.DB, *deferredAudit); auditErr != nil {
			// errors.Join, NOT %w on auditErr: the caller maps
			// ErrStoreCountOverQuota to a 422, and dropping it from the chain
			// would turn a legitimate quota refusal into an opaque 500.
			return Output{}, errors.Join(err,
				fmt.Errorf("planchange: write blocked downgrade audit row: %w", auditErr))
		}
	}
```

Add `"errors"` and `"time"` to the file's imports if not already present (`context` already is).

- [ ] **Step 2: Build and vet**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./...
```

Expected: clean, exit 0.

- [ ] **Step 3: Write a failing handler test for the 422 mapping**

`TestChangePlan_OverQuota_Returns422` (`subscription_change_plan_test.go:131-141`) uses a `fakeOrch` returning a *bare* `ErrStoreCountOverQuota`, so it cannot catch this. Add a sibling that returns the **wrapped** form. Read the existing test and its `fakeOrch` first, and match their construction exactly:

```go
// A failed audit write must not change what the merchant is told: the
// downgrade was still refused for quota, and the frontend keys on the
// store_count_over_quota slug (#397 review finding 1).
func TestChangePlan_OverQuota_AuditWriteFailed_Still422(t *testing.T) {
	wrapped := errors.Join(
		planchange.ErrStoreCountOverQuota,
		fmt.Errorf("planchange: write blocked downgrade audit row: %w", errors.New("boom")),
	)
	// ... build the handler with a fakeOrch returning `wrapped`, mirroring
	// TestChangePlan_OverQuota_Returns422's setup exactly ...
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "store_count_over_quota")
}
```

- [ ] **Step 4: Run the test**

```bash
cd services/marketplace-api
go test ./internal/handlers/admin/ -count=1 -run 'TestChangePlan_OverQuota' -v 2>&1 | tail -20
```

Expected: both tests PASS.

- [ ] **Step 5: MUTATION TEST both fixes**

*Finding 1:* temporarily change `errors.Join(err, ...)` back to the original `fmt.Errorf("...: %w (original refusal: %v)", auditErr, err)`. Re-run Step 4.
Expected: `TestChangePlan_OverQuota_AuditWriteFailed_Still422` **FAILS** with 500 instead of 422. Restore and confirm PASS.

*Finding 2:* temporarily revert `auditCtx` to plain `ctx`, then write a test-free check: confirm by reading that `WritePlanChangeAuditRowTx` receives a cancellable context. A behavioural test for disconnect is not worth building here — if you cannot construct one cheaply, say so plainly in your report rather than fabricating weak coverage. Restore.

Report both mutation outcomes verbatim.

- [ ] **Step 6: Re-run the integration test and the package**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/subscription/planchange/
go test ./... -count=1
```

Expected: `ok`, zero failures; no new unit failures.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/planchange.go \
        services/marketplace-api/internal/handlers/admin/subscription_change_plan_test.go
git commit -m "fix(planchange): keep ErrStoreCountOverQuota in the chain and shield the deferred audit write from request cancellation (#397)"
```

---

## Self-Review


**Spec coverage.** Issue #397's mechanism → Task 1. Its "suggested fix" (write outside the rolled-back transaction) → Task 1 Step 3, using the "restructure so the write happens after" variant, chosen over an inline second connection for the pool reason in finding 9. Finding 4 (discarded error) → Task 1 Step 3's error handling. Findings 2-3 (a real failing test, and it being the package's only failure) → Tasks 1 and 3. Finding 7 (arbitrage sibling) is explicitly out of scope and routed to a new issue.

**Placeholder scan.** No TBDs. Every code step carries the actual code. The one judgement call left to the implementer — the exact seeded plan/actor literals in Task 2 Step 1 — is bounded by an explicit instruction to read the fixture rather than invent values.

**Type consistency.** `executeDowngradeSchedule`'s new three-value signature is declared once in Task 1's Interfaces block and used consistently in Steps 2 and 3. `PlanChangeAuditRow` is passed by value to `WritePlanChangeAuditRowTx` (matching `auditlog.go:46`) and held as `*PlanChangeAuditRow` in the carrier. `deferredAudit` is the same name in both files' snippets.
