# #396 — Tax revalidation cron: remove the cross-connection lock wait

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop `revalidation.Cron.Run` holding a transaction open across a call that writes the same row on a different pooled connection, so the cron completes instead of hanging forever — and restore `./internal/billing/tax/...` to the integration test runner.

**Architecture:** The cron's transaction exists only to scope `pg_advisory_xact_lock`, but it ends up wrapping the entire pass — including `Svc.Submit`, which opens its own transaction on a second connection and updates the same `store_subscriptions` row. We replace the transaction-scoped lock with a **session-scoped** lock (`pg_try_advisory_lock`) held on one dedicated connection, and let each unit of work run as its own short, auto-committed statement on the pool. No transaction is held across `Submit`, so the wait disappears.

**Tech Stack:** Go 1.26, GORM, `database/sql`, Postgres advisory locks, robfig/cron.

**Spec:** GitHub issue tesserix/mark8ly#396, plus the verified findings below.

## Global Constraints

- Run all Go commands from `services/marketplace-api`, never path-scoped, always `-count=1`.
- Required command set: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`. `go vet -tags=integration ./...` is the only command that compiles build-tagged files.
- Integration tests: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`, `-p 1`. **Without that variable they skip silently and a skip prints `ok`.** Any claim about an integration test must name the DSN.
- **ALWAYS pass an explicit `-timeout` when running the revalidation package.** Before the fix these tests *hang* rather than fail. Use `-timeout 90s` so a hang surfaces as a panic with a goroutine dump in about a minute instead of blocking for the 10-minute default.
- Commits: conventional, single line, no signature, no `Co-Authored-By` trailer, no emoji.
- Stage with explicit paths (`git add <path>`). Never `git add -A`.
- Work only inside this worktree: `.claude/worktrees/396-revalidation-deadlock`.

## Verified findings (established before planning — do not re-litigate)

1. **The mechanism is confirmed, same table, same row.**
   - `cron.go:78-81` — `c.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {` then `tx.Exec("SELECT pg_advisory_xact_lock(hashtext('tax_revalidation_cron'))")`. The whole pass runs inside this one transaction.
   - `cron.go:142-146` — `processOne` runs `UPDATE store_subscriptions SET revalidation_attempted_at = now() WHERE tenant_id = ? AND store_id = ?` on `tx`, taking a row lock held for the rest of the pass.
   - `cron.go:151-158` — `processOne` then calls `c.Svc.Submit(ctx, ...)` with the same tenant/store. `Submit` takes no `tx` (`service.go:67`).
   - `service.go:134-143` — `Submit` calls `subscription.WithAdvisoryLock(ctx, s.cfg.DB, in.StoreID, ...)` on the **pool handle**, which opens its own transaction (`advisory_lock.go:16`) on a second connection and runs `UPDATE store_subscriptions ... WHERE tenant_id = ? AND store_id = ?` — the same row.
   - The inner connection blocks on the outer transaction's uncommitted row lock; the outer is idle-in-transaction (blocked in Go, not in Postgres), so there is no cycle for the deadlock detector to see. Unbounded hang, no error.

2. **The two advisory-lock keys do NOT collide.** Outer is `hashtext('tax_revalidation_cron')`; inner is `hashtext(storeID)`. The blocking point is purely the row lock at `service.go:136`, not the advisory lock. Do not "fix" this by changing lock keys.

3. **Only the still-valid path hangs.** `Submit` returns early with no DB write for `ErrValidatorDisabled` (`service.go:84`), `ErrRegistryUnavailable` (`:87`), `ErrInvalidFormat`/`ErrNotFound` (`:97`); the SEA path writes different tables. The hang requires `res.Valid == true`, which is the common production case and is exercised by exactly one test.

4. **Production wiring is worse than the issue says.** `main.go:1880-1890` builds the cron and registers it with `trialScheduler.AddFunc(revalidation.Spec, ...)` passing `workerCtx` — it does **not** use `revalidation.Register` (`cron.go:59-67`), which wraps `Run` in a 30-minute `context.WithTimeout`. So in production the hang is **indefinite**, not 30-minutes-then-error, and each subsequent daily fire blocks behind the wedged transaction, leaking a goroutine and a connection per day until the pod restarts. Fixing the wiring is in scope (Task 2).

