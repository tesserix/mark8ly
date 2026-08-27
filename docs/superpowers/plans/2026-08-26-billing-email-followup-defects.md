# Billing email follow-up defects — implementation plan

Three defects left open by #384, all surfaced by its whole-branch review and
recorded on #381. Ordered by merchant impact.

**Branch:** `fix/381-followup-defects`, worktree `/tmp/m8-defects`, off `1dc04a87`.

## Global Constraints

- Module path `github.com/mark8ly/marketplace-api`; work under `services/marketplace-api/`.
- Single-line conventional commits. No signature, no `Co-Authored-By`, no multi-line body.
- **Never push, open a PR, merge, deploy, or switch branches.**
- `gofmt -l .` empty; `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` clean.
- Integration: `TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" go test -tags=integration -p 1 -count=1 ./<pkg>/ -run <Name> -v`. LAN IP `192.168.1.110`, never `localhost`. **Scope every run with `-run`** — the database is shared with another session. `testdb.NewDB` SKIPS when unreachable, so confirm `--- PASS` by name; a `--- SKIP` proves nothing.
- `store_subscriptions` has an enforced FK to `stores`; seed a store row (`seedStore` helpers exist in the dunning and dispatch test packages).
- Migrations are `.up.sql`/`.down.sql` pairs, embedded via a glob. **Next free number is 000106.** Apply with `DATABASE_URL=... go run ./cmd/migrate up`; the runner reads the env var and has no `-database` flag. Bookkeeping table is `marketplace_db_schema_migrations`.
- Bumping a migration means bumping `ExpectedSchemaVersion` in `migrations.go` — a test guards it.
- Do not edit `internal/email/templates_content.go` (merchant-facing copy, verified byte-for-byte).

---

### Task 1: Stop burning an idempotency slot on an address we never had

**Merchant impact:** highest of the three. All four billing crons claim the
idempotency slot *before* discovering the recipient is undeliverable. During the
`cmd/backfill-email` window that means a merchant whose address lands on day 6
has already permanently lost their day-5 dunning notice — the slot is claimed
and never released.

**Files:**
- Modify: `internal/subscription/dunning/dunning_emails.go`, `trial_reminders.go`, `payment_action_reminders.go`
- Modify: `internal/subscription/lifecycle/winback.go`
- Modify: `internal/subscription/email_claim.go` (add the release)
- Test: the existing integration test files in those two packages

**Design — read this before writing code.** Do NOT hoist `email.ValidateRecipient`
into the crons. Validation lives in the client; callers only classify the error
they get back. That layering was established deliberately in #384 and a previous
attempt to duplicate it was reverted.

Instead, **release the claim when — and only when — the send failed because of
the address**, which the existing classification already tells us:

- `errors.Is(err, email.ErrUndeliverable)` → the address is wrong or absent. This
  is a *recoverable* condition: the backfill or a `customer.updated` webhook may
  supply a real address later. Release the claim so a later run can try again.
- Anything else (`ErrTransport`, `ErrRender`, `ErrNoProvider`) → leave the claim
  burned. At-most-once for transport failures is the deliberate contract from the
  spec: a merchant missing one notice beats a merchant getting two.

Add to `internal/subscription/email_claim.go`:

```go
// ReleaseEmailClaim removes a claim so a later run may retry.
//
// Called ONLY when the send failed because we had no usable address — the
// backfill or a customer.updated webhook may supply one later, and a merchant
// should not permanently lose a notice because their address had not landed
// yet. Every other failure keeps its claim: at-most-once for transport errors
// is deliberate, because a duplicate billing email is worse than a missed one.
func ReleaseEmailClaim(ctx context.Context, db *gorm.DB, subscriptionID uuid.UUID, templateKey, periodKey string) error {
	return db.WithContext(ctx).Exec(`
		DELETE FROM billing_email_sends
		WHERE subscription_id = ? AND template_key = ? AND period_key = ?`,
		subscriptionID, templateKey, periodKey,
	).Error
}
```

In `dunning_emails.go` and `winback.go` (the two using `billing_email_sends`),
call it in the send-error branch guarded on `errors.Is(err, email.ErrUndeliverable)`,
before the `continue`/`return`. Log the release at Info so the retry is visible.

`trial_reminders.go` and `payment_action_reminders.go` use their own per-feature
tables, so add the equivalent targeted `DELETE` for each (`trial_reminders`,
`payment_action_reminders`) under the same guard. Keep each delete keyed by the
full claim tuple that table uses.

- [ ] **Step 1: Write the failing tests**

For dunning, add to `internal/subscription/dunning/dunning_emails_integration_test.go`:

```go
func TestDunning_UndeliverableReleasesClaimSoRetryCanSend(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs", "store_subscriptions", "stores", "billing_email_sends")
	now := time.Now().UTC()

	// A merchant whose address has not been backfilled yet.
	placeholder := "billing+7f3a@mark8ly.local"
	sub := seedPastDueSubscription(t, db, now.AddDate(0, 0, -5), &placeholder)

	client := &stubClient{}
	run := func() *dunning.SendDunningEmails {
		return dunning.NewSendDunningEmails(db, client, nil, &stubVec{}, func() time.Time { return now }).
			WithSkipCounter(&stubSkip{})
	}
	require.NoError(t, run().Run(context.Background()))
	require.Empty(t, client.sent, "mailed an undeliverable address")

	var claims int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM billing_email_sends`).Scan(&claims).Error)
	require.EqualValues(t, 0, claims, "claim was burned; a later run can never deliver this notice")

	// The address lands (backfill or customer.updated), and the same day's run
	// must now be able to deliver.
	good := "merchant@example.com"
	require.NoError(t, db.Exec(`UPDATE store_subscriptions SET email = ? WHERE id = ?`, good, sub.ID).Error)
	require.NoError(t, run().Run(context.Background()))
	require.Equal(t, []string{good}, client.sent, "retry after backfill did not deliver")
}

