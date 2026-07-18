# Refund / Order-Cancel → Shipment Lifecycle Actions

**Date:** 2026-07-18
**Status:** Approved design, ready for implementation planning
**Repo:** mark8ly · service `services/marketplace-api`

## Problem

Issuing a refund (or cancelling an order) does **nothing** to the order's
carrier shipment. A merchant refunds an order, but the Delhivery pickup stays
scheduled and the label stays live — the courier still comes, or the parcel
still ships. Observed live 2026-07-18: label created, full refund issued, the
shipment was never cancelled in Delhivery.

`CancelShipment` is defined on every carrier interface
(`internal/shipping/carrier.go:18`) and implemented (Delhivery
`internal/shipping/delhivery.go:584`, ShipEngine, NinjaVan) — but **called by
nothing**. The capability exists; it was never wired to any flow.

## Goal

When an order is fully refunded or cancelled, take the correct carrier action
for the shipment's **current lifecycle state** — cancel it if it hasn't moved,
return it if it's in transit, or arrange a reverse pickup if it's delivered.
Provide a manual button for cases the automation deliberately skips.

## Non-goals

- Partial-refund auto-cancellation (explicitly excluded — see Trigger model).
- A general Returns/RMA product. Phase 3 reuses reverse-pickup plumbing but is
  scoped to refund/cancel-driven returns, not customer-initiated RMA.
- Non-Delhivery carrier RTO/reverse specifics. The decision layer is
  carrier-agnostic; Phase 2/3 carrier calls are implemented for Delhivery
  first, other carriers no-op with a clear "unsupported" outcome.

## State → action matrix

The shipment's state selects the action. Delhivery status types (from
`MapDelhiveryStatus`, `delhivery.go:1007`) map to our shipment `status` column
values (`pending` → `created` → `in_transit` → `out_for_delivery` →
`delivered`).

| Shipment state | Delhivery status type | Action | Phase |
|---|---|---|---|
| No shipment / no label | — | Noop (refund only) | 1 |
| Manifested, pickup pending (`pending`/`created`) | `PP` (Pickup Pending) | **Cancel forward shipment** — `CancelShipment` (`/api/p/edit` `cancellation:true`) → `CN` | 1 |
| Picked up / in transit (`in_transit`/`out_for_delivery`) | `PU`/`UD` | **Auto-trigger RTO** (return to origin); fall back to a manual notice if unsupported for the route | 2 |
| Delivered (`delivered`) | `DL` | **Create reverse pickup** — reuse order-creation API with `payment_mode:"Pickup"` + return keys; auto-schedules (no separate pickup request) | 3 |

## Trigger model

| Trigger | Auto-runs the state action? |
|---|---|
| Full refund (post-refund `payment_status == refunded`) | **Yes** |
| Order cancel | **Yes** |
| Partial refund (`partially_refunded`) | **No** — still shipping un-refunded items |
| Manual "Cancel / return shipment" button | **Yes** — on demand; covers partial-refund cases and any manual need |

Full-vs-partial is already computed by `orderrefund.DeriveStatus`
(`coordinator.go:87`): `refunded` = full, `partially_refunded` = partial.

## Architecture

### Decision layer (pure, carrier-agnostic)