5. **"Has never run successfully" is overstated — say so accurately in the PR.** A pass over rows that all fail definitively or hit registry outages completes cleanly. The cron wedges the first time a stale row's tax ID is **still valid**. Given the 90-day staleness window and the cron's age, that has almost certainly happened, but it is a data-dependent inference, not a verified fact about production.

6. **The Makefile already documents this exact bug.** `Makefile:69-76` explains that `./internal/billing/tax` is deliberately not `./internal/billing/tax/...` because `revalidation` "would hang instead of failing", and says: *"do not add the ellipsis back until that deadlock is fixed."* Task 3 is what earns the right to add it back. The same missing ellipsis also excludes `internal/billing/tax/seaqueue`, whose status was never measured — Task 3 must measure it before including it.

7. **The blast radius has a concrete mechanism.** `pkg/testdb/testdb.go` truncates `store_subscriptions` on setup and in `t.Cleanup`. `TRUNCATE` needs `ACCESS EXCLUSIVE`, which queues behind the wedged transaction's row lock — so under `-p 1` every later package that truncates that table hangs too. That is why the issue reports `billing/trial`, `campaignbudget` and `category` timing out.

8. **A second, quieter defect the restructure also fixes.** `cron.go:141` calls the stamp a *"CAS-like sentinel: stamp attempted_at first so retries skip this row."* Inside a pass-long transaction the stamp is invisible to anything outside until the whole pass commits, so it cannot serve that purpose. Per-row auto-committed statements make the comment true.

9. **No `SetMaxOpenConns` is set anywhere in the service** (grepped; zero non-test hits), so `database/sql` defaults to unlimited max-open and 2 idle. Dedicating one connection to hold the session lock is therefore safe. Do not repeat the "5 max open" figure from the workspace CLAUDE.md as if it were verified in this code — it is not set here.

10. **Only one test reaches the bug.** `cron_test.go:55 TestRevalidation_StillValid_NoStateChange` uses `FakeValidator{Result: {Valid: true}}` and is the only test that reaches `service.go:136`. It does not fail — it stalls until the package timeout. The other three (`:74`, `:104`, `:119`) use `ErrNotFound`/`ErrRegistryUnavailable` and pass. `tests/integration/tax_validation_criteria_test.go:81` also calls `Run` but only with `ErrNotFound`, so it is safe today and is one fixture change away from wedging.

11. **Deliberate trade-off to state in the PR.** Today the whole pass is one atomic transaction. After the fix, a failure mid-pass leaves earlier rows' work committed. That is the *correct* semantics for a cron sweeping independent rows — and it is what finding 8's sentinel comment already assumes — but it is a real change and must be named, not glossed.

## File Structure

- `internal/billing/tax/revalidation/cron.go` — replace the pass-long transaction + xact lock with a session lock on a dedicated connection; run each unit of work on the pool.
- `cmd/marketplace-api/main.go` — register the cron via `revalidation.Register` so the 30-minute timeout actually applies.
- `Makefile` (repo root) — restore the `/...` on `./internal/billing/tax` and delete the now-false comment.

---

### Task 1: Hold the cron lock on its own connection instead of a pass-long transaction

**Files:**
- Modify: `services/marketplace-api/internal/billing/tax/revalidation/cron.go` — `Run` (lines 69-88), `recheckStaleValidations` (96-135), `processOne` (137-205), `unpublishAfter14Days` (209-243)
- Test: `services/marketplace-api/internal/billing/tax/revalidation/cron_test.go` (existing; currently hangs)

**Interfaces:**
- Consumes: `c.DB *gorm.DB`, `c.Svc *tax.Service` with `Submit(ctx, tax.SubmitInput) error` (`service.go:67`).
- Produces: the four methods change their `tx *gorm.DB` parameter to a plain `db *gorm.DB` (the pool handle). Signatures become:
  ```go
  func (c *Cron) Run(ctx context.Context) error
  func (c *Cron) recheckStaleValidations(ctx context.Context, db *gorm.DB) error
  func (c *Cron) processOne(ctx context.Context, db *gorm.DB, r staleRow)
  func (c *Cron) unpublishAfter14Days(ctx context.Context, db *gorm.DB) error
  ```
  No caller outside this file uses the three unexported ones; `Run`'s signature is unchanged.

