# Apply and remove a console-minted discount on a tenant's subscriptions

mark8ly#660, counterpart to tesserix-home#331. The console **mints** a Stripe
Coupon and records it (`0047`); it cannot apply it, because the Stripe customer
lives here. This builds the endpoints that apply and remove it.

PR 1 of 3. The transport (tesserix-home platform-api billing write path) and the
console call are separate PRs; this one ships unreachable, exactly as the
console's own half did.

## Two premises in #660 are wrong. Read this before the issue.

**1. There is no customer-scoped apply call.** The SDK pins
`APIVersion = "2025-08-27.basil"` (`stripe-go/v82@v82.5.1/api_version.go`), and
Basil **removed** coupon attachment from the Customer API — `CustomerUpdateParams`,
`CustomerCreateParams` and `CustomerParams` carry no `Coupon`, `PromotionCode`
or `Discounts` field. (v76 still has `Coupon *string`; the v82 CHANGELOG records
the removal.) Only read (`Customer.Discount`) and delete
(`V1Customers.DeleteDiscount`) survive.

**2. A customer discount would not have stacked anyway.** Stripe: *"When a
subscription has no discounts, the customer-level discount, if any, applies to
invoices."* Customer-level is a **fallback, masked by** any subscription
discount. A tenant whose store had redeemed a merchant promo would have received
**no platform discount at all, silently** — worse than the collision it was
meant to avoid.

## The design: read-modify-write the subscription's `discounts` array

Basil's replacement is a stackable array (up to 20), whose entries are a
discriminated choice (`SubscriptionUpdateDiscountParams`):

```go
Coupon        *string // create a new discount from this coupon
Discount      *string // REUSE an existing discount already on the object
PromotionCode *string
```

The `Discount` arm is what makes this non-destructive:

- **Apply** — read current `discounts`, write back
  `[existing ids as Discount…] + [our coupon as Coupon]`.
- **Remove** — read current, write back the array **minus ours**, preserving the
  rest.

### This fixes a live bug, it does not work around one

