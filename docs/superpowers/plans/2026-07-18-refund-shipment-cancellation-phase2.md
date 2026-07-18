# Refund / Cancel → Shipment Lifecycle — Phase 2 Implementation Plan (in-transit RTO)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** When an order is fully refunded or cancelled and its shipment is already in transit, auto-trigger a return-to-origin (RTO) at the carrier; fall back to a recorded manual notice when the carrier or shipment state can't support it.

**Architecture:** The decision layer (Phase 1) already maps `in_transit`/`out_for_delivery` → `ActionTriggerRTO`. Phase 2 makes the executor act on it: a new optional carrier capability `ReturnToOriginer`, which Delhivery implements by delegating to the same `/api/p/edit cancellation:true` call (see Open-question resolution). The executor type-asserts the capability; carriers without it (or a carrier rejection) record an `unsupported`/`failed` outcome — the manual notice. No migration, no wiring changes — the refund/cancel/manual hooks already call `CancelForOrder`, which now handles in-transit shipments too.

**Tech Stack:** Go 1.26.5, httptest. Service: `services/marketplace-api`.

## Open-question resolution (spec Q2 — verified against docs 2026-07-18)

- Delhivery has **no distinct RTO endpoint**. The Cancel Order API (`POST /api/p/edit`, `{"cancellation":"true"}`) is eligible in states **"Manifested In Transit Pending Open Scheduled"**. Cancelling an in-transit **prepaid/COD** shipment moves it to **"Returned"** (RTO to the client warehouse); a not-yet-picked-up pickup package moves to "Cancelled".
  - Source: Delhivery Cancel Order API doc + "RTO … occur[s] when the client cancels the order while it was in the 'In Transit' stage."
- The **NDR API** (`/api/p/update`, actions `RE-ATTEMPT`/`DEFER_DLV`/`EDIT_DETAILS`) is for **post-failed-delivery** handling, **not** proactive RTO — out of scope.
- ⇒ Delhivery `ReturnToOrigin(waybill)` = the exact same request as `CancelShipment(waybill)` (live-verified in Phase 1). The only observable difference is the resulting Delhivery status, which Delhivery derives from the shipment's state. `out_for_delivery` is **not** in the eligible list, so a proactive RTO there will return `<status>Failure</status>` and be recorded as `failed` (the manual notice) — acceptable.

## Global Constraints

- Best-effort: an RTO failure must never affect the refund/cancel (unchanged from Phase 1 — the executor swallows + records).
- Merchant-facing reason via the carrier's clean `<error>` extraction (unchanged — `ReturnToOrigin` delegates to the hardened `CancelShipment`).
- Optional capability interface (type-assert), matching the codebase's `WarehouseSyncer`/`PickupScheduler`/`LabelFetcher` pattern — do NOT add a method to the base `Carrier` interface (would force every carrier to implement it).
- Single-line conventional commits, no signatures, no PRs.

## File Structure

- `internal/shipping/carrier.go` — MODIFY: add optional `ReturnToOriginer` interface.
- `internal/shipping/delhivery.go` — MODIFY: add `ReturnToOrigin` (delegates to `CancelShipment`).
- `internal/shipping/delhivery_test.go` — MODIFY: assert `ReturnToOrigin` hits `/api/p/edit cancellation:true` and honours the failure body.
- `internal/shipmentcancel/executor.go` — MODIFY: wire `ActionTriggerRTO` → `execReturnToOrigin`; extract a shared `resolveCarrier` helper.
- `internal/shipmentcancel/executor_test.go` — MODIFY: add RTO-capable-carrier success test; the existing in-transit-unsupported test now documents the no-capability path.
- `internal/handlers/admin/shipment_cancel_integration_test.go` — MODIFY: add an in-transit → RTO integration test; extend `recordingCarrier` with `ReturnToOrigin`.

---

### Task 1: Delhivery `ReturnToOrigin` capability

**Files:**
- Modify: `internal/shipping/carrier.go`, `internal/shipping/delhivery.go`
- Test: `internal/shipping/delhivery_test.go`

