# #399 — Make `ApplyTrialRamp` idempotent against consumption

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop a re-run of the trial ramp re-inflating budget a merchant has already spent, on **both** transition days — day 4 and day 8 — without changing what a first run does.

**Architecture:** `GREATEST(remaining, N)` is a floor, so re-running raises a consumed balance back up. Rather than converting the ramp to delta arithmetic (which would change first-run behaviour and break an existing, correct test), we record which ramp step has been applied to a row and guard the UPDATE on it. The ramp stays a single atomic statement — the guard lives in the `WHERE` clause, so concurrent runs on multiple pods remain safe.

**Tech Stack:** Go 1.26, GORM, Postgres, golang-migrate, testify.

**Spec:** GitHub issue tesserix/mark8ly#399, plus the verified findings below.

## Global Constraints

- Run all Go commands from `services/marketplace-api`, never path-scoped, always `-count=1`.
- Required command set: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`.
- Integration tests: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`, `-p 1`. **Without that variable they skip silently and a skip prints `ok`.** Any claim about an integration test must name the DSN.
- **This change adds a migration. `ExpectedSchemaVersion` in `services/marketplace-api/migrations.go` MUST be bumped in the same commit.** It is currently `110`; the new migration is `000111` so it becomes `111`. `cmd/marketplace-api` refuses to start if the database's migration state does not match, so a migration without the bump is a broken deploy.
- The new migration must ship both `.up.sql` and `.down.sql`, matching the naming of its neighbours.
- Commits: conventional, single line, no signature, no `Co-Authored-By` trailer, no emoji.
- Stage with explicit paths (`git add <path>`). Never `git add -A`.
- Work only inside this worktree: `.claude/worktrees/399-trial-ramp-idempotency`.

## Verified findings (established before planning — do not re-litigate)

1. **Reproduced by execution, not inferred.** `TestApplyTrialRamp_Idempotent_ReRunSameDay` (`ramp_integration_test.go:78-99`) already encodes the issue's exact repro and **fails today**:
   ```
   ramp_integration_test.go:97: Not equal: expected: 1800, actual: 2000
     Messages: idempotent: re-running the ramp must not re-inflate remaining
   ```
   Run with `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`, `go test -tags=integration -p 1 -count=1 ./internal/campaignbudget/`.

2. **That is the package's ONLY failing test**, verified by a full-package run. So the fix makes `./internal/campaignbudget` green, which is what makes Task 3 safe.

3. **THE ISSUE IS UNDERSTATED: the defect is at TWO sites, not one.** The issue names only day 4. Day 8 has the identical shape:
   - `ramp.go:67-68` — `SET limit_set = GREATEST(remaining, 2000), remaining = GREATEST(remaining, 2000)`
   - `ramp.go:83-84` — `SET limit_set = $1, remaining = GREATEST(remaining, $1)` where `$1` is the plan allowance

   Day-8 repro: seed `remaining=1500`, ramp to allowance 5000, consume 3000 → 2000, re-run day 8 → `GREATEST(2000, 5000)` = 5000. **A fix touching only day 4 leaves the bug live.**

4. **The docstring is affirmatively wrong** (`ramp.go:48-50`): *"re-running on the same transition day with a smaller remaining uses GREATEST semantics so consumed balance is never re-inflated."* The reasoning is inverted — `GREATEST` is safe against re-inflation only when the second operand is a monotonic watermark (as in `internal/outbox/publisher.go:212`), not a constant ceiling. The inline comment at `ramp.go:63-64` repeats the error. Both must be corrected, not left to contradict the new behaviour.

5. **No outer idempotency guard exists — severity is NOT lowered.** Sole caller is `internal/campaignbudget/cron/jobs.go:87` inside `RunTrialRampOnce`, registered at `main.go:1988` on `trialScheduler` with spec `"CRON_TZ=UTC 0 0 * * *"`. Verified absent repo-wide: no advisory lock in the ramp path (contrast `billing/tax/revalidation/cron.go:79`, which does take one), no `job_runs`/`cron_runs` table, no "last ramp day applied" marker. `cron/jobs.go:2-3` claims *"Both jobs are pure idempotent functions of (db, now()) — safe to re-run, safe to run on multiple pods concurrently"* — true for `MonthlyReset`, false for the ramp. Multi-pod deployment is the realistic double-application path.

