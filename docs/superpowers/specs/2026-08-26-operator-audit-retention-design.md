# Design — retention for operator audit rows (#365)

`tenantpurge.purgePlan` deletes a tenant's `audit_logs` **except** rows with
`actor_type = 'operator'` (`internal/tenantpurge/purge.go:370`). That exclusion
is load-bearing: without it the outbox backstop's second purge destroys the
record of the destruction it is backing up (#288). Those rows therefore outlive
the tenant they describe, and nothing has ever deleted them.

Everything below was measured on 2026-08-26 against production and the code at
`27303f81`. Where this document contradicts #365, the issue is the thing that
was wrong, and those places are called out.

## What #365 gets wrong, corrected against production

- **There are THREE operator rows, not two.** All three are `tenant.purged`,
  all from #288's verification on 2026-08-25, and all three tenant ids resolve
  to **zero** surviving stores.
- **`tenant_name` is NOT in the metadata.** #365 lists it among the retained
  fields. The keys actually present are: `capability`, `reason`, `reason_code`,
  `store_ids`, `store_slugs`, `tables`, `total_rows`. The identifying surface is
  therefore narrower than the issue states — but `store_slugs` and the free-text
  `reason` remain, and `reason` is the sharp edge.

## The structural fact #365 does not state

**A retention engine already exists**, and it is blind to these rows.

`internal/audit/prune_cron.go` prunes `audit_logs` nightly at 02:00 UTC against
per-plan windows derived from `plangate` — 90 days for trial and starter, 365
for studio, unlimited for pro, marketplace not pruned. It is not missing.

It cannot reach operator rows for two independent reasons:

1. Its DELETE **joins `store_subscriptions` on `store_id`** so that each row is
   pruned against its own tenant's plan. Operator rows on this surface carry
   `store_id = NULL` **deliberately** — a store-scoped audit row would surface a
   platform operator's action inside the merchant's own store-scoped audit view,
   which is a product decision the trial-extend handler declined to make by
   default (`billing_trial_extend.go`).
2. Even were `store_id` set, a purge deletes that tenant's `store_subscriptions`
   rows, so the join would match nothing afterwards.

So "no retention policy" understates it: the policy machinery exists and is
structurally incapable of seeing these rows. Any fix needs a separate path
regardless of which retention number is chosen. This is #311's "store-less audit
rows are unprunable", observed from the other end.

## Decisions

### 1. Seven years, matching `billing_archive`

Operator rows are retained **7 years from `created_at`**, then deleted.

The number is not invented. `billing_archive` is documented as *"retained 7
years after hard-delete under legal-obligation basis"* (migration
`000046_billing_archive.up.sql:24`, §23.2). An operator governance record about
a destruction is the same class of artefact under the same basis. Reusing the
number means the estate has ONE retention story to defend rather than two that
must be reconciled.

### 2. The rule covers ALL operator rows, not only orphaned ones

Not a widening for its own sake. Because operator rows carry `store_id = NULL`,
**every** operator row is unprunable today — `tenant.suspended`,
`tenant.unsuspended` and `trial.extended` included, whether their tenant still
exists or not. A rule scoped only to purged tenants would leave the others
accumulating forever: the same defect with a smaller blast radius and a more
confusing explanation.

### 3. Free text is never written on an erasure purge

When `reason_code = "erasure_request"`, the free-text `reason` is **not written
to the database at all**. The response tells the operator so explicitly.

**Why never-write rather than strip-later.** CloudNativePG backs the cluster up
to GCS continuously (barman, 3-day PITR retention). Text that exists even
briefly is captured there, so stripping it from the row later does not remove
it from the estate. "We never recorded it" is both stronger and easier to
explain than a two-stage rule.

**Why erasure only.** `PurgeReasonCodes`' own comment says `merchant_request`
and `erasure_request` are kept distinct because *"only the second carries a
statutory clock, and an audit trail that cannot tell them apart cannot answer
the question a regulator asks."* On an art.17 erasure, the surviving row is the
one most likely to contain what the erasure was for — an operator writing "cust
asked, jane@example.com, ticket 4471" into a field that then outlives the
deletion by seven years. Every other reason code keeps its free text: a support
escalation or a fraud investigation is exactly where that context earns its
place.

**Why not refuse the request.** Refusing a purge because of a non-empty text
field would block a destructive operation with a statutory deadline on a
formatting objection. An operator retrying an art.17 purge against the clock is
a worse outcome than a discarded sentence.

**Why tell the operator.** Silently discarding text the operator believes they
recorded is its own trap — they would think the reasoning was documented. The
response says it was not.

### 4. `store_slugs` and `store_ids` are retained

They are what make the row auditable: without them it cannot be tied to what was
destroyed, which defeats keeping it. A slug is a merchant's brand — business
identity, and the subject here is a merchant rather than a consumer.

## What gets built

**A separate prune path in `internal/audit`, additive to the existing cron.**

It shares `PruneSpec` and `PruneBatchSize` but has its own DELETE with **no
join**: `actor_type = 'operator' AND created_at < cutoff`, where cutoff is
`now() - 7 years`. It reports under its own metric label so operator pruning and
plan-based pruning are distinguishable in monitoring.

The existing per-plan `pruneBucket` path is **not modified**. Adding a fourth
bucket would mean special-casing the very join the function is built around, and
the two rules have genuinely different shapes: one is plan-derived and
store-scoped, the other is flat and store-less.

**The write-side carve-out** lives in the purge handler, where the audit
metadata is assembled: on `erasure_request`, the `reason` key is omitted from
the metadata map entirely, and the response carries a field stating that free
text was not retained.

### This narrows #311

#311 records the decision that store-less audit rows are **never** pruned. That
decision now stands for everything EXCEPT `actor_type='operator'`. The narrowing
is deliberate and must be written where a reader will find it — in the prune
code and on #311 itself — rather than left as two documents quietly disagreeing.

## Testing

- **The carve-out is a negative, so assert it on the persisted row**, not on the
  response: purge with `reason_code=erasure_request` AND a non-empty `reason`,
  then assert the stored metadata has **no `reason` key at all** — absent, not
  empty string.
- **The discriminating pair** (trap 13): the SAME free text under
  `merchant_request` MUST persist. One reason code cannot prove a per-code rule;
  without both, the test passes against an implementation that drops `reason`
  unconditionally.
- **The prune boundary, on the boundary**: a row at 7 years minus one second
  survives; at 7 years plus one second it is deleted. "Close to the edge" is not
  the edge.
- **The negative guard**: a store-less row with `actor_type <> 'operator'` is
  NOT deleted by the new path — otherwise the narrowing of #311 is wider than
  stated.
- **The existing prune is unmoved**: its tests must pass unchanged, and a
  plan-scoped row must still be pruned on its own window.
- Verification set: `go vet -tags=integration ./...`, `-p 1 -count=1`,
  `TEST_DATABASE_URL`, and `set -o pipefail` wherever an exit code is evidence.

## Out of scope

- Redacting or dropping `store_slugs` / `store_ids` (decided: retained).
- Changing the `actor_type <> 'operator'` exclusion in `purgePlan` — load-bearing
  for #288.
- Retrospective treatment of the three existing production rows: they are #288
  verification artefacts containing no real customer data, and they age out under
  the same 7-year rule as everything else.
- #259's erasure-request workflow and its statutory clock. This design makes the
  surviving row safe; it does not implement erasure handling.