**Interfaces:**
- Produces: `type ReturnToOriginer interface { ReturnToOrigin(ctx context.Context, trackingNumber string) error }` in package `shipping`.
- Produces: `func (c *DelhiveryCarrier) ReturnToOrigin(ctx context.Context, shipmentID string) error` — same request as `CancelShipment`.

- [ ] **Step 1: Write the failing test**

Add to `internal/shipping/delhivery_test.go`:

```go
func TestDelhivery_ReturnToOrigin_UsesCancelEndpoint(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<root><status>Success</status><waybill>WBN9</waybill></root>`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	if err := c.ReturnToOrigin(context.Background(), "WBN9"); err != nil {
		t.Fatalf("ReturnToOrigin returned %v", err)
	}
	if gotPath != "/api/p/edit" {
		t.Errorf("path = %q, want /api/p/edit", gotPath)
	}
	if !strings.Contains(gotBody, `"cancellation"`) || !strings.Contains(gotBody, "WBN9") {
		t.Errorf("body = %q, want cancellation + waybill", gotBody)
	}
}

func TestDelhivery_ReturnToOrigin_SurfacesFailureBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<root><error>Not cancellable in current state</error><status>Failure</status></root>`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	err := c.ReturnToOrigin(context.Background(), "WBN9")
	if err == nil || !strings.Contains(err.Error(), "Not cancellable") {
		t.Fatalf("ReturnToOrigin err = %v, want the <error> text", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/shipping/ -run TestDelhivery_ReturnToOrigin -v`
Expected: FAIL — `ReturnToOrigin` undefined.

- [ ] **Step 3: Add the interface + method**

In `internal/shipping/carrier.go`, after the `PickupScheduler` interface block, add:

```go
// ReturnToOriginer is implemented by carriers that can proactively return an
// already-picked-up / in-transit shipment to its origin (RTO). Delhivery is
// the first implementor: it has no distinct RTO endpoint — cancelling an
// in-transit prepaid/COD shipment via /api/p/edit moves it to "Returned"
// (RTO to the client warehouse). Carriers that don't implement this record an
// "unsupported" outcome so the merchant arranges the return manually.
type ReturnToOriginer interface {
	ReturnToOrigin(ctx context.Context, trackingNumber string) error
}
```

In `internal/shipping/delhivery.go`, immediately after `CancelShipment`, add:

```go
// ReturnToOrigin triggers an RTO for an in-transit shipment. Delhivery exposes
// no separate RTO endpoint: cancelling an in-transit prepaid/COD shipment via
// /api/p/edit (cancellation:true) moves it to "Returned" (RTO to the client
// warehouse). The request/response is therefore identical to CancelShipment —
// we delegate rather than duplicate. Delhivery rejects states outside
// "Manifested/In Transit/Pending/Open/Scheduled" (e.g. out-for-delivery) with
// <status>Failure</status>, which surfaces as a clean error the caller records
// as a manual-notice fallback.
func (c *DelhiveryCarrier) ReturnToOrigin(ctx context.Context, shipmentID string) error {
	return c.CancelShipment(ctx, shipmentID)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd services/marketplace-api && go test ./internal/shipping/ -run TestDelhivery_ReturnToOrigin -v && go test ./internal/shipping/`
Expected: PASS (new + full package).

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/shipping/carrier.go services/marketplace-api/internal/shipping/delhivery.go services/marketplace-api/internal/shipping/delhivery_test.go
git commit -m "feat(marketplace-api): add delhivery return-to-origin capability for in-transit shipments"
```

---

### Task 2: Executor wires `ActionTriggerRTO`

**Files:**
- Modify: `internal/shipmentcancel/executor.go`
- Test: `internal/shipmentcancel/executor_test.go`

**Interfaces:**
- Consumes: `shipping.ReturnToOriginer` (Task 1).
- Behaviour: `ActionTriggerRTO` → resolve carrier → if it implements `ReturnToOriginer`, call `ReturnToOrigin` and record `rto`/`succeeded`|`failed`; else record `rto`/`unsupported` (manual notice). `ActionReversePickup` still records `unsupported` (Phase 3).

- [ ] **Step 1: Write the failing test**

Add to `internal/shipmentcancel/executor_test.go`:

```go
// fakeRTOCarrier implements shipping.Carrier AND shipping.ReturnToOriginer.
type fakeRTOCarrier struct {
	fakeCarrier
	rtoCalls int
	rtoErr   error
}