New unit — proposed `internal/shipmentcancel/` (or a method group on the
shipping service; implementer's call, keep it isolated and unit-testable):

```
ResolveAction(shipmentStatus string) Action
  → ActionNoop | ActionCancelForward | ActionTriggerRTO | ActionReversePickup
```

Pure function of the shipment status string. Every state → action pair is a
table test. This is the heart of the feature and must be independently
testable with zero carrier/DB dependencies.

### Executor layer

`Execute(ctx, action, shipment, carrier) Outcome` dispatches to the carrier:

- `ActionCancelForward` → `carrier.CancelShipment(trackingNumber)` (exists)
- `ActionTriggerRTO` → new `carrier.ReturnToOrigin(...)` (Phase 2; Delhivery
  endpoint TBD-verify — see Open questions)
- `ActionReversePickup` → new `carrier.CreateReverseShipment(...)` reusing the
  order-creation call with `payment_mode:"Pickup"` (Phase 3)

Carriers that don't implement an action return an `unsupported` outcome, not an
error — the caller surfaces "arrange manually with the carrier."

### Hook points

- **Refund coordinator success path** (`orderrefund/coordinator.go`,
  `Refund()` after a `RefundResult` with full status). Best-effort call into
  the executor; failure never affects the refund result.
- **Order cancel handler** (`handlers/admin/orders.go:416` `Cancel`). Note:
  today `shipmentBlocksCancel` (`orders.go:133`) *blocks* cancel when a
  shipment exists. This feature **changes that interaction** — instead of
  blocking, cancel should cancel/return the shipment per the matrix. Reconcile
  this explicitly during implementation.
- **New manual endpoint** — `POST /admin/.../orders/:id/shipments/:sid/cancel`
  (or similar) for the button. Runs `Resolve` + `Execute` for that shipment.

### Per-shipment, per-order

An order can have multiple shipments. Resolve + execute **per shipment**
independently; aggregate outcomes for the response.

## Data model

Record the carrier action outcome so the admin sees per-shipment status and a
sweep can retry failures. Add to the `shipments` table (migration; bump
`ExpectedSchemaVersion`):

- `cancel_action` — enum: `none | cancel_forward | rto | reverse_pickup`
- `cancel_status` — enum: `none | requested | succeeded | failed | unsupported`
- `cancel_reason` — text, Delhivery's short message on failure (never raw body)
- `cancel_requested_at` — timestamptz

Reverse pickups (Phase 3) create a **new shipment row** (the reverse leg) linked
to the order; the forward shipment keeps its delivered state.

## Error handling (critical)

- **Best-effort, never blocks the refund.** The money already moved. A carrier
  failure logs + records `cancel_status=failed` + surfaces a notice. It must
  never roll back or fail the refund. Mirrors the existing warehouse-sync
  pattern (`handlers/admin/settings.go syncWarehouseAsync`).
- **Clean merchant messages** — use `delhiveryWarehouseMessage`-style extraction
  (already built, `delhivery.go`) so the UI shows Delhivery's short reason, not
  the raw XML/JSON body (which leaks address/phone/hours).
- **Retry sweep** — a cron re-drives `cancel_status=failed` rows, same shape as
  the existing refund-sweep cron. (Phase 1 can log-only; add sweep in a later
  slice if failures prove common.)

## Testing

- **Decision layer:** table test every shipment status → expected action.
- **Executor (Phase 1):** httptest mock of Delhivery `/api/p/edit`; assert
  `CancelShipment` called with the waybill, 2xx = success, non-2xx → clean
  reason recorded, refund unaffected.
- **Trigger integration:** full refund → cancel action fires; partial refund →
  no action; order cancel → action fires; manual endpoint → action fires.
- **Best-effort guarantee:** carrier returns 500 → refund still succeeds,
  `cancel_status=failed` recorded.
- Follow existing patterns: `delhivery_test.go` (httptest carrier mocks),
  `orderrefund` tests.

## Phasing

Spec covers all three; implement in order, each shippable independently.

### Phase 1 — Pre-pickup cancel (the immediate bug fix)
Decision layer + `ActionNoop`/`ActionCancelForward` + data-model columns +
refund/order-cancel/manual hooks + reconcile `shipmentBlocksCancel`. Uses the
existing `CancelShipment`. **This resolves the reported bug.**
**Open item:** verify whether cancelling the waybill auto-closes the scheduled
pickup (`pickup_request_id`) or needs a separate Delhivery pickup-cancel call.

### Phase 2 — In-transit RTO
`carrier.ReturnToOrigin` for Delhivery + `ActionTriggerRTO` + auto-trigger on
in-transit state with a manual-notice fallback when unsupported.
**Open item:** confirm the exact Delhivery intercept/RTO endpoint against a live
in-transit shipment (Cancel Order API vs NDR API — see Open questions).

### Phase 3 — Reverse pickup for delivered
`carrier.CreateReverseShipment` reusing order-creation with
`payment_mode:"Pickup"` + return keys (auto-schedules). Creates a new reverse
shipment row. `ActionReversePickup` + delivered-state auto-trigger.

## Open questions (verify during implementation — not placeholders)

1. **Pickup-request cancel (P1):** does `CancelShipment` (waybill) auto-close
   the scheduled pickup, or is a separate call needed? A pickup can cover
   multiple shipments — don't cancel a pickup with other live waybills.
2. **RTO endpoint (P2):** Delhivery exposes a Cancel Order API and an NDR API;
   confirm which handles in-transit RTO/intercept and its payload.
   Docs: https://delhivery-express-api-doc.readme.io/reference/package-lifecycle-1
3. **Reverse pickup payload (P3):** exact return-key set for
   `payment_mode:"Pickup"`.
   Docs: https://delhivery-express-api-doc.readme.io/reference/reverse-pickups

## Key references

- `internal/shipping/carrier.go:18` — `CancelShipment` interface
- `internal/shipping/delhivery.go:584` — Delhivery `CancelShipment` (`/api/p/edit`)
- `internal/shipping/delhivery.go:1007` — `MapDelhiveryStatus`
- `internal/orderrefund/coordinator.go:87` — `DeriveStatus` (full vs partial)
- `internal/orderrefund/coordinator.go:112` — `Refund` (success hook point)
- `internal/handlers/admin/orders.go:416` — order `Cancel`
- `internal/handlers/admin/orders.go:133` — `shipmentBlocksCancel` (reconcile)
- `shipments` table: `tracking_number`, `pickup_request_id`, `status`