6. **A migration IS required.** `campaign_email_budget` is `(store_id, month, remaining, limit_set, updated_at)` with PK `(store_id, month)` and `CHECK (remaining >= 0)` — `migrations/000047_campaign_email_budget.up.sql:4-15`. There is **no** cumulative-granted column, and `limit_set` cannot serve as one because the ramp mutates it. No later migration alters this table.

7. **Every writer of `remaining` was enumerated.** `ramp.go:68` (defect), `ramp.go:83` (defect), `reserve.go:31` (`remaining = remaining - $1` guarded by `AND remaining >= $1` — correct), `recompute.go:42-45` (clamps down only — correct), `monthly_reset.go:34-46` (`ON CONFLICT DO NOTHING` — genuinely idempotent). Sibling quota code checked and clean: `campaignbudget/transactional/counter.go:33` and `outbox/publisher.go:212,220` are legitimate monotonic watermarks. **Only the two ramp sites are defective.**

8. **The two existing day-4 tests jointly define the required behaviour, and BOTH must keep passing:**
   - `TestApplyTrialRamp_Day3To4_RaisesToCeiling` (`ramp_integration_test.go:17-37`) seeds `remaining=50, limit_set=500` and asserts that after ramp both are `2000`. So a **first** run uses ceiling semantics.
   - `TestApplyTrialRamp_Idempotent_ReRunSameDay` (`:78-99`) asserts a **second** run changes nothing.

   Together: *apply the ceiling once, then never again.* This is why the fix is an applied-marker guard and **not** delta arithmetic — delta would make the first run yield `50 + 1500 = 1550` and break the first test. Do not "fix" that test to accommodate a delta design.

9. **`./internal/campaignbudget` is absent from `make test-int`.** The list includes `./internal/campaignbudget/cron/...` but not the parent package (`Makefile:87-107`), which is why a correct, failing test has sat unnoticed. Task 3 closes that.

10. **A separate defect found during verification — OUT of scope, to be filed, not fixed here.** `ramp.go:67` sets `limit_set = GREATEST(remaining, 2000)`, deriving the *ceiling* from the consumption-dependent `remaining`. Combined with `monthly_reset.go` seeding trial stores at **5000** (`monthly_reset.go` CTE, `('trial', 5000)`), a trial store's day-4 ramp computes `limit_set = GREATEST(5000, 2000) = 5000`, silently skipping the documented D4–7 = 2000 tier entirely. The seed also contradicts the migration comment's `D1-3=500` (`migrations/000047_campaign_email_budget.up.sql:18`). **This is a product decision about budget tiers, not an idempotency bug — do not change tier values in this plan.** File it as its own issue.

## File Structure

- `services/marketplace-api/migrations/000111_campaign_email_budget_ramp_step.up.sql` / `.down.sql` — add the applied-step column.
- `services/marketplace-api/migrations.go` — bump `ExpectedSchemaVersion` 110 → 111.
- `services/marketplace-api/internal/campaignbudget/models.go` — mirror the new column.
- `services/marketplace-api/internal/campaignbudget/ramp.go` — guard both UPDATEs; correct the docstring.
- `services/marketplace-api/internal/campaignbudget/ramp_integration_test.go` — add a day-8 idempotency test.
- `Makefile` — add `./internal/campaignbudget` to `test-int`.

---

### Task 1: Add the applied-step column and bump the schema version

**Files:**
- Create: `services/marketplace-api/migrations/000111_campaign_email_budget_ramp_step.up.sql`
- Create: `services/marketplace-api/migrations/000111_campaign_email_budget_ramp_step.down.sql`
- Modify: `services/marketplace-api/migrations.go` (`ExpectedSchemaVersion`)
- Modify: `services/marketplace-api/internal/campaignbudget/models.go`

**Interfaces:**
- Produces: column `campaign_email_budget.ramp_step_applied SMALLINT NOT NULL DEFAULT 0`, holding the highest ramp day already applied to the row (0 = none, then 4, then 8). Task 2's SQL guards on it.

- [ ] **Step 1: Write the up migration**

Create `migrations/000111_campaign_email_budget_ramp_step.up.sql`:

```sql
-- 000111 — §5.1 trial ramp idempotency (#399).
-- ramp_step_applied records the highest ramp transition day already applied to
-- this row (0 = none, 4 = D3->D4 applied, 8 = D7->D8 applied). ApplyTrialRamp
-- guards its UPDATE on it, so a re-run within the same UTC day (multi-pod
-- scheduling, retry, replay, backfill) cannot re-apply the GREATEST ceiling and
-- refund budget the merchant has already consumed.
--
-- DEFAULT 0 backfills existing rows as "no ramp applied". That is deliberately
-- permissive: a store mid-trial gets one more ramp application, matching
-- today's behaviour exactly once, and never again.
ALTER TABLE campaign_email_budget
    ADD COLUMN IF NOT EXISTS ramp_step_applied SMALLINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN campaign_email_budget.ramp_step_applied IS
    '§5.1 — highest trial-ramp transition day applied (0/4/8). Guards ApplyTrialRamp against re-inflating consumed budget (#399).';
```

