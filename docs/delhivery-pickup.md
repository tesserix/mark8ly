# Delhivery pickup scheduling

## What this is

When a merchant clicks **Create shipping label** on an order, mark8ly
previously generated a Delhivery waybill via `POST /api/cmu/create.json`
and stopped there. The waybill stayed at the **Manifested** status on
one.delhivery.com until the merchant manually opened the dashboard and
clicked **Add to Pickup** — which then routed a pickup executive to the
warehouse.

This doc covers the second half of that flow: mark8ly now calls
Delhivery's pickup-request endpoint right after the label is created so
the waybill advances to **Pickup Scheduled** automatically, and exposes
a manual **Reschedule pickup** path for exception cases.

## The endpoint we call

```
POST https://track.delhivery.com/fm/request/new/
Authorization: Token <api-key>
Content-Type: application/json

{
  "pickup_location": "Primary",
  "pickup_date": "2026-04-22",
  "pickup_time": "14:00:00",
  "expected_package_count": 1
}
```

Implementation: `services/marketplace-api/internal/shipping/delhivery.go`
— `(*DelhiveryCarrier).SchedulePickup` (satisfies the
`shipping.PickupScheduler` interface defined in `carrier.go`).

The trailing slash on `/fm/request/new/` is load-bearing — dropping it
404s on production.

## Responses observed during the live probe

All probes ran against the production host with a real test token on a
low-balance account; redacted body shown verbatim.

### Wallet below threshold (the test account state)

```
HTTP 400
{"prepaid":"Client wallet balance is 298.48 which is less than 500.0"}
```

The wallet check runs BEFORE the pickup-specific validation, so we
could not fully probe the happy path on this account. Top up the wallet
on one.delhivery.com → Billing before re-running integration tests.

### Invalid / rotated token

```
HTTP 401
{"detail":"Invalid token"}
```

Same shape as `/api/cmu/create.json` — our existing token-rotation
runbook applies.

### Success (from Delhivery API docs and the SDK ecosystem)

```
HTTP 200
{
  "pr_id": 987654,
  "pickup_id": 987654,
  "incoming_center_name": "Pune_H",
  "success": "Request received"
}
```

Delhivery aliases `pr_id`, `pickup_id`, and `pickup_request_id` in
different responses — the parser in `parseDelhiveryPickupID` accepts
whichever is non-zero, in that order.

### Duplicate (same warehouse + date already booked)

```
HTTP 400
{"pr_exists":"pickup already scheduled for this client warehouse on this date"}
```

Translated by the carrier code to the sentinel error
`ErrPickupAlreadyScheduled` and a `Pickup{ProviderPickupID:
"already-scheduled"}`. Callers treat this as success — the pickup
exists, regardless of whether it was this POST that created it.

## Auto-schedule behaviour

On `POST /admin/stores/:storeId/orders/:id/shipments` (the "Create
shipping label" path), the handler:

1. Creates the waybill via `CreateShipment` as before.
2. Persists the shipment row with the waybill.
3. If `carrier_configs.auto_schedule_pickup = TRUE` AND the carrier
   implements `shipping.PickupScheduler`, calls `SchedulePickup` with:
   - `WarehouseName = carrier_config.warehouse_name`
   - `Date         = nextBusinessDay(time.Now().UTC())`
   - `TimeStart    = carrier_config.default_pickup_slot_start` (or
     `14:00:00` if blank)
   - `ExpectedPackageCount = 1`
4. On success (or the duplicate sentinel), writes
   `shipments.pickup_request_id` + `shipments.pickup_scheduled_for` and
   appends an `EventKindPickupScheduled` timeline event.
5. On any other error, logs WARN and keeps going — the merchant can
   retry via the manual endpoint.

`nextBusinessDay` rolls Sundays forward to Monday; Indian public
holidays are not handled here (the list is merchant-specific and
Delhivery's UI handles the rollover on their side).

## How to disable auto-schedule

Settings → Shipping → (carrier card) → **Auto-schedule Delhivery pickup
when a label is created** — unchecking this stops the pickup call from
firing on Create. The merchant must then click **Reschedule pickup**
on each order manually, or go to one.delhivery.com.

Existing carrier-config rows default to `auto_schedule_pickup = TRUE`
via the SQL default in
`tesserix-k8s/charts/apps/db-schema-bootstrap/schemas/marketplace/marketplace/shipping_pickup.sql`
so the feature opts in automatically for everyone already live.

## Manual reschedule

```
POST /api/v1/admin/stores/:storeId/orders/:id/shipments/:shipmentId/pickup/schedule
{
  "date": "2026-04-22",     // optional — defaults to next business day
  "slot_start": "14:00:00"  // optional — defaults to config
}
```

Used by the "Reschedule pickup" button in the admin order panel. Both
body fields are optional — an empty POST is the common case when the
auto-schedule succeeded with defaults and the merchant just wants to
retry after a wallet top-up.

Returns the updated shipment DTO, including the new
`pickup_request_id` + `pickup_scheduled_for`.

## Known Delhivery quirks

1. **Minimum lead time.** Submitting a `pickup_date` in the past or
   "today after cutoff" is accepted by the API but silently drops in
   some regions. We default to the next business day.
2. **Sundays.** The endpoint accepts Sunday dates but dispatches no
   agent — `nextBusinessDay` skips Sundays explicitly.
3. **Duplicate (warehouse, date) tuples.** See above — 400 with an
   "already" phrase in the body. Treat as success.
4. **Trailing slash.** `POST /fm/request/new` (without the slash)
   returns 404. The slash matters.
5. **Wallet balance.** The prepaid balance must exceed Delhivery's
   minimum (`500.0` at the time of writing) before any pickup is
   accepted. Keep this out of the auto-schedule error-classifier: it's
   an operator-resolvable state, not a code bug.
