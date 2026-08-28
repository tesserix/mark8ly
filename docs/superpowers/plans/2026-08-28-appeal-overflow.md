# #398 — Merchant appeal text overflows `mismatch_reason varchar(100)`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a merchant's arbitrage appeal actually be recorded, instead of failing with SQLSTATE 22001 because the column cannot hold the text the code itself generates.

**Architecture:** `mismatch_reason` is `varchar(100)` but is used as an append-only narrative log: an evaluator-written reason, then one `MERCHANT_APPEAL ...` block per appeal. That is structurally unbounded, so the column type is the defect. Widen it to `TEXT`, cap the display site that renders it as a one-line inbox subtitle, and stop the failure path being invisible.

**Tech Stack:** Go 1.26, GORM, Postgres, golang-migrate, testify.

**Spec:** GitHub issue tesserix/mark8ly#398, plus the verified findings below.

## Global Constraints

- Run all Go commands from `services/marketplace-api`, never path-scoped, always `-count=1`.
- Required command set: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`.
- Integration tests: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`, `-p 1`. **Without that variable they skip silently and a skip prints `ok`.** Any claim about an integration test must name the DSN.
- **This change adds a migration. `ExpectedSchemaVersion` in `services/marketplace-api/migrations.go` MUST be bumped in the same commit.** It is currently `110`. **Use `000112` for this migration and set the constant to `112`** — `000111` is reserved by the concurrent #399 branch. If, when you start, `000111` does not exist in `migrations/` and the constant is still `110`, that only means #399 has not merged yet; still use `112` and note it in your report, and flag that whichever branch merges second may need renumbering.
- The new migration must ship both `.up.sql` and `.down.sql`.
- Commits: conventional, single line, no signature, no `Co-Authored-By` trailer, no emoji.
- Stage with explicit paths (`git add <path>`). Never `git add -A`.
- Work only inside this worktree: `.claude/worktrees/398-appeal-overflow`.

## Verified findings (established before planning — do not re-litigate)

1. **Reproduced by execution against the live database, not inferred.** `TestAppealService_MarksAuditRowUnderReview` (`appeal_test.go:31-80`) fails today with the real error:
   ```
   appeal.go:101 ERROR: value too long for type character varying(100) (SQLSTATE 22001)
   UPDATE "subscription_arbitrage_audit" SET "mismatch_reason"='PPP tier with card_country=US (developed)
   ---
   MERCHANT_APPEAL jurisdiction=IN justification=Our office is registered in India doc=gs://mark8ly-docs/appeal-123.pdf', ...
   appeal_test.go:69: Received unexpected error: update audit row: ERROR: value too long ... (SQLSTATE 22001)
   ```
   Run with `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`, `go test -tags=integration -p 1 -count=1 ./internal/arbitrage/`.

2. **That is the package's ONLY failing test.** Verified by a full-package run. So the fix makes `./internal/arbitrage` green.

3. **THE ISSUE IS UNDERSTATED.** The boilerplate is **34 characters**, not "~40" (`"\n---\nMERCHANT_APPEAL jurisdiction="`), 36 with the 2-char country, and 56 with the `" justification="` (15) and `" doc="` (5) prefixes. But the real understatement is `existingReason`, which is never empty on a flagged row — `Evaluate` always sets it (`evaluator.go:59-69`, persisted at `recorder.go:103-105`):
   - single-signal: `"PPP tier with card_country=US (developed)"` = **41 chars**
   - dual-signal: `"PPP tier with card_country=US (developed); ip_country=GB (developed)"` = **68 chars**

   So `68 + 36 = 104 > 100`: **a dual-signal flag overflows on the jurisdiction alone, with an empty justification and no document URL.** The issue's "any realistic appeal overflows" understates this — for that class of row, an *empty* appeal overflows. For a single-signal flag only 23 chars remain, of which `" justification="` eats 15, leaving **8 characters** of merchant text.

4. **The column type is confirmed at both ends and is NOT stale.** `migrations/000044_subscription_arbitrage_audit.up.sql:14` declares `mismatch_reason VARCHAR(100)`; no later migration alters it (only 000044 up/down mention the table). Live DB agrees: `character varying`, length `100`. GORM mirrors it at `internal/arbitrage/models.go:34` — `type:varchar(100)`. **The GORM tag must be changed too, or GORM's own DDL/AutoMigrate view stays wrong.**

