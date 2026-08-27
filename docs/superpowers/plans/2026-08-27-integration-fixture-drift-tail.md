# Integration Fixture Drift — Tail Triage

Task 7 of `docs/superpowers/plans/2026-08-27-integration-fixture-drift.md`. Re-measures the whole
suite after Tasks 1–6 (clusters A, B, C), proves the branch added zero regressions, and classifies
every remaining failure as **FIXTURE** (fixed by Tasks 10–15 below) or **PRODUCT** (a real defect,
out of scope for this branch).

## Measurement method — read this before re-measuring anything

A plain `go test -tags=integration -p 1 ./...` run against the shared `TEST_DATABASE_URL` is **not
a usable instrument on this branch**, for two independent reasons proven while measuring:

1. **`internal/billing/tax/revalidation` deadlocks**, and under `-p 1` its poisoned transaction/held
   lock makes every package that runs *after* it in the same `go test` invocation time out too — not
   fail cleanly, time out at whatever `-timeout` was given. With the package included in the run,
   `billing/trial`, `campaignbudget`, `campaignbudget/cron`, and `category` all hit the test timeout
   and report nothing useful. With it simply excluded from the package list, those same packages run
   in 11.3s, 1.1s, 0.6s, and 0.9s respectively. **Exclude `internal/billing/tax/revalidation` from any
   `./...`-shaped run**, or every package alphabetically after it becomes uninterpretable.

2. **Packages pollute each other through the shared database even with #1 solved.** `pkg/testdb.NewDB`
   truncates only the tables it is explicitly told about (`testdb.NewDB(t, "campaign_email_budget")`,
   for example, truncates exactly that table and nothing else). When two packages insert into the same
   table under different truncate lists — or when a package's own truncate list is incomplete — rows
   from an earlier package's run are still present when a later package's test queries `COUNT(*)` or a
   list endpoint. A `./...` run measured this way invented roughly 46 "new failures" that do not
   reproduce on a clean database, and 7 of the 19 packages that looked FAIL on a dirty database
   actually pass cleanly. **A `./...` count is not a diffable signal.**

**The only trustworthy instrument found: `TRUNCATE` all ~100 tables, then run exactly one package,
repeat for every package, never let two packages share a database state.** This was run at both the
baseline commit (`30c3fdff`) and this branch's HEAD. Anyone re-measuring this suite without truncating
between packages will get fiction and should be told so before they burn a day on it — this note
exists so the next person doesn't have to rediscover it.

Per-package clean-DB result at this branch's HEAD (`/tmp/clean-iso.txt`):

```
./internal/apikeys :: FAIL
./internal/arbitrage :: FAIL
./internal/audit :: ok
./internal/billing/dispatch :: ok
./internal/billing/trial :: ok
./internal/campaignbudget :: FAIL
./internal/category :: FAIL
./internal/handlers/admin :: FAIL
./internal/handlers/platformadmin :: ok
./internal/handlers/storefront :: FAIL
./internal/handlers/webhooks :: FAIL
./internal/orderrefund :: FAIL
./internal/page :: FAIL
./internal/product :: FAIL
./internal/subscription/dunning :: ok
./internal/subscription/lifecycle :: ok
./internal/subscription/planchange :: FAIL
./internal/tenantpurge :: ok
./internal/whitelabel/lifecycle :: FAIL
```

## Before / after

| | baseline `30c3fdff` | branch HEAD |
|---|---|---|
| failing tests | 187 | **25** |
| failing packages | 22 | **12** |

**FIXED: 162. REGRESSIONS: 0.** The baseline was independently re-measured with the same clean-DB
method and diffed by test *name* against the original baseline capture — identical, so the original
187 was accurate and the 162-fixed / 0-regression numbers are trustworthy, not an artifact of
re-deriving the baseline differently.

`make test-int` (16 packages) is green, verified twice. None of clusters A/B/C's signatures
(`products.vendor_id` NOT NULL, `store_subscriptions_store_id_fkey`,
`stores.storefront_customer_portal_secret` NOT NULL) appears anywhere in the remaining 25 — clusters
A, B, C contribute zero failures, as Tasks 1–6 intended.

## Remaining signature histogram (clean DB, this branch)