`AttachCoupon` (`internal/billing/stripe/coupon.go:93`) sets the coupon as the
**sole** discount — its own comment says so — and `DetachCoupon` (`:107`) sends
an empty slice, clearing **all** discounts. `internal/promo/service.go:152`
attaches merchants' promo codes through those same helpers, and already knows
the hazard (`:180`: *"detaching here would strip an unrelated coupon the
subscription already carried"*).

So today an operator override would delete a merchant's promo, and a revoke
would delete whatever discount happened to be there. The read-modify-write pair
replaces them. **Migrate `internal/promo`'s call sites in this PR** — leaving two
mechanisms against one Stripe field is how the collision comes back.

## Scope: all current AND future stores (decided)

`store_subscriptions` is `UNIQUE (store_id)` with a separate `tenant_id`
(`migrations/000015_subscriptions.up.sql`), so a tenant with several stores has
several subscriptions and several customers. #660 says "the tenant's
subscription customer", singular; it is not.

Decided: the grant is a standing property of the tenant.

- Apply fans out over every store subscription the tenant owns, and reports
  **per store** — a store with no subscription yet is an explicit outcome, not a
  silent skip (`internal/billing/appaddon/handler.go:133` is the precedent for
  the guard).
- **`StripeSubscriptionID` is nullable** (`internal/subscription/models.go:116`).
  A trialing, card-less tenant has nothing to attach to — and is exactly the
  population an operator discounts. Those stores report `pending`, and the
  override is applied when the subscription is created (T6).
- Remove fans out the same way and is as audited as the apply
  (tesserix-home#331: "removal is as audited as application").

## One transaction per store, not one across the tenant

`internal/billing/trial/extend.go:232-357` puts the Stripe call **inside** the
transaction, holding `SELECT … FOR UPDATE` on the subscription row across the
network call, bounded by `stripeCallTimeout = 10s` (`:139`). Its reasoning
(`:270-280`) is that Stripe must move first and be the source of truth: a Stripe
failure rolls back and writes nothing locally; a failed commit afterwards leaves
Stripe **ahead**, which bills the merchant *later* than we show — the safe
direction.

Follow that, but **per store**. One transaction spanning N stores would hold N
row locks across N Stripe round-trips. Each store gets its own
transaction-with-Stripe-call, so one store's failure neither rolls back nor
blocks the others, and the per-store report is the honest unit.

## Tasks

Each is one atomic commit. Tests first.

### T1 — `EmitTx` on the audit emitter

`internal/audit/emitter.go`. tesserix-home#331 requires the audit row be written
**inside** the transaction that applies the change.

Cheaper than it looks: `Repository.Create(ctx, db *gorm.DB, e *Entry)`
(`repository.go:43`) **already takes the handle as a parameter** and
`gormRepository` is stateless (`:55`). `EmitTx` is `EmitSync` with the handle
passed in. Nothing touches the queue, worker or `stop`.

- Must live on `*Emitter` in package `audit`: `buildEntry` is unexported by
  design (`export_test_helpers.go:6`) and owns all actor/operator/capability/IP
  derivation. Do not re-derive it in `platformadmin`.
- **Takes the caller's `ctx`**, contradicting both neighbours. `Emit`'s worker
  and `EmitSync` deliberately use a fresh `context.Background()` with a 5s
  timeout so a disconnecting client cannot cancel the record (`:184-187`,
  `:236-240`). That is correct for them and **wrong here** — the insert must
  share the transaction's cancellation. Say so in the doc comment.
- Nil-receiver: returns an **error**, never a silent no-op —
  `nil_repo_test.go`'s `TestNilEmitter_AllExportedMethodsAreSafe` gains a
  twelfth method, and a transactional audit that quietly does nothing is the
  exact failure #331 exists to prevent.
- Do **not** use the `emailtemplates` revision-row pattern (migration 000130).
  That exists only because an email template key is estate-wide and
  `audit_logs.tenant_id` is `NOT NULL` (`routes.go:371-401`). A tenant discount
  has a real tenant id, so that blocker never fires and a second governance
  store would fragment the trail.

### T2 — non-destructive Stripe discount helpers

`internal/billing/stripe/coupon.go`.

- `AddSubscriptionDiscount(ctx, c, subID, couponID)` — retrieve, map existing
  discounts to `{Discount: id}`, append `{Coupon: couponID}`, update.
- `RemoveSubscriptionDiscount(ctx, c, subID, couponID)` — retrieve, write back
  without that one.
- **Already present is a no-op, absent is a no-op.** Both must be safe to retry;
  the endpoint's idempotency layer is a separate guarantee and must not be the
  only one.
- Stripe caps the array at 20 — refuse with a named error rather than letting
  the API reject it, so the message says which subscription.
- Then **replace `AttachCoupon`/`DetachCoupon` and migrate `internal/promo`**
  (`service.go:152`, `:182`, `:204`). Delete the old pair; leaving it is leaving
  the foot-gun loaded.

### T3 — the domain service

`internal/billing/tenantdiscount/` (new package).

`Apply(ctx, in) (Result, error)` and `Remove(ctx, in) (Result, error)`. Per
store: open a transaction, `SELECT … FOR UPDATE` the subscription row, call
Stripe inside it (bounded, per `stripeCallTimeout`), write local state if any,
then `EmitTx` the audit row **last, inside the same transaction**.

**Name the residual divergence.** If the audit insert fails, the transaction
rolls back but the Stripe discount **stays applied**. That is the same class as
`ErrStripeAppliedLocalWriteFailed` (`trial/extend.go:83-90`) and needs its own
sentinel, its own log line carrying coupon + subscription + customer ids, and a
stated decision: an unattributable discount is rolled back locally rather than
kept silently. Do not let this fall out of ordering by accident.

Per-store outcomes, each explicit: `applied`, `already_applied`, `pending`
(no `stripe_subscription_id` yet), `failed` with a reason.

### T4 — the platform-admin endpoints

`internal/handlers/platformadmin/billing_tenant_discount.go`. Model on
`billing_trial_extend.go` — it is a complete precedent.

- `POST /admin/billing/tenants/:tenantID/discount` and `DELETE` the same path.
  **Bare UUID** tenant id, like every handler here (`tenant_lifecycle.go:200`);
  the console's namespaced `<source>:<id>` is split by platform-api (PR 2).
- Mandatory scoped `Idempotency-Key`, reserved **after** validation and
  immediately before the work (`billing_trial_extend.go:189-206`), scoped
  `"tenant_discount:" + tenantID + ":" + key` because the table's PK is
  service-wide. Release on domain failure so a corrected retry proceeds.
- Mandatory `reason`, `truncateUTF8`'d before it reaches metadata
  (`:349-362`) — under `EmitTx` an unmarshalable reason no longer means
  "succeeded unaudited", it means "the discount silently failed".
- Distinct error codes per domain sentinel; driver text logged, never echoed.
- **Add both routes to `RequiredWriteCapabilities`**
  (`middleware.go:104-119`), value `""` — the vocabulary is the console's
  (#275/#333). `TestAllWriteRoutesDeclareACapability` fails the build otherwise,
  and once `CapabilityValueChecked` flips an undeclared write route 403s.
- Mount conditionally in `routes.go`, refusing to mount without the emitter —
  "a write endpoint that cannot be attributed should not exist."

### T5 — tests

Both layers, per the package convention: unit with hand-rolled stubs
(`billing_trial_extend_test.go`) and `//go:build integration` against a real DB
(`billing_trial_extend_integration_test.go`, `testdb.NewDB(t, "idempotency_keys")`).
`./internal/handlers/platformadmin/...` is already in `make test-int`'s list.

Must cover: the fan-out reporting per store; a `pending` store; same
idempotency key replaying without a second Stripe call or a second audit row;
the audit row written in-transaction (assert it is absent after a forced
rollback); and that applying an override **preserves an existing merchant promo
discount** — the regression the whole design exists for.

### T6 — apply on subscription creation

The "future stores" half. Where a subscription is created for a store whose
tenant holds a live override, apply it. Find the creation path and hook it
there; without this the discount silently stops covering the tenant as they
grow, which surfaces in a renewal negotiation.

## Out of scope, stated so it is not smuggled in

- **The transport.** tesserix-home's platform-api billing module is read-only —
  no `fed.Post`, no `splitTenantID`. PR 2.
- **The console call and its copy.** `tenant-pricing-override-controls.tsx:62,84,94`
  flag `overrideMintedMessage` as MUST-REWRITE when attach lands. PR 3.
- **"At most one active override per tenant", enforced here.** #660 asserts it,
  but there is no local table to enforce against — the grant record lives in the
  console's `0047`. This PR enforces *idempotent application*, not global
  uniqueness. Say so rather than implying the stronger guarantee.
- **Customer-level discounts.** Not creatable at this API version. `DeleteDiscount`
  stays useful only for cleaning up pre-Basil legacy discounts; not this endpoint.

## Global constraints

- **Comment accuracy.** Run the command before writing the sentence that
  describes it; count anything you assert a count about. Do not state a rule
  more broadly than the code implements it.
- Do not weaken an existing assertion. Do not touch `apps/`.
- `go build ./...`, `go vet ./...` and `go test ./...` from
  `services/marketplace-api`, all clean, before each commit.