- [ ] **Step 2: Write the down migration**

Create `migrations/000111_campaign_email_budget_ramp_step.down.sql`:

```sql
ALTER TABLE campaign_email_budget
    DROP COLUMN IF EXISTS ramp_step_applied;
```

- [ ] **Step 3: Bump the expected schema version**

In `services/marketplace-api/migrations.go`, change:

```go
const ExpectedSchemaVersion uint = 110
```

to:

```go
const ExpectedSchemaVersion uint = 111
```

This is not optional — `cmd/marketplace-api` refuses to start when the database version does not match this constant.

- [ ] **Step 4: Mirror the column on the model**

In `internal/campaignbudget/models.go`, add to the budget struct (which currently mirrors `store_id, month, remaining, limit_set, updated_at`), matching the existing tag style:

```go
	RampStepApplied int16 `gorm:"column:ramp_step_applied;not null;default:0"`
```

Read the struct first and match its field alignment and tag conventions exactly.

- [ ] **Step 5: Apply the migration to the dev database**

The integration tests run against a live shared database, so the column must exist there before Task 2's tests can pass:

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -c \
  "ALTER TABLE campaign_email_budget ADD COLUMN IF NOT EXISTS ramp_step_applied SMALLINT NOT NULL DEFAULT 0;"
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "select column_name, data_type, column_default from information_schema.columns where table_name='campaign_email_budget' and column_name='ramp_step_applied';"
```

Expected: the second command prints `ramp_step_applied|smallint|0`.

Note this database is **shared** with other work — the `IF NOT EXISTS` makes the statement safe to re-run, and adding a defaulted column is non-destructive.

- [ ] **Step 6: Build and vet**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./...
```

Expected: clean, exit 0.

- [ ] **Step 7: Confirm the migration files are well-formed**

```bash
cd services/marketplace-api
go test ./... -count=1 -run 'Migration' 2>&1 | tail -20
```

Expected: pass. `migrations_test.go` exists at the service root and typically checks up/down pairing and numbering — if it fails, read what it asserts and fix the migration to satisfy it. **Do not edit the test to accommodate a malformed migration.**

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/migrations/000111_campaign_email_budget_ramp_step.up.sql \
        services/marketplace-api/migrations/000111_campaign_email_budget_ramp_step.down.sql \
        services/marketplace-api/migrations.go \
        services/marketplace-api/internal/campaignbudget/models.go
git commit -m "feat(campaignbudget): add ramp_step_applied to campaign_email_budget (#399)"
```

---

### Task 2: Guard both ramp UPDATEs on the applied step

**Files:**
- Modify: `services/marketplace-api/internal/campaignbudget/ramp.go` (docstring at 42-50, day-4 SQL at 62-73, day-8 SQL at 75-88)
- Test: `services/marketplace-api/internal/campaignbudget/ramp_integration_test.go`

**Interfaces:**
- Consumes: `campaign_email_budget.ramp_step_applied` from Task 1.
- Produces: `ApplyTrialRamp`'s signature is unchanged — `func ApplyTrialRamp(ctx context.Context, db *gorm.DB, storeID uuid.UUID, day int, plan string) error`.

- [ ] **Step 1: Confirm the existing test fails for the right reason (RED — the test already exists)**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  -run 'TestApplyTrialRamp_Idempotent_ReRunSameDay' -v ./internal/campaignbudget/
```

Expected: `FAIL`, `Not equal: expected: 1800, actual: 2000`. A sub-second `ok` with no `=== RUN` means it skipped — stop and escalate.

- [ ] **Step 2: Add a day-8 idempotency test (finding 3 — the site the issue does not name)**

Append to `ramp_integration_test.go`, matching the file's existing style (`firstOfMonthUTC`, `testdb.NewDB(t, "campaign_email_budget")`):

