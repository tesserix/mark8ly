# #369 — Expire operator free text on its own clock, keep the structural record for 7 years

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop an operator's free-text `reason` — which can name a person, e.g. *"jane@example.com disputing the chargeback"* — outliving a GDPR art.17 erasure by seven years, while keeping the governance record itself for the full seven.

**Architecture:** Retention is split by field rather than by row. The prune cron gains a second operator pass that strips the `reason` key from `audit_logs.metadata` (a `jsonb` column, so `metadata - 'reason'`) once a row is older than 180 days. Structural fields — actor, action, `reason_code`, timestamps — are untouched and still expire at 7 years via the existing `pruneOperatorRows`. The new pass mirrors that function's batched shape exactly.

**Tech Stack:** Go 1.26, GORM, Postgres `jsonb`, testify.

**Spec:** GitHub issue tesserix/mark8ly#369. The issue deliberately left the approach undecided; **the product owner chose "split retention by field" with a 180-day free-text window** — see Decision below.

## Global Constraints

- Run all Go commands from `services/marketplace-api`, never path-scoped, always `-count=1`.
- Required command set: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`.
- Integration tests: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`, `-p 1`. **Without that variable they skip silently and a skip prints `ok`.** Any claim about an integration test must name the DSN.
- **No migration.** This is a data-lifecycle change over an existing `jsonb` column; no schema change, so **do not touch `ExpectedSchemaVersion`**.
- **Strip `reason` ONLY. Never `reason_code`.** They are different keys with different purposes: `reason_code` is a closed, validated vocabulary and is the field a regulator's question turns on; `reason` is the free text. `metadata - 'reason'` removes exactly the one key.
- Commits: conventional, single line, no signature, no `Co-Authored-By` trailer, no emoji.
- Stage with explicit paths (`git add <path>`). Never `git add -A`.
- Work only inside this worktree: `.claude/worktrees/369-operator-freetext-retention`.

## Decision (made by the product owner — implement this, do not re-open it)

Of the four options #369 lists, **option 3, shorten retention for rows carrying free text**, was chosen, with a **180-day** window.

Rationale to carry into the PR:
- **Not option 1 (redact at erasure time)** — that is the same two-stage pattern #365 explicitly rejected for the purge row (`tenant_purge.go:290-303`: *"'We never recorded it' is both stronger and easier to explain than a two-stage rule"*), and it edits audit history on demand, which is its own governance problem.
- **Not option 2 (never store free text)** — discards context exactly where an operator most needs it.
- **Not option 4 (accept)** — the structural fields carry the governance value; the free text is incidental personal data, and keeping it seven years is not necessary for the record's purpose.
- **180 days** covers the full card chargeback window (Visa/Mastercard run to roughly 120-180 days), which is the main reason an operator re-reads a suspension note. After that the note's operational value is near zero while its privacy cost persists.

## Verified findings (established before planning — do not re-litigate)

1. **The two sites named in the issue are real and write free text unconditionally:**
   - `internal/handlers/platformadmin/tenant_lifecycle.go:269` — `"reason": req.Reason` in the `tenant.suspended` / `tenant.unsuspended` metadata.
   - `internal/handlers/platformadmin/billing_trial_extend.go:277` — `"reason": reason` in the `trial.extended` metadata.

2. **The enumeration is COMPLETE — verified exhaustively, not trusted.** There are four operator-emit sites on the platformadmin surface, and the two the issue does not name are both correctly out of scope:
   - `inbox_actions.go:193` — emits an operator row with **no `Metadata` field at all**. Its `body.Notes` goes to `migration.Review` via `ApproveAsOperator`/`RejectAsOperator`, landing in `migration_fast_path_reviews` — which the purge **does** delete (`tenantpurge/purge.go:382`). No surviving free text.
   - `tenant_purge.go:308` — already carved out by #365.
   - `internal/breakglass/audit.go:78,105,129` also write `md["reason"]`, but all three set `ForceActorType: audit.ActorSystem`, so they are **not** operator rows and the purge deletes them (`actor_type <> 'operator'`).