5. **"Silently" is WRONG in one specific sense and RIGHT in three — the PR must say this accurately.** The merchant does *not* get silence: `internal/handlers/admin/arbitrage_appeal.go:101-111` maps the error to **HTTP 500** `{"error":"arbitrage_appeal_failed"}`. But:
   - **Zero logging.** No `logrus`/`slog`/`log.` in either `appeal.go` or `arbitrage_appeal.go`; `AbortWithStatusJSON` does not populate `c.Errors`, so the 22001 detail never reaches production logs. The root cause is invisible.
   - **Billing-ops is never notified** — `s.publisher.Publish` (`appeal.go:110`) sits after the early `return`, so the queue message is skipped. The code comment claims billing-ops "also polls the table directly", but the row that failed to update is exactly the one it would poll.
   - **The PII/compliance log lies** — `s.piiLogger.LogPIIAccess` fires first (`appeal.go:60-65`) with `Operation: "arbitrage_appeal_submit"`, so the compliance trail records a submitted appeal that was never persisted.

   Net: severity is unchanged or worse than filed.

6. **No input validation prevents it.** `arbitrage_appeal.go:27-37` has `binding:"required"` on `Jurisdiction` only; `Justification` and `DocumentURL` carry no `max=`. The single cap is `appeal.go:79-82`, truncating justification at **1000** — ten times the column width. `DocumentURL` is uncapped. The issue is not overstated on this point.

7. **`mismatch_reason` is the ONLY at-risk column — scope is closed.** Every column on `subscription_arbitrage_audit` was enumerated live: `id/subscription_id/tenant_id/store_id/reviewed_by` (uuid), `card_country/billing_country/ip_country` (char(2), and `NormalizeCountry` returns exactly 2 chars or `"??"`), `ip_hash` (varchar(64), HMAC-SHA256 hex is exactly 64), `resolved_price_tier` (varchar(20), enum-ish), `resolution` (varchar(30), CHECK-constrained), timestamps. Only `mismatch_reason` receives concatenated free text. There is **no sibling audit table** — `internal/arbitrage` declares exactly one `TableName()` (`models.go:43`).

8. **Both write sites enumerated.** `recorder.go:103-105` INSERTs it, bounded at 68 chars by `Evaluate` — safe. `appeal.go:104` UPDATEs it — the defect. Read-only elsewhere.

9. **One display site must be capped when the column widens.** `internal/inbox/arbitrage.go:53-56` assigns `subtitle = *r.MismatchReason` verbatim as an inbox item subtitle. Today `varchar(100)` is an accidental length guard; widening to TEXT removes it, so a multi-appeal narrative would render as a wall of text in the operator inbox. Capping there is part of this fix, not a nicety.

10. **A test that would catch this already exists but never runs.** `appeal_test.go:31-80` uses realistic input (33-char justification, 32-char doc URL) totalling 162 chars appended. It is double-gated: `//go:build integration` plus `TEST_DATABASE_URL` (`testhelpers_integration_test.go:18-23`). `./internal/arbitrage` is **absent from `make test-int`**, so it has never run. Task 4 closes that.

## File Structure

- `services/marketplace-api/migrations/000112_arbitrage_mismatch_reason_text.up.sql` / `.down.sql` — widen the column.
- `services/marketplace-api/migrations.go` — bump `ExpectedSchemaVersion` to 112.
- `services/marketplace-api/internal/arbitrage/models.go` — drop the `varchar(100)` GORM tag.
- `services/marketplace-api/internal/arbitrage/appeal.go` — log the write failure instead of swallowing the cause.
- `services/marketplace-api/internal/inbox/arbitrage.go` — cap the rendered subtitle.
- `Makefile` — add `./internal/arbitrage` to `test-int`.

---

### Task 1: Widen the column and bump the schema version

**Files:**
- Create: `services/marketplace-api/migrations/000112_arbitrage_mismatch_reason_text.up.sql`
- Create: `services/marketplace-api/migrations/000112_arbitrage_mismatch_reason_text.down.sql`
- Modify: `services/marketplace-api/migrations.go`
- Modify: `services/marketplace-api/internal/arbitrage/models.go:34`

**Interfaces:**
- Produces: `subscription_arbitrage_audit.mismatch_reason` becomes `TEXT`. Task 2's test and Task 3's display cap both depend on it.