```
6  value too long for type character varying(60)       internal/apikeys
2  value too long for type character varying(100)      internal/arbitrage
2  column "created_at" of relation "stores" missing    internal/handlers/admin
2  404 page not found                                  internal/handlers/admin (unmounted routes)
1  nil pointer dereference                              internal/whitelabel/lifecycle (audit.Emitter)
1  shipments_order_id_fkey                              internal/handlers/admin
1  current transaction is aborted (25P02)                internal/page (downstream of another failure)
```

The 12 failing packages: `apikeys`, `arbitrage`, `campaignbudget`, `category`, `handlers/admin`,
`handlers/storefront`, `handlers/webhooks`, `orderrefund`, `page`, `product`,
`subscription/planchange`, `whitelabel/lifecycle`.

## Classification — all 25 named failures

**Already known, already being filed elsewhere — classified PRODUCT here without re-analysis, per
the controller's instruction:**

| Test | Package | Signature | Why PRODUCT |
|---|---|---|---|
| — (panic, no named test) | `whitelabel/lifecycle` | nil pointer dereference | `audit.NewEmitter` accepts a nil `Repo`; `write` (`internal/audit/emitter.go:216`) dereferences it on a background goroutine and kills the whole test binary. Filed: **issue #318**. Task 9 of the parent plan already covers this. |
| `TestAppealService_MarksAuditRowUnderReview` | `arbitrage` | `value too long for type character varying(100)` | `internal/arbitrage/appeal.go:73-118` appends merchant-supplied justification/document-URL text to `mismatch_reason` (`varchar(100)`) with no length guard before the `UPDATE`. Confirmed by reading `appeal_test.go:31-69` and `appeal.go:88-96` — the appended string is routinely > 100 chars for realistic input. Production truncation/validation bug, not a fixture problem. |
| `internal/billing/tax/revalidation` (deadlock, no named test in the 25) | — | hang, not a `--- FAIL` | Excluded from every `./...` run per the measurement-method section above. Already known, already filed separately. No further analysis performed, per instruction. |

**apikeys — already covered by the existing Task 8, not re-analyzed or duplicated here:**

| Test | Package | Signature |
|---|---|---|
| `TestLastUsedWorker_PersistsUpdate`, `TestRepo_Create_AndLookupByTenantPrefix`, `TestRepo_Revoke_FlipsRow`, `TestRepo_RotationOverlap_StaysUsableUntilExpiry`, `TestRepo_CountActiveForStore_ExcludesRevoked`, `TestRepo_UpdateLastUsed` | `apikeys` | `value too long for type character varying(60)` |

Task 8 already diagnoses this precisely (a 63-character fixture literal against a 60-character
column) and has its own verify/Makefile/commit steps. No new task is added for it here.

**New PRODUCT-class findings, discovered during this triage:**