```go
func TestApplyTrialRamp_Day8_Idempotent_ReRunSameDay(t *testing.T) {
	// Day 8 has the same GREATEST shape as day 4 and the same defect (#399):
	// ramp to the plan allowance, spend some of it, re-run — the balance must
	// NOT climb back to the allowance.
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 1500, 2000)`, storeID, month).Error)

	require.NoError(t, campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 8, "trial"))

	var afterRamp int
	require.NoError(t, db.Raw(
		`SELECT remaining FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&afterRamp))

	// Consume a chunk of the ramped budget.
	require.NoError(t, db.Exec(`
		UPDATE campaign_email_budget SET remaining = remaining - 3000
		WHERE store_id = $1`, storeID).Error)

	require.NoError(t, campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 8, "trial"))

	var remaining int
	require.NoError(t, db.Raw(
		`SELECT remaining FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&remaining))
	require.Equal(t, afterRamp-3000, remaining,
		"idempotent: re-running the day-8 ramp must not re-inflate remaining")
}
```

Run it and confirm it **FAILS** before the fix:

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  -run 'TestApplyTrialRamp_Day8_Idempotent_ReRunSameDay' -v ./internal/campaignbudget/
```

Expected: FAIL. If it passes before the fix, the day-8 defect is not what finding 3 says — stop and report rather than proceeding.

- [ ] **Step 3: Guard both UPDATEs**

In `ramp.go`, change the day-4 SQL to:

```go
		const sql = `
			UPDATE campaign_email_budget
			SET limit_set         = GREATEST(remaining, 2000),
			    remaining         = GREATEST(remaining, 2000),
			    ramp_step_applied = 4
			WHERE store_id = $1
			  AND month    = date_trunc('month', (now() AT TIME ZONE 'utc'))::date
			  AND ramp_step_applied < 4`
```

and the day-8 SQL to:

```go
		const sql = `
			UPDATE campaign_email_budget
			SET limit_set         = $1,
			    remaining         = GREATEST(remaining, $1),
			    ramp_step_applied = 8
			WHERE store_id = $2
			  AND month    = date_trunc('month', (now() AT TIME ZONE 'utc'))::date
			  AND ramp_step_applied < 8`
```

The guard lives in the `WHERE`, so each remains a single atomic statement — concurrent runs on multiple pods still produce one application, which is what finding 5 requires.

- [ ] **Step 4: Only count the metric when a row actually changed**

`TrialRampAppliedTotal.WithLabelValues(...).Inc()` currently fires unconditionally at the end of `ApplyTrialRamp`, which would now over-count every no-op re-run. Capture the result and increment only on a real application. Change each `db.WithContext(ctx).Exec(sql, ...)` to capture the result, e.g. for day 4:

```go
		res := db.WithContext(ctx).Exec(sql, storeID)
		if res.Error != nil {
			return fmt.Errorf("ramp day-4: %w", res.Error)
		}
		applied = res.RowsAffected > 0
```

Declare `var applied bool` before the `switch`, do the same for day 8, and replace the trailing unconditional increment with:

```go
	if applied {
		TrialRampAppliedTotal.WithLabelValues(strconv.Itoa(day)).Inc()
	}
	return nil
```

- [ ] **Step 5: Correct the docstring and the inline comment (finding 4)**

Replace the `// Idempotency:` paragraph in `ApplyTrialRamp`'s doc comment with:

```go
// Idempotency: each transition day is applied AT MOST ONCE per budget row,
// enforced by the `ramp_step_applied < N` guard in the WHERE clause. GREATEST
// alone is NOT sufficient — it is a floor, so a re-run after the merchant has
// spent budget would raise the balance back to the ceiling and refund consumed
// spend (#399). The guard is part of the same single atomic UPDATE, so
// concurrent runs on multiple pods still apply the step exactly once.
```

Delete the now-false inline comment at the day-4 case (`// Idempotent: if remaining already >= 2000, no change.`) and replace it with:

```go
		// Applied at most once: the ramp_step_applied guard, not GREATEST, is
		// what makes this idempotent.
```