3. **Those rows genuinely survive a purge and live 7 years.** `tenantpurge/purge.go:369-371` is `DELETE FROM audit_logs WHERE tenant_id = ? AND actor_type <> 'operator'` — deliberate, per #288. `actor_type` is set to `operator` from `platform_operator_id` (`internal/audit/emitter.go:274`). Retention is `OperatorRetentionYears = 7` (`internal/audit/operator_prune.go:18`), applied at `prune_cron.go:136`.

4. **The mechanism is feasible — checked, not assumed.** `audit_logs.metadata` is `jsonb` (live DB), and `audit_logs` has **no `REVOKE UPDATE`**: the only three migrations containing `REVOKE` are `000045_business_entity_attestations`, `000050_subscription_plan_change_audit` and `000075_app_contract_attestations`. So an `UPDATE ... SET metadata = metadata - 'reason'` is permitted on this table, unlike on `subscription_plan_change_audit`.

5. **The prune cron's existing shape is the template.** `pruneOperatorRows` (`operator_prune.go:40-70`) loops a batched `DELETE ... WHERE id IN (SELECT id ... LIMIT ?)`, checks `ctx.Done()` each iteration, and terminates when `RowsAffected == 0`. `prune_cron.go:134-162` calls it, records `stats`, feeds `c.counter(label, n)`, and swallows failure — distinguishing `context.Canceled`/`DeadlineExceeded` (logged at Info, a clean shutdown) from real errors (Error). The new pass must mirror all of that.

## File Structure

- `services/marketplace-api/internal/audit/operator_prune.go` — add the free-text window constant, its metric label, and `pruneOperatorFreeText`.
- `services/marketplace-api/internal/audit/prune_cron.go` — run the new pass and record its stats.
- `services/marketplace-api/internal/audit/operator_freetext_integration_test.go` — new integration test.

---

### Task 1: Add the free-text stripping pass

**Files:**
- Modify: `services/marketplace-api/internal/audit/operator_prune.go`
- Modify: `services/marketplace-api/internal/audit/prune_cron.go` (the operator section, ~lines 133-162, and the `stats` struct)

**Interfaces:**
- Consumes: `c.db *gorm.DB`, `c.batchSize`, `c.counter CounterFn`, `c.logger` — all already on `PruneCron`.
- Produces:
  ```go
  const OperatorFreeTextRetentionDays = 180
  const OperatorFreeTextMetricLabel   = "operator_freetext_180d"
  func (c *PruneCron) pruneOperatorFreeText(ctx context.Context, cutoff time.Time) (int64, int, error)
  ```
  Returns `(rowsStripped, batches, error)`, mirroring `pruneOperatorRows`.

- [ ] **Step 1: Add the constants**

In `internal/audit/operator_prune.go`, below `OperatorMetricLabel`, add:

```go
// OperatorFreeTextRetentionDays is how long the free-text `reason` on an
// operator audit row is kept, counted from created_at. The row itself lives
// OperatorRetentionYears; only the free text expires on this shorter clock.
//
// Retention is split by FIELD because the two carry different value. The
// structural fields — actor, action, reason_code, timestamps — are what a
// governance question actually turns on, and they justify seven years. The
// free text is incidental personal data: an operator writes "jane@example.com
// disputing the chargeback, ticket 4471", and under #365's rule that row
// survives a GDPR art.17 erasure of the same tenant by seven years (#369).
//
// 180 days covers the full card chargeback window (Visa/Mastercard run to
// roughly 120-180 days), which is the main reason an operator re-reads a
// suspension note. Past that the note's operational value is near zero while
// its privacy cost persists.
//
// NOT redaction-at-erasure-time: that is the same two-stage rule #365
// rejected for the purge row, and it edits audit history on demand.
const OperatorFreeTextRetentionDays = 180

// OperatorFreeTextMetricLabel is the bucket label the free-text strip
// reports under, so it is distinguishable from row deletion in monitoring.
const OperatorFreeTextMetricLabel = "operator_freetext_180d"
```