| Test | Package | Signature | Evidence |
|---|---|---|---|
| `TestApplyTrialRamp_Idempotent_ReRunSameDay` | `campaignbudget` | assertion mismatch (`expected: 1800, actual: 2000`) | `internal/campaignbudget/ramp.go:56-73` — `ApplyTrialRamp`'s day-4 path runs `SET remaining = GREATEST(remaining, 2000)`. The docstring (`ramp.go:44-46`) claims this is idempotent — "re-running on the same transition day with a smaller remaining uses GREATEST semantics so consumed balance is never re-inflated" — but that is only true if `remaining` never *drops* below 2000 between runs. The test seeds 50→ramp(day4)→2000, then `Reserve(200)` drops remaining to 1800, then re-runs `ApplyTrialRamp(day4)` — `GREATEST(1800, 2000)` re-inflates `remaining` back to 2000, contradicting the docstring's own idempotency claim. Real bug: the cron is not idempotent against merchant consumption between runs. |
| `TestIntegration_Category_ListActiveByStoreID_ExcludesInactiveAndDeleted` | `category` | assertion mismatch (expected 2 active rows, got 3 — the row explicitly created with `IsActive=false` came back active) | `internal/category/models.go:20` — `IsActive bool \`gorm:"column:is_active;not null;default:true"\`` combined with a plain (non-pointer) `bool` field. This is a documented GORM footgun: when a field carries a `default:` tag, GORM skips the field on `.Create()` if its value equals the Go zero value — and `false` **is** the zero value for `bool`. The result: `category.Repository.Create` (`internal/category/repository.go:33-41`) can never actually persist `is_active=false`; every insert silently falls back to the column default `true`, regardless of what the caller set. Confirmed by reading `repository_integration_test.go:249-253` (`inactive.IsActive = false` before `repo.Create`) against the DB result: `IsActive:true` in the failing test's dump. |
| `TestRepository_GetBySlug_PublishedOnly`, `TestService_GetBySlug_FiltersUnpublishedOnStorefrontRead` | `page` | "unpublished page should not be returned" / "unpublished page must not leak to storefront" — both got the page back instead of nil | **Same root cause as the category finding above.** `internal/page/models.go:21` — `Published bool \`gorm:"column:published;not null;default:true"\`` — identical footgun. A page created with `Published: false` is silently stored as `published=true`. |
| `TestStorefrontPages_Get_UnpublishedReturns404`, `TestStorefrontPages_List_OnlyPublished` | `handlers/storefront` | "want 404 for unpublished page, got 200" / "want 2 published pages, got 3" | Downstream of the same `page.Published` GORM-default bug — `seedPageForStorefront` (`pages_integration_test.go:52-69`) calls `page.Service.Create` with `Published: &pub` where `pub=false`; the write silently becomes `true`, so the "unpublished" fixture page is actually published and the storefront correctly (if confusingly) serves it. Not a separate bug — one root cause, five tests. |
| `TestIntegration_ProductService_UpdateAggregate_RemovedVariantIDsSoftDelete` | `product` | assertion mismatch ("active variants = 4, want 2") | `internal/product/models.go:130` — `ProductVariant.DeletedAt *time.Time` (a plain pointer, not `gorm.DeletedAt`), so GORM's automatic soft-delete filtering never applies. Every `Preload("Variants")` call in `internal/product/repository.go` (lines 209, 281, 351, 405, 447) loads variants with no `deleted_at IS NULL` condition, so soft-deleted variants are returned alongside active ones on every aggregate read. The test's own soft-delete assertion (`deletedCount == 2`) passes — the deletion itself works — but the subsequent `svc.Get` still returns all 4 variants because the read path never filters them out. Systemic: this affects every caller of `GetByIDForStore` / the other four `Preload("Variants")` sites, not just this test. |
| `TestIntegration_ProductService_UpdateAggregate_OptionValueInUseRejected` | `product` | got `variant_matrix_mismatch` instead of the expected `ErrOptionValueInUse` | `internal/product/service_aggregate.go:144-107` calls `ValidateMatrix` (`internal/product/matrix.go:52`) **before** the transaction that would reach `applyOptionsDiff`'s `OptionValueInUse` check (`service_aggregate.go:445`). `ValidateMatrix` rejects any variant whose `OptionValues` reference a value not present in the *desired* option spec — which by definition includes every variant still referencing a value that is being removed. Traced through: with the test's desired options (`Size:[M]`, `expected=1`) and 2 supplied variants, the count check (`expected != len(variants)`) fires first (`variant_matrix_mismatch`); even a variant-count-matching construction would still fail earlier on `ValidateMatrix`'s per-variant "references unknown option value" check, never reaching `applyOptionsDiff`. **`OptionValueInUse` at `service_aggregate.go:445` appears to be unreachable through the public `UpdateAggregate` API** given the current validation order — worth its own issue: either loosen `ValidateMatrix` to let this scenario through so the intended guard actually runs, or confirm the code path is genuinely dead and remove/document it as defense-in-depth. |
| `TestExecute_Downgrade_StudioToStarter_OverQuota_Rejected` | `subscription/planchange` | assertion mismatch (audit row count: expected 1, got 0) | Read per the controller's specific ask: does the orchestrator write the `downgrade_blocked_over_quota` audit row on this path, or not? **It writes it, but the write is rolled back.** `internal/subscription/planchange/downgrade.go:41-50` calls `WritePlanChangeAuditRowTx(ctx, tx, ...)` with `Action: "downgrade_blocked_over_quota"`, using the *same* `tx` the whole call is running in, then returns `Output{}, ErrStoreCountOverQuota` at `downgrade.go:70`. That `tx` comes from `subscription.WithAdvisoryLock` (`internal/subscription/advisory_lock.go:15-22`), which wraps the entire operation in `db.Transaction(func(tx) error { ... })` — standard GORM semantics: a non-nil return rolls the transaction back. Since `executeDowngradeSchedule` returns `ErrStoreCountOverQuota` (non-nil) up through `Execute` (`planchange.go:154-198`), the whole transaction — including the audit-row insert written moments earlier — is rolled back. The comment at `downgrade.go:40` ("Write a blocked audit row so ops can see why the downgrade was refused") is defeated by the transaction it runs inside. Real bug: the audit trail for the one case ops most needs to see (a blocked downgrade) never persists. |