- [ ] **Step 1: Confirm the failing test fails for the right reason (RED — the test already exists)**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  -run 'TestAppealService_MarksAuditRowUnderReview' -v ./internal/arbitrage/ 2>&1 | tail -20
```

Expected: `FAIL` with `SQLSTATE 22001` / `value too long for type character varying(100)`. A sub-second `ok` with no `=== RUN` means it skipped — stop and escalate.

- [ ] **Step 2: Write the up migration**

Create `migrations/000112_arbitrage_mismatch_reason_text.up.sql`:

```sql
-- 000112 — widen subscription_arbitrage_audit.mismatch_reason to TEXT (#398).
--
-- The column is used as an append-only narrative: the evaluator's reason,
-- then one "MERCHANT_APPEAL ..." block per appeal. That is unbounded by
-- design, so varchar(100) could not hold the code's own output — a
-- dual-signal flag (68-char reason) plus the 36-char appeal boilerplate is
-- 104 chars before a single character of merchant text, and the UPDATE
-- failed with SQLSTATE 22001.
--
-- TEXT rather than a wider varchar: there is no defensible finite bound on
-- "reason plus N appeals", and Postgres stores both identically.
ALTER TABLE subscription_arbitrage_audit
    ALTER COLUMN mismatch_reason TYPE TEXT;

COMMENT ON COLUMN subscription_arbitrage_audit.mismatch_reason IS
    'Append-only narrative: evaluator reason, then one MERCHANT_APPEAL block per appeal. TEXT because it is unbounded (#398). Truncate at display sites, not on write.';
```

- [ ] **Step 3: Write the down migration**

Create `migrations/000112_arbitrage_mismatch_reason_text.down.sql`:

```sql
-- Irreversible in general: rows written after the widening may exceed 100
-- chars, and narrowing would fail or truncate. Truncate explicitly so the
-- down migration is deterministic rather than erroring on real data.
UPDATE subscription_arbitrage_audit
   SET mismatch_reason = left(mismatch_reason, 100)
 WHERE mismatch_reason IS NOT NULL
   AND length(mismatch_reason) > 100;

ALTER TABLE subscription_arbitrage_audit
    ALTER COLUMN mismatch_reason TYPE VARCHAR(100);
```

- [ ] **Step 4: Bump the expected schema version**

In `services/marketplace-api/migrations.go`, set:

```go
const ExpectedSchemaVersion uint = 112
```

(See the Global Constraints note about `000111` being reserved by #399.) Report in your final report what value the constant held when you started.

- [ ] **Step 5: Drop the varchar tag on the model**

In `internal/arbitrage/models.go:34`, change:

```go
	MismatchReason    *string    `gorm:"column:mismatch_reason;type:varchar(100)"`
```

to:

```go
	MismatchReason    *string    `gorm:"column:mismatch_reason;type:text"`
```

Preserve the existing field alignment in the struct.

- [ ] **Step 6: Apply the change to the dev database**

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -c \
  "ALTER TABLE subscription_arbitrage_audit ALTER COLUMN mismatch_reason TYPE TEXT;"
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "select column_name, data_type, character_maximum_length from information_schema.columns where table_name='subscription_arbitrage_audit' and column_name='mismatch_reason';"
```

Expected: the second command prints `mismatch_reason|text|` (no length). This database is shared; widening a column is non-destructive.

- [ ] **Step 7: Build, vet, and confirm the test now passes**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./...
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  -run 'TestAppealService_MarksAuditRowUnderReview' -v ./internal/arbitrage/ 2>&1 | tail -20
```

Expected: `--- PASS`, and its assertions hold (`MismatchReason` contains `MERCHANT_APPEAL` and `IN`).

- [ ] **Step 8: Confirm the migration files are well-formed**

```bash
cd services/marketplace-api
go test ./... -count=1 -run 'Migration' 2>&1 | tail -20
```

Expected: pass, including the check that `ExpectedSchemaVersion` matches the highest migration. If it fails, fix the MIGRATION or the constant — never the test.

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/migrations/000112_arbitrage_mismatch_reason_text.up.sql \
        services/marketplace-api/migrations/000112_arbitrage_mismatch_reason_text.down.sql \
        services/marketplace-api/migrations.go \
        services/marketplace-api/internal/arbitrage/models.go
git commit -m "fix(arbitrage): widen mismatch_reason to TEXT so a merchant appeal can be recorded (#398)"
```

---

### Task 2: Pin the overflow with a test that fails on a narrow column

**Files:**
- Modify: `services/marketplace-api/internal/arbitrage/appeal_test.go`

**Interfaces:**
- Consumes: the widened column from Task 1.

Rationale: the existing test passes now, but it would also have passed at 150 chars. Finding 3's real case — a **dual-signal** flag overflowing with an *empty* appeal — is not covered by any test, and it is the worst case.

- [ ] **Step 1: Add a dual-signal, long-appeal test**

Read `appeal_test.go:31-80` first and mirror its fixture construction exactly (how it seeds the flag via the recorder, and how it builds `AppealService`). Then add:

```go
// The worst case from #398: a DUAL-signal flag already carries a 68-char
// evaluator reason, so the 36-char appeal boilerplate alone exceeded
// varchar(100) before any merchant text. Plus a realistic long
// justification and doc URL, which the 1000-char service cap allows.
func TestAppealService_DualSignalFlag_LongAppeal_IsRecorded(t *testing.T) {
	// ... seed a flag whose mismatch_reason carries BOTH signals, mirroring
	// TestAppealService_MarksAuditRowUnderReview's setup ...

	err := svc.Submit(context.Background(), arbitrage.AppealInput{
		TenantID:      tenantID,
		StoreID:       storeID,
		ActorUserID:   actorID,
		Jurisdiction:  "IN",
		Justification: strings.Repeat("Our registered office is in Bengaluru. ", 12),
		DocumentURL:   "gs://mark8ly-docs/appeals/2026/appeal-with-a-fairly-long-object-name-123456.pdf",
	})
	require.NoError(t, err, "a dual-signal flag with a realistic appeal must be recordable")

	var got arbitrage.SubscriptionArbitrageAudit
	require.NoError(t, db.Where("id = ?", auditID).First(&got).Error)
	require.NotNil(t, got.MismatchReason)
	require.Contains(t, *got.MismatchReason, "MERCHANT_APPEAL")
	require.Contains(t, *got.MismatchReason, "jurisdiction=IN")
	require.Greater(t, len(*got.MismatchReason), 100,
		"this test is only meaningful if the value exceeds the old varchar(100) bound")
}
```

Add `"strings"` to the imports if absent. The final assertion is deliberate: it makes the test self-verifying about *why* it exists.

- [ ] **Step 2: Run it**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  -run 'TestAppealService_DualSignalFlag_LongAppeal_IsRecorded' -v ./internal/arbitrage/ 2>&1 | tail -20
```

Expected: `--- PASS`.

- [ ] **Step 3: MUTATION TEST — prove it fails on the old column type**

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -c \
  "ALTER TABLE subscription_arbitrage_audit ALTER COLUMN mismatch_reason TYPE VARCHAR(100) USING left(mismatch_reason, 100);"
```

Re-run Step 2's command.

Expected: **FAIL with SQLSTATE 22001.** That is the proof the test pins the column width and not merely the code path.

Then restore and re-confirm:

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -c \
  "ALTER TABLE subscription_arbitrage_audit ALTER COLUMN mismatch_reason TYPE TEXT;"
```

Re-run Step 2 — expected `--- PASS`. **You must leave the column as TEXT.** Report both outcomes verbatim.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/appeal_test.go
git commit -m "test(arbitrage): cover the dual-signal flag whose appeal boilerplate alone overflowed varchar(100) (#398)"
```

---

### Task 3: Cap the display site and stop the failure being invisible

**Files:**
- Modify: `services/marketplace-api/internal/inbox/arbitrage.go:53-56`
- Modify: `services/marketplace-api/internal/arbitrage/appeal.go:98-107`

**Interfaces:**
- Consumes: the widened column from Task 1.

Rationale: findings 9 and 5. `varchar(100)` was an accidental length guard on the inbox subtitle; widening removes it. And the write failure currently reaches production logs nowhere at all.

- [ ] **Step 1: Cap the inbox subtitle**

In `internal/inbox/arbitrage.go`, replace the subtitle assignment:

```go
		subtitle := "Price tier " + r.ResolvedPriceTier
		if r.MismatchReason != nil && *r.MismatchReason != "" {
			subtitle = *r.MismatchReason
		}
```

with:

```go
		subtitle := "Price tier " + r.ResolvedPriceTier
		if r.MismatchReason != nil && *r.MismatchReason != "" {
			// mismatch_reason is an unbounded append-only narrative since
			// #398 widened it to TEXT — varchar(100) used to cap this by
			// accident. A subtitle is one line, so truncate for display.
			subtitle = truncateSubtitle(*r.MismatchReason, 120)
		}
```

and add, near the bottom of the file:

```go
// truncateSubtitle shortens s to at most max runes, appending an ellipsis
// when it cuts. Rune-based so a multi-byte character is never split.
func truncateSubtitle(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
```

- [ ] **Step 2: Log the write failure**

In `internal/arbitrage/appeal.go`, the `Updates(...)` error currently returns `fmt.Errorf("update audit row: %w", err)` and nothing logs it. Determine how this package/service logs — check whether `AppealService` already holds a logger field, and if it does not, **do not add a new dependency**; instead use `slog.Default()`, which `internal/handlers/platformadmin/audit.go:38` already uses as a fallback in this codebase. Replace the error return with:

```go
		}).Error; err != nil {
		// This path lost an appeal silently for as long as the column was too
		// narrow (#398): the merchant got a 500, but nothing logged the cause,
		// billing-ops was never notified (the Publish below is skipped), and
		// the PII log above already recorded a "submitted" appeal.
		slog.Default().Error("arbitrage appeal: update audit row failed",
			"audit_id", row.ID, "tenant_id", in.TenantID, "store_id", in.StoreID,
			"appended_len", len(appended), "err", err)
		return fmt.Errorf("update audit row: %w", err)
	}
```

Add `"log/slog"` to the imports. **Do not log `justification`, `appended`, or `DocumentURL` themselves** — this is merchant free text on a PII-audited path; the length is the diagnostic, the content is not.

- [ ] **Step 3: Build, vet, and run both packages**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./...
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/arbitrage/ ./internal/inbox/ 2>&1 | tail -20
go test ./... -count=1
```

Expected: `ok` for both; no new unit failures. Note `internal/inbox/arbitrage_integration_test.go:53` seeds a `mismatch_reason` fixture — confirm it still passes with the truncation in place.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/inbox/arbitrage.go \
        services/marketplace-api/internal/arbitrage/appeal.go
git commit -m "fix(arbitrage): truncate the inbox subtitle and log appeal write failures (#398)"
```

---

### Task 4: Wire `./internal/arbitrage` into `make test-int`

**Files:**
- Modify: `Makefile` (repo root)

**Interfaces:**
- Consumes: a fully green `./internal/arbitrage` from Tasks 1-3.

- [ ] **Step 1: Confirm absent and green**

```bash
grep -n "arbitrage" Makefile
```

Expected: no match.

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/arbitrage/ 2>&1 | tail -20
```

Expected: `ok`, zero failures. **If anything fails, STOP — do not add a red package.**

- [ ] **Step 2: Add it to the list**

In the `test-int` package list, add after `./internal/audit/... \`:

```
	    ./internal/arbitrage/... \
```

Match the surrounding indentation (a tab then four spaces) and keep the trailing backslash.

- [ ] **Step 3: Verify the expansion**

```bash
make -n test-int | grep -o "\./internal/arbitrage[^ ]*"
```

Expected: `./internal/arbitrage/...`.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "test: run internal/arbitrage in test-int so the #398 overflow guard cannot stop running"
```

---

## Self-Review

**Spec coverage.** #398's mechanism → Task 1. Its suggested fix offers "widen, or move appeal text to its own table"; this plan widens, because finding 7 shows `mismatch_reason` is the only affected column and a separate table would be a schema redesign disproportionate to a 22001. Finding 3's dual-signal worst case → Task 2. Finding 5's three real "silent" failures → Task 3 Step 2 (logging); the billing-ops skip and the lying PII log are *consequences* of the write failing and are resolved by it succeeding, which the PR should state rather than claim as separate fixes. Finding 9's display regression → Task 3 Step 1. Finding 10's runner gap → Task 4.

**Placeholder scan.** Both migrations, the model tag, the truncation helper and the logging block are given verbatim. Task 2 Step 1 deliberately leaves the fixture construction to the implementer with an explicit instruction to mirror the existing test — the surrounding fixture helpers are not quoted here, and inventing them would be worse than reading them.

**Type consistency.** `mismatch_reason` is `TEXT` in the migration and `type:text` in the GORM tag. `truncateSubtitle(s string, max int) string` is defined once and called once. `MismatchReason` stays `*string` throughout — the widening does not change Go-side nullability.