- [ ] **Step 2: Add the stripping function**

Append to `internal/audit/operator_prune.go`, mirroring `pruneOperatorRows`' structure exactly:

```go
// pruneOperatorFreeText removes the free-text `reason` key from the metadata
// of operator audit rows older than cutoff, in batches, and returns
// (rowsStripped, batches, error).
//
// Strips `reason` ONLY — never `reason_code`. They are different keys: the
// code is a closed validated vocabulary and is the field a regulator's
// question turns on; the text is incidental. `metadata - 'reason'` removes
// exactly the one key and leaves the rest of the object intact.
//
// The `metadata ? 'reason'` guard is what terminates the loop: once a row is
// stripped it no longer matches, so RowsAffected reaches 0 the same way the
// DELETE loop's does. It also means a row with no reason is never rewritten.
func (c *PruneCron) pruneOperatorFreeText(ctx context.Context, cutoff time.Time) (int64, int, error) {
	var totalStripped int64
	batchCount := 0

	for {
		select {
		case <-ctx.Done():
			return totalStripped, batchCount, ctx.Err()
		default:
		}

		res := c.db.WithContext(ctx).Exec(`
			UPDATE audit_logs
			SET metadata = metadata - 'reason'
			WHERE id IN (
				SELECT id
				FROM audit_logs
				WHERE actor_type = 'operator'
				  AND created_at < ?
				  AND metadata ? 'reason'
				LIMIT ?
			)`,
			cutoff, c.batchSize,
		)
		if res.Error != nil {
			return totalStripped, batchCount, fmt.Errorf("audit operator free-text strip: %w", res.Error)
		}
		batchCount++
		totalStripped += res.RowsAffected
		if res.RowsAffected == 0 {
			return totalStripped, batchCount, nil
		}
	}
}
```

**Watch for a `?` collision.** GORM uses `?` as its own placeholder, and `metadata ? 'reason'` is Postgres's jsonb key-exists operator. If GORM misparses it, use the equivalent function form `jsonb_exists(metadata, 'reason')` instead — it means exactly the same thing and contains no `?`. Task 2's Step 2 will tell you which you need: if the query errors with a bind-variable mismatch, switch to `jsonb_exists`. Report which form you shipped.

- [ ] **Step 3: Run the pass from the cron**

In `internal/audit/prune_cron.go`, immediately after the existing operator block (the one ending with the `audit prune: operator path complete` log line, ~line 161), add:

```go
	// #369 — operator FREE TEXT, 180 days. The row itself stays for
	// OperatorRetentionYears; only the incidental personal data in `reason`
	// expires early. Same failure handling as the operator path above: a
	// lock conflict must not fail the whole pass.
	ftCutoff := now.AddDate(0, 0, -OperatorFreeTextRetentionDays)
	ftStripped, ftBatches, ftErr := c.pruneOperatorFreeText(ctx, ftCutoff)
	stats.OperatorFreeTextStripped = ftStripped
	stats.BatchesRun += ftBatches
	if c.counter != nil && ftStripped > 0 {
		c.counter(OperatorFreeTextMetricLabel, ftStripped)
	}
	if ftErr != nil {
		stats.ErrorsByPlan["operator free text (180 day retention)"]++
		if errors.Is(ftErr, context.Canceled) || errors.Is(ftErr, context.DeadlineExceeded) {
			c.logger.Info("audit prune: operator free-text path interrupted by shutdown",
				"stripped_so_far", ftStripped, "err", ftErr.Error())
		} else {
			c.logger.Error("audit prune: operator free-text path failed",
				"stripped_so_far", ftStripped, "err", ftErr.Error())
		}
	} else {
		c.logger.Info("audit prune: operator free-text path complete",
			"cutoff", ftCutoff.Format(time.RFC3339),
			"rows_stripped", ftStripped, "batches", ftBatches)
	}
```