func (f *fakeRTOCarrier) ReturnToOrigin(_ context.Context, _ string) error {
	f.rtoCalls++
	return f.rtoErr
}

func TestExecutor_InTransit_RTO_Success(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "in_transit"}},
	}}
	car := &fakeRTOCarrier{}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "succeeded" || out[0].Action != ActionTriggerRTO {
		t.Fatalf("outcomes = %+v, want rto/succeeded", out)
	}
	if car.rtoCalls != 1 {
		t.Errorf("ReturnToOrigin calls = %d, want 1", car.rtoCalls)
	}
	if car.cancelCalls != 0 {
		t.Errorf("CancelShipment called %d times, want 0 (RTO path)", car.cancelCalls)
	}
	if store.sets[0].Action != string(ActionTriggerRTO) || store.sets[0].Status != "succeeded" {
		t.Errorf("recorded = %+v, want rto/succeeded", store.sets)
	}
}

func TestExecutor_InTransit_RTO_CarrierFailure(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "out_for_delivery"}},
	}}
	car := &fakeRTOCarrier{rtoErr: errors.New("delhivery: cancel shipment: Not cancellable in current state")}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "failed" || out[0].Action != ActionTriggerRTO {
		t.Fatalf("outcomes = %+v, want rto/failed", out)
	}
	if store.sets[0].Reason != "Not cancellable in current state" {
		t.Errorf("reason = %q, want cleaned carrier reason", store.sets[0].Reason)
	}
}
```

The existing `TestExecutor_InTransit_Unsupported` (fakeCarrier, no `ReturnToOrigin`) now documents the no-capability path — leave it unchanged.

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/shipmentcancel/ -run 'TestExecutor_InTransit_RTO' -v`
Expected: FAIL — `TestExecutor_InTransit_RTO_Success` gets `unsupported`, not `succeeded` (RTO not wired yet).

- [ ] **Step 3: Wire the executor**

In `internal/shipmentcancel/executor.go`, replace the `ActionTriggerRTO, ActionReversePickup` combined case with split handling, and refactor carrier resolution into a shared helper.

Replace the switch arm:

```go
	case ActionTriggerRTO, ActionReversePickup:
		// Phase 2/3 handle these; until then, record so the admin sees it and
		// arranges the return manually with the carrier.
		reason := "This shipment has left for delivery — arrange the return manually with the carrier."
		e.record(ctx, sh.ID, action, statusUnsupported, reason)
		return Outcome{ShipmentID: sh.ID, Action: action, Status: statusUnsupported, Reason: reason}
```

with:

```go
	case ActionTriggerRTO:
		return e.execReturnToOrigin(ctx, sh)

	case ActionReversePickup:
		// Phase 3 handles delivered shipments; until then, record so the admin
		// sees it and arranges the return manually with the carrier.
		reason := "This shipment was delivered — arrange the return manually with the carrier."
		e.record(ctx, sh.ID, ActionReversePickup, statusUnsupported, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusUnsupported, Reason: reason}
```

Replace `execCancelForward` with a version that uses a shared resolver, and add `execReturnToOrigin`:

```go
// resolveCarrier validates the tracking number and builds the carrier for a
// shipment. A non-empty failureReason means the caller should record a
// `failed` outcome with it and stop.
func (e *Executor) resolveCarrier(ctx context.Context, sh *shipping.ShipmentRecord) (carrier shipping.Carrier, failureReason string) {
	if strings.TrimSpace(sh.TrackingNumber) == "" {
		return nil, "No tracking number on the shipment — nothing to send to the carrier."
	}
	c, err := e.resolve(ctx, sh.StoreID, sh.Carrier)
	if err != nil {
		e.warn("shipmentcancel: resolve carrier failed", "shipment_id", sh.ID.String(), "carrier", sh.Carrier, "err", err)
		return nil, "Could not reach the carrier — retry from the shipment."
	}
	return c, ""
}

func (e *Executor) execCancelForward(ctx context.Context, sh *shipping.ShipmentRecord) Outcome {
	carrier, failReason := e.resolveCarrier(ctx, sh)
	if failReason != "" {
		e.record(ctx, sh.ID, ActionCancelForward, statusFailed, failReason)
		return Outcome{ShipmentID: sh.ID, Action: ActionCancelForward, Status: statusFailed, Reason: failReason}
	}
	if err := carrier.CancelShipment(ctx, sh.TrackingNumber); err != nil {
		reason := cleanReason(err)
		e.warn("shipmentcancel: carrier cancel failed", "shipment_id", sh.ID.String(), "carrier", sh.Carrier, "err", err)
		e.record(ctx, sh.ID, ActionCancelForward, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionCancelForward, Status: statusFailed, Reason: reason}
	}
	e.record(ctx, sh.ID, ActionCancelForward, statusSucceeded, "")
	return Outcome{ShipmentID: sh.ID, Action: ActionCancelForward, Status: statusSucceeded}
}

// execReturnToOrigin returns an in-transit shipment to origin. Carriers that
// don't implement ReturnToOriginer record an `unsupported` outcome (the manual
// notice); a carrier rejection (e.g. a state that can't RTO) records `failed`
// with the carrier's clean reason — also a manual notice.
func (e *Executor) execReturnToOrigin(ctx context.Context, sh *shipping.ShipmentRecord) Outcome {
	carrier, failReason := e.resolveCarrier(ctx, sh)
	if failReason != "" {
		e.record(ctx, sh.ID, ActionTriggerRTO, statusFailed, failReason)
		return Outcome{ShipmentID: sh.ID, Action: ActionTriggerRTO, Status: statusFailed, Reason: failReason}
	}
	rtoer, ok := carrier.(shipping.ReturnToOriginer)
	if !ok {
		reason := "This carrier can't return an in-transit shipment automatically — arrange the return manually with the carrier."
		e.record(ctx, sh.ID, ActionTriggerRTO, statusUnsupported, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionTriggerRTO, Status: statusUnsupported, Reason: reason}
	}
	if err := rtoer.ReturnToOrigin(ctx, sh.TrackingNumber); err != nil {
		reason := cleanReason(err)
		e.warn("shipmentcancel: carrier RTO failed", "shipment_id", sh.ID.String(), "carrier", sh.Carrier, "err", err)
		e.record(ctx, sh.ID, ActionTriggerRTO, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionTriggerRTO, Status: statusFailed, Reason: reason}
	}
	e.record(ctx, sh.ID, ActionTriggerRTO, statusSucceeded, "")
	return Outcome{ShipmentID: sh.ID, Action: ActionTriggerRTO, Status: statusSucceeded}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd services/marketplace-api && go test ./internal/shipmentcancel/ -v`