- [ ] **Step 1: Confirm the test HANGS today (this is the RED step — note it hangs, it does not fail)**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -timeout 90s \
  -run 'TestRevalidation_StillValid_NoStateChange' \
  ./internal/billing/tax/revalidation/ 2>&1 | tail -30
```

Expected: **`panic: test timed out after 1m30s`** with a goroutine dump, NOT a normal assertion failure. Look in the dump for a goroutine blocked in the `Submit` → `WithAdvisoryLock` → `Exec` path — that is the second connection waiting on the row lock.

If it instead passes in under a second, `TEST_DATABASE_URL` did not take effect and the test skipped — stop and escalate. A skip is not a pass.

- [ ] **Step 2: Rewrite `Run` to take a session-scoped lock on a dedicated connection**

Replace the body of `Run` (everything from `return c.DB.WithContext(ctx).Transaction(` to the closing of that call) with:

```go
	// The cron lock is SESSION-scoped and held on its own dedicated
	// connection, NOT pg_advisory_xact_lock inside a pass-long transaction
	// (#396). The old shape kept a transaction open across c.Svc.Submit,
	// which writes the same store_subscriptions row on a second pooled
	// connection — that write blocked on this transaction's uncommitted row
	// lock while this transaction waited for Submit to return. Postgres saw
	// one waiter and one idle-in-transaction session, not a cycle, so the
	// deadlock detector never fired and the cron hung forever.
	sqlDB, err := c.DB.DB()
	if err != nil {
		return fmt.Errorf("revalidation: db handle: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("revalidation: dedicated conn: %w", err)
	}
	defer conn.Close()

	var acquired bool
	if err := conn.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock(hashtext('tax_revalidation_cron'))`,
	).Scan(&acquired); err != nil {
		return fmt.Errorf("revalidation: advisory lock: %w", err)
	}
	if !acquired {
		// Another pass is still running. Skipping is correct for a daily
		// sweep: blocking would just queue passes behind each other.
		slog.Info("revalidation: another pass holds the cron lock, skipping")
		return nil
	}
	defer func() {
		// WithoutCancel: the lock must be released even if ctx is done,
		// otherwise it is held until this connection is closed.
		if _, err := conn.ExecContext(context.WithoutCancel(ctx),
			`SELECT pg_advisory_unlock(hashtext('tax_revalidation_cron'))`); err != nil {
			slog.Warn("revalidation: advisory unlock", "err", err)
		}
	}()

	// Each unit of work below runs as its own auto-committed statement on the
	// pool. Nothing holds a transaction across Submit.
	if err := c.recheckStaleValidations(ctx, c.DB); err != nil {
		return err
	}
	return c.unpublishAfter14Days(ctx, c.DB)
```

Keep the `c.Now` and `c.BatchSize` defaulting at the top of `Run` exactly as it is.

- [ ] **Step 3: Change the three helpers to take the pool handle**

Rename the `tx *gorm.DB` parameter to `db *gorm.DB` in `recheckStaleValidations`, `processOne` and `unpublishAfter14Days`, and update every use inside them (`tx.WithContext(...)` → `db.WithContext(...)`, `tx.Raw(...)` → `db.Raw(...)`). Update the `c.processOne(ctx, tx, r)` call inside `recheckStaleValidations` to pass `db`.

Do **not** change any SQL text, the `staleRow` struct, the batching, the notifier block, or the audit blocks. This step is a parameter rename plus the call-site update — nothing else.

Also update `recheckStaleValidations`'s doc comment if it mentions running inside the pass transaction.

- [ ] **Step 4: Build and vet**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./...
```

Expected: clean, exit 0. Confirm `context`, `fmt` and `log/slog` are imported in `cron.go` (they should already be; add any that are missing).

- [ ] **Step 5: Run the previously-hanging test**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -timeout 90s \
  -run 'TestRevalidation_StillValid_NoStateChange' -v \
  ./internal/billing/tax/revalidation/ 2>&1 | tail -20
```

Expected: **`--- PASS`**, completing in seconds. Its assertions (`tax_id_validated` still true, `revalidation_attempted_at` stamped) must hold — the fix must not have skipped the work.

- [ ] **Step 6: Run the whole revalidation package**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -timeout 120s -v \
  ./internal/billing/tax/revalidation/ 2>&1 | grep -E "^(--- |ok|FAIL|panic)" | head -20
```

Expected: four `--- PASS` lines, no `panic`, no `FAIL`.

- [ ] **Step 7: MUTATION TEST — prove the test actually pins the fix**

Temporarily restore the old shape: wrap the two calls at the end of `Run` back into `c.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { ... })` passing `tx` instead of `c.DB`, keeping the session lock in place. Re-run Step 5's command.

Expected: **the test hangs again** — `panic: test timed out after 1m30s`.

Restore the fix and re-run Step 5 to confirm `PASS`. Record both outcomes.

This mutation is the whole point: it proves the test is sensitive to the transaction shape, not merely to the code compiling.

- [ ] **Step 8: Prove the blast radius is gone**

Run revalidation together with two packages the issue named as collateral damage, in one `-p 1` run — the combination that used to hang:

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -timeout 300s \
  ./internal/billing/tax/... ./internal/campaignbudget/cron/... 2>&1 | tail -20
```

Expected: all packages report `ok` or `no test files`; nothing hangs. Report the wall-clock time.

Note: `./internal/billing/tax/seaqueue` is included by the `/...` here and its status was never measured (finding 6). If it fails, report the failure but do NOT treat it as caused by your change — check it at the base commit before drawing a conclusion:
```bash
git worktree add /private/tmp/base-396 aa7feaed --detach
```

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/revalidation/cron.go
git commit -m "fix(revalidation): hold the cron lock on its own connection so the pass never waits on its own row lock (#396)"
```

---

### Task 2: Register the cron so its 30-minute timeout actually applies

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go:1880-1892`

**Interfaces:**
- Consumes: `revalidation.Register(scheduler *robcron.Cron, c revalidation.Cron) (robcron.EntryID, error)` (`cron.go:59-67`), which already wraps `Run` in a 30-minute `context.WithTimeout(context.Background(), ...)` and logs failures.
- Produces: nothing later tasks depend on.

Rationale (finding 4): `Register` exists precisely to bound this cron's runtime, and production does not use it. Task 1 removes the hang's cause; this task restores the belt-and-braces bound so any *future* stall self-limits instead of leaking a goroutine and a connection per day.

- [ ] **Step 1: Replace the hand-rolled AddFunc with Register**

In `main.go`, inside the existing `if taxService != nil {` block, replace the `trialScheduler.AddFunc(revalidation.Spec, func() { ... })` call and its surrounding error handling with:

```go
		// Register (not a bare AddFunc) so the pass runs under revalidation's
		// own 30-minute timeout. Without it the cron inherits workerCtx, which
		// has no deadline — so a stalled pass would hang forever and every
		// later fire would queue behind it, leaking a goroutine and a
		// connection per day (#396).
		if _, err := revalidation.Register(trialScheduler, *revalidationCron); err != nil {
			log.Error("register tax revalidation cron", "err", err)
		}
```

Keep the `revalidationCron := &revalidation.Cron{DB: conn, Svc: taxService, Audit: auditEmitter}` construction above it unchanged.

Before writing this, **check `Register`'s parameter type**: it takes `revalidation.Cron` by value (`cron.go:60`), while `revalidationCron` is a `*revalidation.Cron`. Dereference as shown. If the signature differs from what this plan states, follow the compiler and say so in your report.

- [ ] **Step 2: Confirm the scheduler type matches**

`Register` expects `*robcron.Cron` (the `github.com/robfig/cron/v3` type). Verify `trialScheduler` is that type:

```bash
cd services/marketplace-api
grep -n "trialScheduler *:*=" cmd/marketplace-api/main.go | head
```

If `trialScheduler` is a different type or a wrapper, STOP and report — do not force a conversion.

- [ ] **Step 3: Build and vet**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./...
```

Expected: clean, exit 0.

- [ ] **Step 4: Confirm the cron is still registered exactly once**

```bash
cd services/marketplace-api
grep -n "revalidation\." cmd/marketplace-api/main.go
```

Expected: the `Cron{...}` construction and exactly one `revalidation.Register(...)`. There must be no leftover `AddFunc(revalidation.Spec, ...)` — a duplicate registration would run the pass twice per night.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "fix(revalidation): register the cron with its 30-minute timeout instead of the unbounded worker context (#396)"
```

---

### Task 3: Restore `./internal/billing/tax/...` to the integration runner

**Files:**
- Modify: `Makefile` (repo root) — the comment at lines 69-76 and the package entry at line 96

**Interfaces:**
- Consumes: a non-hanging `./internal/billing/tax/...` from Task 1.
- Produces: nothing.

- [ ] **Step 1: Measure `seaqueue`, which the missing ellipsis also excluded**

Finding 6: `internal/billing/tax/seaqueue` was never measured. Measure it before including it.

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -timeout 120s ./internal/billing/tax/seaqueue/ 2>&1 | tail -20
```

Record the result. If it **passes** (or has no test files), proceed to Step 2 and restore the full ellipsis. If it **fails**, do NOT add the ellipsis: instead list `./internal/billing/tax` and `./internal/billing/tax/revalidation/...` explicitly, leave `seaqueue` out, and rewrite the comment to say that `seaqueue` alone is now the reason — then report the failure so it can be filed as its own issue. Do not add a red package to the target.

- [ ] **Step 2: Update the package list**

In `Makefile`, change the line reading `	    ./internal/billing/tax \` to:

```
	    ./internal/billing/tax/... \
```

(or to the two explicit entries from Step 1's failure branch). Preserve the exact leading whitespace and trailing backslash.

- [ ] **Step 3: Replace the now-false comment**

Delete the comment block at `Makefile:69-76` (the paragraph beginning `@# ./internal/billing/tax below is deliberately NOT` and ending `so it stays out until someone does.`) and replace it with:

```
	@# ./internal/billing/tax/... is fully included again: the revalidation
	@# deadlock it used to hide was fixed in #396 — Cron.Run no longer holds a
	@# transaction open across Svc.Submit, so the pass cannot wait on its own
	@# row lock.
```

Leave every other comment in the target untouched — in particular the `./internal/subscription` paragraph, which is about `planchange` and is a different issue.

- [ ] **Step 4: Verify the target expands correctly**

```bash
make -n test-int | grep -o "\./internal/billing/tax[^ ]*"
```

Expected: `./internal/billing/tax/...` (or your Step 1 fallback entries). No bare `./internal/billing/tax` remaining.

- [ ] **Step 5: Run the tax subtree exactly as the target will**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -timeout 300s ./internal/billing/tax/... 2>&1 | tail -20
```

Expected: every package `ok` or `no test files`; no hang, no failure. **If anything fails or hangs, revert Step 2 and report** — do not leave the target red.

- [ ] **Step 6: Commit**

```bash
git add Makefile
git commit -m "test: restore ./internal/billing/tax/... to test-int now that the revalidation deadlock is fixed (#396)"
```

---

## Self-Review

**Spec coverage.** #396's mechanism → Task 1. Its "suggested fix" offers two options — pass `tx` into `Submit`, or make the two writes share one transaction. This plan takes a third, better shape: remove the long transaction entirely. Threading `tx` into `Submit` would force the tax service's public API to carry a transaction for one caller's benefit, and `Submit` already opens its own per-store advisory lock, which would then nest inside the cron's transaction — trading one lock-ordering hazard for another. Findings 4 and 11's production consequences → Task 2. Findings 6 and 7 (the runner exclusion and the blast radius) → Task 3 and Task 1 Step 8. Finding 8's sentinel bug is fixed as a side effect of Task 1 Step 3 and should be mentioned in the PR.

**Placeholder scan.** No TBDs. `Run`'s replacement body, the doc comment, the `main.go` replacement and the Makefile comment are all given verbatim. Two places deliberately hand the implementer a decision with an explicit rule for each branch — Task 2 Step 1 (`Register`'s parameter type: follow the compiler and report) and Task 3 Step 1 (`seaqueue` pass/fail: two spelled-out branches). Both are bounded choices with stated outcomes, not open questions.

**Type consistency.** `recheckStaleValidations`, `processOne` and `unpublishAfter14Days` all take `db *gorm.DB` after Task 1 and are called with `c.DB` from `Run` and with `db` from `recheckStaleValidations`. `Run(ctx context.Context) error` is unchanged, so `Register` and both test call sites keep compiling. `conn` is a `*sql.Conn` from `sqlDB.Conn(ctx)`, used only for the two lock statements.