Then add the field to the stats struct that already carries `OperatorRowsDeleted` (find it — it is the type `Run` returns), matching its style:

```go
	// OperatorFreeTextStripped counts operator rows whose free-text `reason`
	// was removed by the 180-day window (#369). These rows are NOT deleted.
	OperatorFreeTextStripped int64
```

- [ ] **Step 4: Build and vet**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./...
```

Expected: clean, exit 0.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/audit/operator_prune.go \
        services/marketplace-api/internal/audit/prune_cron.go
git commit -m "feat(audit): expire operator free-text reason after 180 days while keeping the row 7 years (#369)"
```

---

### Task 2: Pin the behaviour with a boundary test

**Files:**
- Create: `services/marketplace-api/internal/audit/operator_freetext_integration_test.go`

**Interfaces:**
- Consumes: `pruneOperatorFreeText` via `PruneCron.Run`, and the `OperatorFreeTextRetentionDays` constant.

Read `internal/audit/operator_prune_integration_test.go` first and mirror its fixture construction — how it builds a `PruneCron`, seeds an operator row with a controlled `created_at`, and asserts. It already does "the boundary, on the boundary" for the 7-year window, so it is the right model. **Do not invent a fixture helper; reuse what that file uses.**

- [ ] **Step 1: Write the test**

Create `internal/audit/operator_freetext_integration_test.go` with a `//go:build integration` tag (first line, matching its siblings) and these four cases:

```go
// #369 — the free-text `reason` on an operator row expires at 180 days; the
// row and its STRUCTURAL fields live the full OperatorRetentionYears.

// 1. THE BOUNDARY, ON THE BOUNDARY. 180 days minus a second keeps the text.
func TestOperatorFreeText_JustInsideWindow_Kept(t *testing.T) { ... }

// 2. Past 180 days, `reason` is gone — but the ROW is still there and its
//    structural fields are intact. This is the whole point of splitting
//    retention by field rather than deleting the row early.
func TestOperatorFreeText_PastWindow_StrippedButRowAndStructureSurvive(t *testing.T) {
	// after Run:
	//   - the row still exists
	//   - metadata has NO "reason" key
	//   - metadata STILL HAS "reason_code" with its original value
	//   - action / actor_type / created_at unchanged
}

// 3. reason_code must NEVER be collateral damage — it is the field a
//    regulator's question turns on (#365 kept it deliberately).
func TestOperatorFreeText_ReasonCodeSurvives(t *testing.T) { ... }

// 4. Non-operator rows are untouched, however old.
func TestOperatorFreeText_NonOperatorRowUnaffected(t *testing.T) { ... }
```

Seed rows with `created_at` set explicitly (as the 7-year test does) rather than relying on wall-clock. For case 2, seed metadata containing **both** keys, e.g. `{"reason_code":"fraud","reason":"jane@example.com disputing the chargeback, ticket 4471"}`, so the test demonstrates the exact scenario in the issue.

- [ ] **Step 2: Run it**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -run 'TestOperatorFreeText' -v ./internal/audit/ 2>&1 | tail -30
```

Expected: four `--- PASS`. A sub-second `ok` with no `=== RUN` means it skipped — stop and escalate.

**This is also where the `?` question from Task 1 Step 2 resolves.** If the query fails with a bind-variable / placeholder error, switch `metadata ? 'reason'` to `jsonb_exists(metadata, 'reason')` and re-run. Report which form you shipped and why.

- [ ] **Step 3: MUTATION TEST — three separate mutations**

*Mutation A — the window.* Change `OperatorFreeTextRetentionDays` to `36500` (100 years) so nothing is old enough. Re-run.
Expected: **case 2 FAILS** (the reason is still present). Restore; confirm PASS.

*Mutation B — the key.* Change `metadata - 'reason'` to `metadata - 'reason_code'`. Re-run.
Expected: **case 3 FAILS** (reason_code was destroyed). Restore; confirm PASS. This is the important one: it proves the test would catch the single worst mistake this change could make.

*Mutation C — the actor filter.* Remove `AND actor_type = 'operator'` from the WHERE. Re-run.
Expected: **case 4 FAILS** (a non-operator row was stripped). Restore; confirm PASS.

If any mutation leaves all tests green, that guard is unpinned — STOP and escalate. Report all six observations verbatim.

- [ ] **Step 4: Run the whole audit package**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/audit/... 2>&1 | tail -20
go test ./... -count=1
```