func TestDunning_TransportFailureKeepsClaimBurned(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs", "store_subscriptions", "stores", "billing_email_sends")
	now := time.Now().UTC()

	addr := "merchant@example.com"
	seedPastDueSubscription(t, db, now.AddDate(0, 0, -5), &addr)

	// A transport failure must NOT release the claim — at-most-once is deliberate.
	client := &stubClient{err: errors.New("sendgrid 503")}
	cron := dunning.NewSendDunningEmails(db, client, nil, &stubVec{}, func() time.Time { return now }).
		WithSkipCounter(&stubSkip{})
	require.NoError(t, cron.Run(context.Background()))

	var claims int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM billing_email_sends`).Scan(&claims).Error)
	require.EqualValues(t, 1, claims, "transport failure released the claim; a retry could duplicate")
}
```

Adjust helper and stub names to what already exists in that file — `stubClient`,
`stubVec`, `stubSkip` and `seedPastDueSubscription` are all present. Note
`stubClient.Send` already mirrors the real client's contract by calling
`ValidateRecipient` first, which is what makes the first test's placeholder
produce `ErrUndeliverable`. If `seedPastDueSubscription` does not return the
seeded row, change it to.

Add the equivalent pair for win-back in
`internal/subscription/lifecycle/winback_integration_test.go`, and one
release-path test each for trial reminders and payment-action reminders in their
existing integration files.

- [ ] **Step 2: Run and confirm they fail**

```bash
cd /tmp/m8-defects/services/marketplace-api && \
TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" \
  go test -tags=integration -p 1 -count=1 ./internal/subscription/dunning/ -run 'TestDunning_Undeliverable|TestDunning_Transport' -v
```
Expected: the release test FAILS with `claims = 1` (slot burned); the
transport test PASSES already (it pins existing behaviour).

- [ ] **Step 3: Implement** `ReleaseEmailClaim` plus the four guarded call sites.

- [ ] **Step 4: Re-run** the four packages' tests, scoped with `-run`, and confirm PASS by name.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "fix(billing): release the claim when a send fails on a missing address"
```

---

### Task 2: Name the column after what it holds

**Files:**
- Create: `migrations/000106_payment_action_reminders_store_id.up.sql` / `.down.sql`
- Modify: `internal/subscription/dunning/payment_action_reminders.go` (the INSERT)
- Modify: `migrations.go` (`ExpectedSchemaVersion` → 106)

`payment_action_reminders.subscription_id` holds a **store id** — `payment_action_reminders.go:127`
inserts `row.StoreID` — while the structurally identical `trial_reminders` inserts
`row.ID`, an actual subscription id. Nothing misbehaves today because store and
subscription are 1:1, but migration 105's own comment says these per-feature
tables should eventually fold into `billing_email_sends`, and whoever does that
will silently migrate store ids into a subscription-id column.

**Rename the column to match the data; do not change what is inserted.** Changing
the inserted value would strand every existing row: merchants who already received
a T-14 reminder keyed by store id would be re-sent it under a new key. The rename
is behaviourally inert and corrects the lie.

- [ ] **Step 1: Write the migration**

`000106_payment_action_reminders_store_id.up.sql`:

```sql
-- payment_action_reminders.subscription_id has always held a STORE id --
-- SendPaymentActionReminders inserts row.StoreID -- while the structurally
-- identical trial_reminders table holds a real subscription id. Nothing
-- misbehaves today because store and subscription are 1:1, but migration 105
-- records the intent to fold both tables into billing_email_sends, and that
-- migration would silently move store ids into a subscription_id column.
--
-- Renaming is behaviourally inert: existing rows keep their values, which were
-- store ids all along. Deliberately NOT changing what the code inserts --
-- that would strand every existing claim and re-send reminders merchants
-- have already had.
ALTER TABLE payment_action_reminders RENAME COLUMN subscription_id TO store_id;
ALTER INDEX IF EXISTS par_subscription_idx RENAME TO par_store_idx;
```

`.down.sql`:

```sql
-- Reverting restores the misleading name. No data changes either way.
ALTER INDEX IF EXISTS par_store_idx RENAME TO par_subscription_idx;
ALTER TABLE payment_action_reminders RENAME COLUMN store_id TO subscription_id;
```

Check migration `000057` for the real index name before writing this and use
whatever it actually declares.

- [ ] **Step 2: Update the INSERT** in `payment_action_reminders.go` to name
`store_id`, leaving `row.StoreID` as the value, and drop any now-stale comment
about subscription ids.

- [ ] **Step 3: Bump** `ExpectedSchemaVersion` to `106`.

- [ ] **Step 4: Apply and verify**

```bash
cd /tmp/m8-defects/services/marketplace-api && \
DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" go run ./cmd/migrate up
docker run --rm postgres:15 psql "postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" -c "\d payment_action_reminders"
```
Expect a `store_id` column and no `subscription_id`. Then run the package's
existing SCA tests scoped with `-run TestSCAReminders` and confirm PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "fix(dunning): rename payment_action_reminders.subscription_id to store_id"
```

---

### Task 3: Get the provider HTTP call out of the advisory lock

**Files:**
- Modify: `internal/billing/dispatch/dispatcher.go`, `handlers.go`
- Modify: `internal/handlers/webhooks/stripe.go`, `internal/billing/dispatch/orphan_resolver.go`
- Test: `internal/billing/dispatch/dispatcher_test.go`

The trial-billed confirmation is sent inside `WithAdvisoryLock`'s transaction
(`handlers.go:369`). That holds a per-store advisory lock and one of a 5-connection
pool across a SendGrid call (15s timeout) and possibly a Resend fallback, against
Stripe's 30s webhook timeout. It is not a correctness bug — the non-transactional
claim added in #384 prevents duplicates — but it is a latency and pool-starvation
risk on the hot webhook path.

**Design.** The transaction boundary is already clean:
`stripe.go:161` is `WithAdvisoryLock(ctx, db, storeID, func(tx) error { return Dispatch(ctx, tx, evt) })`,
so everything after that call has committed. Defer the send to there.

Do **not** store pending sends on the `Dispatcher` struct — it is shared across
requests and that would be a data race. Carry them per-request in the context:

- Add a small collector type in `dispatch` with a context key, a
  `WithDeferredSends(ctx) (context.Context, *DeferredSends)` constructor, and an
  `Add(func() error)` / `Run(ctx) []error` pair.
- `handleInvoicePaid` keeps the claim inside the transaction (the claim is already
  on the non-transactional handle and must stay there), and appends the actual
  `Send` to the collector instead of calling it inline.
- `stripe.go` and `orphan_resolver.go` create the collector before
  `WithAdvisoryLock` and drain it **after** the lock returns without error. A send
  error must stay non-fatal: log it, do not fail the webhook, or Stripe retries
  every other side effect.

If you find a materially simpler shape that keeps the send outside the lock and
non-fatal, take it and explain the choice in your report.

- [ ] **Step 1: Write the failing test**

Add to `dispatcher_test.go` a test asserting the send happens *after* commit — for
example, a stub email client that queries `store_subscriptions.first_charge_at`
when `Send` is invoked and records whether it was already visible. Inside the
transaction it is not yet committed and so invisible to a separate connection;
after commit it is. Assert it was visible, which is only true if the send moved
out of the transaction.

- [ ] **Step 2: Run and confirm it fails** (`-run TestInvoicePaid` scoped).

- [ ] **Step 3: Implement** the collector and rewire both call sites.

- [ ] **Step 4: Re-run** the whole `dispatch` package scoped to `-run TestInvoicePaid|TestDispatch|TestHandleCustomerUpdated` and confirm the previously passing tests still pass — especially `TestInvoicePaid_TrialBilled_NotResentAfterRollback`, which pins that a rollback cannot double-send.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "fix(billing): send the trial-billed confirmation after the webhook transaction commits"
```

---

## Final verification

- [ ] `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `gofmt -l .` all clean.
- [ ] Full integration suite, diffed against `1dc04a87` in **both directions**. Baseline at that commit is **187 unique failures / 22 packages** (measured during #384). New failures must be an empty set — check names, not the exit code, and check for `--- SKIP` masquerading as success.
- [ ] `ps aux | grep "[g]o test -tags=integration"` before starting a full run — two suites against one database corrupt each other.