- [ ] **Step 6: Build, vet, and run both idempotency tests plus the two first-run tests**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./...
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -run 'TestApplyTrialRamp' -v ./internal/campaignbudget/ 2>&1 | grep -E "^(--- |ok|FAIL)"
```

Expected: **all four `--- PASS`** — the two new/existing idempotency tests AND `TestApplyTrialRamp_Day3To4_RaisesToCeiling` and `TestApplyTrialRamp_Day7To8_UsesPlanAllowance`.

Finding 8 is the thing to watch: the first-run tests assert ceiling semantics and **must not** regress. If `Day3To4_RaisesToCeiling` now fails, your guard is wrong (a fresh row has `ramp_step_applied = 0`, so `0 < 4` must let the first run through) — fix the guard, do not touch the test.

- [ ] **Step 7: MUTATION TEST both guards**

*Day 4:* remove `AND ramp_step_applied < 4` from the day-4 SQL. Re-run Step 6's command.
Expected: **`TestApplyTrialRamp_Idempotent_ReRunSameDay` FAILS** (`expected: 1800, actual: 2000`). Restore; confirm PASS.

*Day 8:* remove `AND ramp_step_applied < 8` from the day-8 SQL. Re-run.
Expected: **`TestApplyTrialRamp_Day8_Idempotent_ReRunSameDay` FAILS**. Restore; confirm PASS.

Both mutations must fail their respective test. If either still passes, that guard is unpinned — STOP and escalate. Report all four observations verbatim.

- [ ] **Step 8: Run the full package**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/campaignbudget/... 2>&1 | tail -20
go test ./... -count=1
```

Expected: `ok` for every package, zero `--- FAIL`; no new unit failures. The pre-change baseline for `./internal/campaignbudget` was exactly one failure, so anything else is yours.

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/ramp.go \
        services/marketplace-api/internal/campaignbudget/ramp_integration_test.go
git commit -m "fix(campaignbudget): apply each trial-ramp step at most once so a re-run cannot refund consumed budget (#399)"
```

---

### Task 3: Wire `./internal/campaignbudget` into `make test-int`

**Files:**
- Modify: `Makefile` (repo root), the `test-int` package list

**Interfaces:**
- Consumes: a fully green `./internal/campaignbudget/...` from Tasks 1-2.

- [ ] **Step 1: Confirm the parent package is absent and now green**

```bash
grep -n "campaignbudget" Makefile
```

Expected: only `./internal/campaignbudget/cron/... \` — the parent is not listed.

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/campaignbudget/... 2>&1 | tail -20
```

Expected: every package `ok`. **If anything fails, STOP — do not add a red package to the target.**

Note the `/...` here also covers `internal/campaignbudget/concurrency` and `internal/campaignbudget/transactional`, which the Makefile comment says were never measured. This run measures them. If either fails, report it and fall back to listing `./internal/campaignbudget` and `./internal/campaignbudget/cron/...` explicitly rather than the ellipsis.

- [ ] **Step 2: Replace the cron-only entry**

In `Makefile`, change `	    ./internal/campaignbudget/cron/... \` to:

```
	    ./internal/campaignbudget/... \
```

(or to the explicit two-entry fallback from Step 1.) Preserve the exact leading whitespace and trailing backslash.

Then update the comment paragraph that says `./internal/campaignbudget/cron/... similarly leaves internal/campaignbudget/concurrency and internal/campaignbudget/transactional unassessed — their status was never measured, so they stay out too.` to reflect what you measured in Step 1 — either that they now run, or which one still does not and why.

- [ ] **Step 3: Verify the expansion**

```bash
make -n test-int | grep -o "\./internal/campaignbudget[^ ]*"
```

Expected: `./internal/campaignbudget/...` (or your fallback entries); no bare cron-only entry left.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "test: run internal/campaignbudget in test-int so the #399 idempotency guard cannot stop running"
```

---

## Self-Review

**Spec coverage.** #399's mechanism and repro → Task 2 Steps 1, 3. Its "suggested fix" proposes tracking cumulative granted and topping up the delta; this plan uses an applied-step guard instead, because finding 8 shows delta arithmetic would break `TestApplyTrialRamp_Day3To4_RaisesToCeiling`, an existing and correct test of first-run behaviour. The guard achieves the issue's stated goal — "any code path that can call ApplyTrialRamp more than once for the same day silently grants extra budget" — without that regression. Finding 3's day-8 site → Task 2 Steps 2, 3, 7. Finding 4's wrong docstring → Step 5. Finding 6's migration requirement → Task 1. Finding 9's runner gap → Task 3. Finding 10 is explicitly out of scope and routed to a new issue.

**Placeholder scan.** No TBDs. Both migrations, the model field, both SQL statements, the metric change, the docstring and the new test are given verbatim. Task 3 Step 1 hands the implementer a bounded pass/fail decision with both branches spelled out.

**Type consistency.** `ramp_step_applied` is `SMALLINT` in SQL and `int16` on the model, consistently named in the migration, the model, both SQL guards and both mutation tests. `ApplyTrialRamp`'s signature is unchanged, so `cron/jobs.go:87` keeps compiling. `applied` is declared once before the `switch` and read once after it.