Expected: `ok`, zero `--- FAIL`. `./internal/audit/...` is already in `make test-int`, so this package must stay green — the existing 7-year operator tests in particular must not regress.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/audit/operator_freetext_integration_test.go
git commit -m "test(audit): pin the 180-day operator free-text window and that reason_code survives it (#369)"
```

---

### Task 3: Say so at the two sites that write the text

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/tenant_lifecycle.go` (near line 269)
- Modify: `services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend.go` (near line 277)

Rationale: an operator typing a chargeback note into `reason` cannot tell from the call site that the text has a different lifetime from the row it sits in. A future reader adding a third operator action needs to know the rule exists.

- [ ] **Step 1: Annotate both metadata blocks**

Above the `"reason":` entry in each of the two `Metadata: map[string]any{...}` literals, add:

```go
				// `reason` is FREE TEXT and expires after
				// audit.OperatorFreeTextRetentionDays (180 days), separately
				// from the row, which lives audit.OperatorRetentionYears.
				// An operator may put a person's name or email here, and it
				// must not outlive a GDPR art.17 erasure of this tenant by
				// seven years (#369). `reason_code` below is structural and
				// is kept for the full window.
```

Place it so it reads naturally against the existing surrounding comments — `billing_trial_extend.go` already carries a long explanatory block above its metadata, so match that file's tone and indentation.

- [ ] **Step 2: Build, vet, test**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./...
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/platformadmin/ 2>&1 | tail -20
```

Expected: build and vet clean. For the test run: the handoff records **2 known pre-existing failures** in this package, both `relation "inbox_action_idempotency" does not exist` — a missing table in the test environment, not a code defect, and I confirmed that table is absent from the dev database. If you see exactly those two, that is the expected baseline. **Anything else failing is yours** — do not wave it through as pre-existing; check it at the base commit:
```bash
git worktree add /private/tmp/base-369 aa7feaed --detach
```

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/tenant_lifecycle.go \
        services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend.go
git commit -m "docs(platformadmin): note that operator reason free text expires on the 180-day clock (#369)"
```

---

## Self-Review

**Spec coverage.** #369 asks for a decision among four options and then its implementation; the decision is recorded above and implemented by Tasks 1-2. Finding 2's exhaustive enumeration is why only two write sites need annotating (Task 3) — the issue's scope turned out to be exactly right, confirmed rather than assumed. The issue's note that this "interacts with #259's statutory clock" is not addressed here and should not be: #259 is a separate effort, and the 180-day window is independent of when an erasure request arrives.

**Placeholder scan.** Task 1's code is given verbatim. Task 2 Step 1 deliberately gives test *names, cases and assertions* but not fixture boilerplate, with an explicit instruction to mirror `operator_prune_integration_test.go` — the fixture helpers are not quoted here and inventing them would be worse than reading them. That is a bounded, stated deferral, not a placeholder.

**Type consistency.** `pruneOperatorFreeText` returns `(int64, int, error)`, matching `pruneOperatorRows` and the `ftStripped, ftBatches, ftErr` destructuring. `OperatorFreeTextRetentionDays` is an untyped int constant used with `now.AddDate(0, 0, -OperatorFreeTextRetentionDays)`. `OperatorFreeTextStripped` is `int64`, matching `OperatorRowsDeleted`.