Expected: PASS (new RTO tests + existing tests, including `TestExecutor_InTransit_Unsupported` unchanged).

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/shipmentcancel/executor.go services/marketplace-api/internal/shipmentcancel/executor_test.go
git commit -m "feat(marketplace-api): auto-trigger RTO for in-transit shipments on refund/cancel"
```

---

### Task 3: Integration coverage + full verify

**Files:**
- Modify: `internal/handlers/admin/shipment_cancel_integration_test.go`

- [ ] **Step 1: Extend `recordingCarrier` with RTO + add the in-transit test**

Add a `ReturnToOrigin` method (with an `rtoCalls` counter) to `recordingCarrier` so it satisfies `shipping.ReturnToOriginer`, plus a `rtoErr` field. Then add:

```go
func TestFullRefund_InTransit_TriggersRTO(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	orderID := createAndConfirmOrder(t, env, base, headers, "100.00")
	tUUID, sUUID, oUUID := uuid.MustParse(tenantID), uuid.MustParse(storeID), uuid.MustParse(orderID)
	seedCapturedPaymentTxn(t, env.db, tUUID, sUUID, oUUID, "100.00")
	seedActiveGatewayConfig(t, env.db, tUUID, sUUID)
	// Seed an IN-TRANSIT shipment (status='in_transit').
	seedShipmentWithStatus(t, env.db, tUUID, sUUID, oUUID, "WBN-INT-RTO", "in_transit")

	car := &recordingCarrier{}
	coord := coordinatorWithCanceller(env.db, car)

	if _, err := coord.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: oUUID, Amount: nil, Reason: "test", Actor: "test", ScopeID: "req-rto",
	}); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	if car.rtoCalls != 1 || car.cancelCalls != 0 {
		t.Fatalf("rtoCalls=%d cancelCalls=%d, want 1/0", car.rtoCalls, car.cancelCalls)
	}
	var row struct{ CancelAction, CancelStatus string }
	if err := env.db.Table("shipments").
		Select("cancel_action", "cancel_status").
		Where("order_id = ?", oUUID).Scan(&row).Error; err != nil {
		t.Fatalf("reload shipment: %v", err)
	}
	if row.CancelAction != "rto" || row.CancelStatus != "succeeded" {
		t.Fatalf("cancel state = %s/%s, want rto/succeeded", row.CancelAction, row.CancelStatus)
	}
}
```

Generalise the seed helper: rename `seedPendingShipment` to `seedShipmentWithStatus(t, db, tenantID, storeID, orderID, waybill, status)` (parameterise the `'pending'` literal), and update the Phase 1 callers to pass `"pending"`. Add the `ReturnToOrigin` method to `recordingCarrier`:

```go
func (r *recordingCarrier) ReturnToOrigin(_ context.Context, waybill string) error {
	r.rtoCalls++
	r.lastWaybill = waybill
	return r.rtoErr
}
```
(add `rtoCalls int` and `rtoErr error` fields to the struct).

- [ ] **Step 2: Compile-check the integration build**

Run: `cd services/marketplace-api && go vet -tags integration ./internal/handlers/admin/`
Expected: compiles clean.

- [ ] **Step 3: Full unit suite + vet**

Run:
```bash
cd services/marketplace-api && go build ./... && go test ./internal/shipmentcancel/... ./internal/shipping/... ./internal/handlers/admin/... . && go vet ./internal/shipmentcancel/... ./internal/shipping/...
```
Expected: PASS, clean.

- [ ] **Step 4: Run integration tests against ephemeral Postgres**

Spin an ephemeral Postgres (pgcrypto + uuid-ossp extensions), migrate, then:
Run: `TEST_DATABASE_URL=… go test -tags integration ./internal/handlers/admin/ -run 'FullRefund|PartialRefund' -v`
Expected: PASS (Phase 1 + the new RTO test).

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/shipment_cancel_integration_test.go
git commit -m "test(marketplace-api): integration coverage for in-transit RTO on full refund"
```

---

## Self-Review (against the spec, Phase 2 row)

- **State→action:** `in_transit`/`out_for_delivery` → `ActionTriggerRTO` → carrier RTO (Task 2). ✅
- **Auto-trigger on in-transit:** the refund/cancel/manual hooks already call `CancelForOrder`; the executor now dispatches RTO. No new wiring. ✅
- **Manual-notice fallback when unsupported:** carrier lacks capability → `unsupported`; carrier rejects state (e.g. out-for-delivery) → `failed` with clean reason. Both surface for manual handling. ✅
- **Carrier-agnostic, Delhivery first:** optional `ReturnToOriginer`; other carriers no-op with `unsupported`. ✅
- **Best-effort:** unchanged — executor swallows + records, refund unaffected. ✅
- **Open question Q2:** resolved — same Cancel Order API call; no NDR/separate endpoint. Documented on the method. ✅

## Not live-verified (flag)

The in-transit → "Returned" (RTO) transition is **doc-verified**, not verified against a real in-transit Delhivery shipment (none exists to test non-destructively). The *call itself* is byte-identical to the live-verified Phase 1 cancel, so the risk is limited to Delhivery's state-machine response, which the executor records rather than acts on blindly. Confirm on the next real in-transit refund.