**FIXTURE-class findings — fixed by Tasks 10–15 appended to the parent plan below:**

| Test | Package | Signature | Root cause | Fixed by |
|---|---|---|---|---|
| `TestPromoApply_BelowAbsoluteFloor_INR`, `TestRefund_OutsideCoolingOff` | `handlers/admin` | `404 page not found` | The route **exists** — `internal/handlers/admin/routes.go:704-761` mounts `/subscription/apply-promo` and `/subscription/refund` inside `if deps.SubscriptionHandler != nil { ... if deps.PromoHandler != nil {...}; if deps.RefundHandler != nil {...} }`. Both test routers (`setupTestRouter` in `products_integration_test.go`, `setupRefundTestEnv` in `refund_integration_test.go`) construct `admin.Deps{}` without ever setting `SubscriptionHandler`, so the whole `/subscription` group — including the routes these two tests exercise — is never registered. This is exactly the "show from source the route exists and the test is wrong" case the brief called out; confirmed, not deleting anything. | Task 10 |
| `TestShipmentDispatchedEmailGate` | `handlers/admin` | `shipments_order_id_fkey` violation | The test generates `orderID := uuid.New()` (`shipments_dispatched_dedup_test.go:39`) and inserts a `shipments` row referencing it directly, without ever inserting a parent `orders` row. `shipments.order_id` has a real FK. A `seedOrderRowForSync` helper already exists in the same package (`shipments_tracking_sync_test.go:290-309`) for exactly this. | Task 13 |
| `TestShipmentsSync_AdvancesStatusLadder`, `TestShipmentsSync_CarrierErrorDoesNotBlockOthers` | `handlers/admin` | `column "created_at" of relation "stores" does not exist` | `seedStoreRowForSync` (`shipments_tracking_sync_test.go:273-286`) hand-rolls `INSERT INTO stores (..., created_at, ...)`. Per the parent plan's verified schema facts, **`stores` has no `created_at` column at all.** `testdb.SeedStore` (Task 1) already exists and is already used by every other cluster-C/A fixed site — this one was simply never migrated over. | Task 13 |
| `TestGatewayFor_ActiveConfig` | `orderrefund` | "no secret store or encryptor wired" | Exactly the config-wiring gap named in the original brief for `resolver_integration_test.go:239`. `orderrefund.NewResolver(db)` (`resolver.go:51`) intentionally returns an unconfigured resolver — the doc comment at `resolver.go:48` says "Wire `WithSecretStore` (or `WithEncryptor`) before use." The test never chains either. The sibling test file in the same package, `resolver_creds_test.go:56`, already uses `crypto.NewNoopEncryptor()` for this exact purpose. | Task 11 |
| `TestFullWebhookFlow_AllAllowlistedEvents` | `handlers/webhooks` | `open ../../scripts/webhook-fixtures: no such file or directory` | Off-by-one relative path. The test file lives at `internal/handlers/webhooks/stripe_integration_test.go` — three directories below `services/marketplace-api/` — but its `filepath.Join("..", "..", "scripts", "webhook-fixtures")` (`stripe_integration_test.go:41`) only climbs two. Verified: `ls ../../scripts/webhook-fixtures` from that package directory fails; `ls ../../../scripts/webhook-fixtures` lists the 11 fixture files that `git ls-files` confirms are tracked and present. | Task 12 |
| `TestRepository_Create_DuplicateSlug_Errors` | `page` | `current transaction is aborted, commands ignored until end of transaction block (SQLSTATE 25P02)` | The test (`internal/page/repository_integration_test.go:148-176`) runs three `repo.Create` calls inside one shared `testdb.NewTx` transaction and *expects* the second one to fail (duplicate slug) while the third succeeds. Postgres aborts the entire transaction on any real constraint-violation error until `ROLLBACK`/a `SAVEPOINT` recovery — there is none here, so the third statement fails with 25P02 regardless of what it is. This is exactly the masking effect the parent plan's Task 7 brief predicted ("an early insert failure aborts the transaction and masks every assertion after it"), just occurring within a single test rather than across tests. `category`'s `expectCreateFails` helper (`repository_integration_test.go:39-52`) already solves this with `SAVEPOINT sp` / `ROLLBACK TO SAVEPOINT sp` around exactly this kind of expected-to-fail call. | Task 14 |
| `TestIntegration_ProductRepo_ListPublished_ExcludesDraftArchivedDeletedUnpublished` | `product` | assertion mismatch (expected 1 published row, got 0) | Postgres freezes `now()` (`transaction_timestamp()`) to the moment a transaction begins and holds that value for every statement inside it. `testdb.NewTx` (`pkg/testdb/testdb.go:34-48`) opens the transaction with `db.Begin()` before the test does anything else. The test then calls `seedStore(t, tx)` and builds several aggregates before capturing `now := time.Now()` (`repository_integration_test.go:403`) and using it as `PublishedAt` on an app-clock timestamp captured *after* the transaction began. `ListPublished`'s `WHERE ... published_at <= now()` (`repository.go:319-320`) evaluates Postgres's `now()`, which is still pinned to the earlier transaction-start instant — so the freshly-set `published_at` (a later, real wall-clock value) reads as "in the future" relative to the frozen `now()`, and the row is filtered out. Deterministic, not flaky: the gap between `db.Begin()` and the `time.Now()` capture is real wall-clock time spent on prior seeding calls. Not a production issue — request-scoped transactions in production begin immediately before this query runs, so the gap is negligible there; it only bites a long-lived test transaction that seeds first and queries later. | Task 15 |

