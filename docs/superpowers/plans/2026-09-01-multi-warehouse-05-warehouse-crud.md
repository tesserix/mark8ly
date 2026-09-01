# #177 PR 5 — Warehouse CRUD, archiving, and per-location stock

Spec: `docs/superpowers/specs/2026-08-31-multi-warehouse-allocation-design.md`
Depends on: PRs 1–4b (merged: #488, #490, #491, #492, #495)
Blocks: PR 6 (sentinel backfill, retiring `DefaultLocationID`)

---

## Correction to the spec before anything else

The spec's Risks section says:

> Nobody is using warehouses yet. Production has zero warehouses and zero
> carrier configs.

**That is no longer true, as of 2026-09-01.** `the-bondi-store` now has:

- a warehouse `52b67b6b-b85d-496d-9466-ee95fbf9f1ee`, named exactly
  **`Main Warehouse`**, at 1 Campbell Parade, Bondi Beach NSW 2026 AU; and
- an active ShipEngine carrier config (test mode) bound to it.

Two consequences, both load-bearing:

1. **The PR 6 backfill is no longer greenfield.** The spec has it *create* a
   warehouse per stock-holding store with `name = 'Main Warehouse'`. Bondi
   already has one under that exact name, and `warehouses` is keyed
   `(store_id, name)` — so a naive `INSERT` either violates the constraint or,
   worse, an `ON CONFLICT DO NOTHING` silently leaves stock pointing at the
   sentinel while the code believes it was migrated. The backfill must be
   idempotent against an existing `Main Warehouse` and must be tested against
   a database that already has one.
2. **There is now one real store to validate against.** The spec's claim that
   "integration coverage carries the whole weight" is weaker than it was: an
   end-to-end allocation on bondi is possible once a payment method exists.

---

## Goal

A merchant manages warehouses as first-class objects, instead of typing an
address into a carrier form and hoping the name matches.

## Why this is the shape

Today `ShippingConfigForm` embeds `AddressFieldset`, and saving a carrier
config upserts a warehouse behind it, keyed on `(store_id, name)`. A merchant
who types a different name gets a **second, stockless warehouse** rather than
an edit — allocation then reports nothing available and the order never ships.

#505 made that trap visible (placeholder + warning) and #508 refused to save an
unquotable config, but both are mitigations. A free-text field that must
exactly match an existing record is the wrong contract. This PR removes it.

---

## Slices

Each is independently reviewable. 5a–5c can land separately; 5d depends on 5a.

### 5a — schema + repository

**Migration `000121`** (note: `000120` is taken by the parcel-weight setting
merged in #514; confirm the next free number before writing):

- `warehouses.archived_at timestamptz NULL`
- partial unique index on `(store_id, name) WHERE archived_at IS NULL`

  The existing uniqueness must become partial, or archiving a warehouse
  permanently burns its name — a merchant who archives "Main Warehouse" can
  never create another. **Write the down migration to restore the total index,
  and check no two live rows would collide before doing so.**

**`internal/warehouse` gains:**

| method | rules |
|---|---|
| `List(storeID, includeArchived)` | ordered by `priority`, then `name` for determinism |
| `Delete(id)` | refused while the warehouse holds stock or owes an unshipped parcel |
| `Archive(id)` | the removal mechanism for a warehouse with allocation history |
| `SetPriorities([]{id, priority})` | one transaction; partial application is not acceptable |

**The deletion/archive split is the subtle part.** Per the spec, a warehouse
with *any* allocation history — even fully shipped — is archived, never
deleted; `ON DELETE RESTRICT` on `order_allocations.warehouse_id` is the hard
backstop. So `Delete` applies only to a warehouse with no history at all.
`Archive` is what the UI's "remove" actually calls in every other case, and
the UI must not present two verbs the merchant has to choose between.

**Mutation-test every refusal.** Delete the guard, watch a test fail. At
minimum: delete-with-stock, delete-with-unshipped-parcel, and
delete-with-allocation-history-must-archive-instead.

### 5b — admin API

`/api/v1/admin/stores/:storeId/warehouses` — list, create, update, delete,
reorder.

- Archived warehouses are excluded from list by default and **must not appear
  in the allocator's candidate set**. Verify that directly; an archived
  warehouse still holding a stock row would otherwise keep receiving
  allocations.
- Reorder takes the full ordered set, not a delta — a delta over a list that
  changed underneath is the classic retry bug.

### 5c — settings/warehouses page

List, create, edit, archive, mark default, drag to set priority.

### 5d — carrier form drops the address

`ShippingConfigForm` loses `AddressFieldset` and gains a **warehouse picker**.

- Exactly one warehouse → bind to it and show it read-only. No choice to make.
- More than one → a select.
- None → link to the warehouses page rather than inlining a create form.

Delete `lib/settings/warehouse-validation.ts` (from #508) in this slice: the
phone requirement moves to the warehouse form, where the address now lives.
**Do not leave both** — two validators for one rule diverge.

`lib/settings/shipping-readiness.ts` (#511) must be updated in step: its
`no_warehouse_address` and `no_warehouse_phone` blockers read carrier-config
fields that no longer hold the address.

### 5e — per-location stock editing

One rule from the spec: **a store with one warehouse sees exactly what it sees
today.** The product form keeps its single stock field until a second
warehouse exists, then expands to a per-warehouse breakdown showing the total.

`UpdateVariantStockInTx` stays the single mutation chokepoint, guarded by its
existing test. It already takes a location; the change is carrying a real
warehouse id down from the API instead of `DefaultLocationID`.

---

## What this PR must NOT do

- **Not** retire `DefaultLocationID` or rewrite `location_id`. That is PR 6,
  and it must not land until this is confirmed running everywhere.
- **Not** change the allocator. PR 2 owns it.
- **Not** touch the `shipments.shipment` → list response shape. That is its own
  public-contract change.

---

## Testing

- Integration tests are `//go:build integration`, gated on `TEST_DATABASE_URL`
  (never `TEST_DB_DSN`), run with `-p 1`. **A skip prints `ok` and reads
  exactly like a pass** — any claim about an integration run must name the DSN
  and the duration.
- Go commands from `services/marketplace-api`, never path-scoped, always
  `-count=1`, including `go vet -tags=integration ./...` — it is the only one
  that compiles build-tagged files, and it has already caught two interface
  breaks this week that `go build` missed.
- Bump `ExpectedSchemaVersion` with the migration. A guard test enforces it;
  forgetting fails service startup, not CI.
- Every guard mutation-tested.

## Risks

- **The partial unique index is the quiet one.** Get it wrong and archiving
  burns a name forever, or two live warehouses share one and the upsert starts
  hitting the wrong row.
- **Archived-but-stocked warehouses.** Archiving does not move stock. Decide
  explicitly whether archiving is refused while stock remains, or whether the
  allocator must exclude archived rows — and test whichever is chosen. Doing
  neither is an oversell.
- **Two validators for the phone rule** if 5d lands without deleting #508's.
- **The bondi warehouse is real data now.** Any migration touching
  `warehouses` runs against a row a live carrier config points at.

## Delivery

5a → 5b → 5c → 5d → 5e, each its own PR. Per CLAUDE.md, execute with
subagent-driven development (superpowers:subagent-driven-development), with
review between tasks.