## Cross-package pollution finding

Documented here for the record, not turned into a task — it's a testing-infrastructure defect, not
covered by "fixture" tasks scoped to specific test files, and deserves its own issue rather than
folding into this plan's task numbering.

**Root cause:** `pkg/testdb.NewDB(t, tablesToCleanup ...string)` truncates *only the tables the
caller names*, per test, on cleanup (`pkg/testdb/testdb.go:50-59`). Nothing enforces that a package's
truncate list is complete, or that two packages inserting into the same table (`stores`,
`store_subscriptions`, `products`, etc.) agree on ownership. When package A's truncate list omits a
table that package B also writes to, rows A left behind are still present when B's test runs a
`COUNT(*)` or unscoped list query — and vice versa depending on run order.

**Why this only became visible now, on this branch, and not at the `30c3fdff` baseline:** at
baseline, 165 of 187 failures were early-insert failures — a `NOT NULL` violation or a missing FK
parent row aborted the test's transaction (or the `NewDB` insert) before the test ever got far enough
to leave meaningful rows behind for a later test to collide with. Clusters A–C (Tasks 1–6) fixed
those early failures, which means tests now run to completion and actually commit real rows via
`NewDB` — which is exactly the condition under which incomplete truncate lists start to matter. This
is the masking effect the parent plan predicted, one level up: earlier, insert failures masked
*assertions within a test*; now that inserts succeed, incomplete truncate lists let *state leak
between tests*. Both are the same shape of problem — an early, silent failure hiding a later,
real one — recurring at different levels of the same system as each layer of masking is peeled back.

**Recommendation:** file a standalone issue on `pkg/testdb.NewDB`'s truncation contract — e.g. an
audit of every truncate list against the tables its package's `INSERT`s actually touch, or a stronger
primitive (`NewDB` truncating a fixed, comprehensive table list by default rather than a per-caller
allowlist). Out of scope to design or implement here; this triage's only obligation is to name it so
it isn't lost.

## Summary

- 25 named failing tests remain after Tasks 1–6.
- 6 are already covered by the existing Task 8 (apikeys) — not duplicated.
- 3 are already-known, already-being-filed production defects (arbitrage varchar overflow, the
  `audit.Emitter` nil-repo panic behind `whitelabel/lifecycle`, the `billing/tax/revalidation`
  deadlock) — no new analysis, no new task.
- 10 are new PRODUCT-class findings from this triage (campaignbudget non-idempotence, the
  `gorm:"default:true"` footgun shared by `category.IsActive` and `page.Published` across 5 tests,
  the product soft-delete `Preload` gap, the unreachable `OptionValueInUse` validation order, and the
  planchange audit-row rollback) — documented above with file:line evidence, no code changed, no
  issues filed by this task (out of scope: documentation only).
- 9 are FIXTURE-class and get concrete tasks (10–15) appended to the parent plan below.
- 1 package (`whitelabel/lifecycle`) fails via panic with no named test, already covered by issue
  #318 / Task 9.
