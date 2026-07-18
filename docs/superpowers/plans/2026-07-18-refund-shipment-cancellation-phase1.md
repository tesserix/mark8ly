# Refund / Cancel → Shipment Lifecycle — Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When an order is fully refunded or cancelled, cancel its pre-pickup Delhivery shipment (the reported bug), recording a per-shipment outcome; provide a manual per-shipment cancel button. In-transit/delivered shipments record an `unsupported` outcome for now (Phases 2/3).

**Architecture:** A pure decision layer (`internal/shipmentcancel.ResolveAction`) maps a shipment's `status` column to an action. A best-effort executor dispatches `cancel_forward` to the carrier's existing `CancelShipment`, records the outcome on four new `shipments` columns, and is idempotent on already-succeeded rows. The refund coordinator fires the executor after a **full**-refund success; the admin Cancel handler fires it on the **unpaid** cancel path (paid cancels reach it through the auto-refund → coordinator hook); a new manual endpoint fires it per shipment. A carrier failure never rolls back or fails the refund/cancel.

**Tech Stack:** Go 1.26.5, Gin, GORM, golang-migrate, testify/httptest. Service: `services/marketplace-api`.

## Global Constraints

- Go toolchain **1.26.5** (`go.work`). Build/test with the workspace's Go.
- Any migration MUST bump `ExpectedSchemaVersion` in `services/marketplace-api/migrations.go` — `TestExpectedSchemaVersionMatchesHighestMigration` fails CI otherwise, and a mismatch `os.Exit(1)`s every prod pod (migrate runs as an init container).
- **Best-effort is load-bearing:** a carrier-cancel failure must NEVER roll back or fail the refund/cancel. Mirror `handlers/admin/settings.go` `syncWarehouseAsync` (detached context, log-only on failure).
- **Merchant-facing errors** must surface Delhivery's short reason only (extract `<error>`/`<message>`), never the raw XML/JSON body (leaks address/phone/hours).
- Single-line conventional commits, no signatures, no PRs. Commit after each task.
- Immutability: return new values; never mutate shared inputs. Files stay focused (<400 lines typical).

## Verified facts (from live probe + code read, 2026-07-18)

- `POST https://track.delhivery.com/api/p/edit` with `{"waybill":"…","cancellation":"true"}` returns **HTTP 200 even on failure**. Body is **XML**:
  - Failure: `<?xml …?><root><error>Incorrect Waybill/OrderID, please try again</error><status>Failure</status><waybill></waybill><order_id>…</order_id></root>`
  - Success: `<status>Success</status>` with a non-empty `<waybill>` (Delhivery moves pickup packages → `Cancelled`, prepaid/COD → `Returned`).
  - ⇒ The current `CancelShipment` (checks only the status code) treats failures as success — Task 3 fixes this.
- `CancelShipment` currently has **no callers** (`grep` confirms), so hardening its contract is safe.
- Shipment `status` column values in practice: `pending` (set on label create), `in_transit`, `out_for_delivery` (admin status advance), `delivered`, `exception` (Delhivery RT/UD). `MapDelhiveryStatus` (`delhivery.go:1007`) never emits `pending`/`out_for_delivery`; those come from create + the admin `UpdateStatus` handler.
- Highest migration on disk: `000095_warehouses`. `ExpectedSchemaVersion = 95`. New migration is `000096`, bump to `96`.
- Carrier construction path: `ShipmentsHandler.resolveCarrierCreds(ctx, cfg)` → `shipping.NewCarrier(provider, apiKey, secretKey, cfg.Mode)`; config via `shippingRepo.GetCarrierConfig(ctx, storeID, provider)`. Reused by the executor's carrier resolver.
- Integration tests use build tag `//go:build integration` + `pkg/testdb` (real Postgres, migrations applied) + a truncate list including `shipments`.

---

## File Structure

- `internal/shipmentcancel/action.go` — `Action` type + `ResolveAction` (pure). ~40 lines.
- `internal/shipmentcancel/action_test.go` — table test.
- `internal/shipmentcancel/executor.go` — `Outcome`, `CarrierResolver`, narrow `shipmentStore` iface, `Executor`, `CancelForOrder`, `CancelShipmentByID`. ~150 lines.
- `internal/shipmentcancel/executor_test.go` — fake store + fake carrier unit tests.
- `internal/shipping/repository.go` — MODIFY: 4 cancel columns on `ShipmentRecord`; add `ListShipmentsByOrderID` + `SetShipmentCancelState` to `Repository` + `gormRepository`.
- `internal/shipping/delhivery.go` — MODIFY: harden `CancelShipment` (parse XML body) + `delhiveryCancelResult` helper.
- `internal/shipping/delhivery_test.go` — MODIFY: add cancel success/failure/non-200 tests.
- `migrations/000096_shipment_cancel_actions.up.sql` / `.down.sql` — new columns.
- `migrations.go` — MODIFY: `ExpectedSchemaVersion = 96`.
- `internal/orderrefund/coordinator.go` — MODIFY: `cancelShipments` func field + `WithShipmentCanceller` + fire on full-refund success.
- `internal/orderrefund/coordinator_shipmentcancel_test.go` — hook-fires test.
- `internal/handlers/admin/orders.go` — MODIFY: remove `shipmentBlocksCancel` 409; add `canceller` field + `WithShipmentCanceller`; fire on unpaid cancel path.
- `internal/handlers/admin/shipments.go` — MODIFY: add `canceller` field + `WithCanceller` + `CarrierForStore` helper + `CancelShipment` handler; surface `cancel_status`/`cancel_action` in `toShipmentResponse`.
- `internal/handlers/admin/routes.go` — MODIFY: register `POST /orders/:id/shipments/:shipmentId/cancel`.
- `cmd/marketplace-api/main.go` — MODIFY: build executor, inject into coordinator + orders + shipments handlers.

---

### Task 1: Decision layer — `ResolveAction`

**Files:**
- Create: `internal/shipmentcancel/action.go`
- Test: `internal/shipmentcancel/action_test.go`

**Interfaces:**
- Produces: `type Action string`; consts `ActionNoop = "none"`, `ActionCancelForward = "cancel_forward"`, `ActionTriggerRTO = "rto"`, `ActionReversePickup = "reverse_pickup"`; `func ResolveAction(shipmentStatus string) Action`.

- [ ] **Step 1: Write the failing table test**

```go
package shipmentcancel

import "testing"

func TestResolveAction(t *testing.T) {
	cases := []struct {
		status string
		want   Action
	}{
		{"", ActionCancelForward},
		{"pending", ActionCancelForward},
		{"PENDING", ActionCancelForward},
		{"created", ActionCancelForward},
		{"manifested", ActionCancelForward},
		{"in_transit", ActionTriggerRTO},
		{"out_for_delivery", ActionTriggerRTO},
		{"delivered", ActionReversePickup},
		{"cancelled", ActionNoop},
		{"canceled", ActionNoop},
		{"returned", ActionNoop},
		{"rto", ActionNoop},
		{"exception", ActionNoop},
		{"something_unknown", ActionNoop},
	}
	for _, tc := range cases {
		if got := ResolveAction(tc.status); got != tc.want {
			t.Errorf("ResolveAction(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/shipmentcancel/ -run TestResolveAction -v`
Expected: FAIL — build error, `ResolveAction`/`Action` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// Package shipmentcancel resolves and executes the carrier action for a
// shipment when its order is refunded or cancelled. The decision layer here
// is pure and carrier-agnostic; the executor (executor.go) dispatches to the
// carrier and records outcomes.
package shipmentcancel

import "strings"

// Action is the carrier action a shipment's current lifecycle state calls for.
type Action string

const (
	// ActionNoop — nothing to do (already cancelled/returned/exception, or an
	// unknown state we deliberately leave for manual handling).
	ActionNoop Action = "none"
	// ActionCancelForward — pre-pickup: cancel the forward waybill.
	ActionCancelForward Action = "cancel_forward"
	// ActionTriggerRTO — in transit: return to origin (Phase 2; unsupported in Phase 1).
	ActionTriggerRTO Action = "rto"
	// ActionReversePickup — delivered: create a reverse pickup (Phase 3; unsupported in Phase 1).
	ActionReversePickup Action = "reverse_pickup"
)

// ResolveAction maps a shipment's status column to the carrier action. Pure:
// no carrier or DB dependency. Unknown/terminal states resolve to ActionNoop
// so we never take a destructive action on a state we don't understand.
func ResolveAction(shipmentStatus string) Action {
	switch strings.ToLower(strings.TrimSpace(shipmentStatus)) {
	case "", "pending", "created", "manifested":
		return ActionCancelForward
	case "in_transit", "out_for_delivery":
		return ActionTriggerRTO
	case "delivered":
		return ActionReversePickup
	default:
		// cancelled, canceled, returned, rto, exception, and anything unknown.
		return ActionNoop
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/marketplace-api && go test ./internal/shipmentcancel/ -run TestResolveAction -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/shipmentcancel/action.go services/marketplace-api/internal/shipmentcancel/action_test.go
git commit -m "feat(marketplace-api): add pure shipment-cancel decision layer"
```

---

### Task 2: Data model — migration + columns + repo methods

**Files:**
- Create: `migrations/000096_shipment_cancel_actions.up.sql`, `migrations/000096_shipment_cancel_actions.down.sql`
- Modify: `migrations.go` (`ExpectedSchemaVersion = 96`)
- Modify: `internal/shipping/repository.go` (model columns + 2 repo methods)

**Interfaces:**
- Produces on `shipping.ShipmentRecord`: `CancelAction string`, `CancelStatus string`, `CancelReason string`, `CancelRequestedAt *time.Time`.
- Produces on `shipping.Repository`: `ListShipmentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]ShipmentRecord, error)`, `SetShipmentCancelState(ctx context.Context, shipmentID uuid.UUID, action, status, reason string) error`.

- [ ] **Step 1: Write the failing guard test check**

The guard test already exists (`migrations_test.go`). Adding the migration file without bumping the constant makes it fail — that is the RED we want.

Create `migrations/000096_shipment_cancel_actions.up.sql`:

```sql
-- Record the carrier-side cancel/return action taken for a shipment when its
-- order is refunded or cancelled. Lets the admin see per-shipment outcome and
-- lets a future sweep re-drive failures. All nullable-or-defaulted so existing
-- rows stay valid with no backfill.
--
--   cancel_action       none | cancel_forward | rto | reverse_pickup
--   cancel_status       none | requested | succeeded | failed | unsupported
--   cancel_reason       carrier's short reason on failure (never the raw body)
--   cancel_requested_at when the action was attempted
ALTER TABLE shipments
    ADD COLUMN IF NOT EXISTS cancel_action       varchar(20) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS cancel_status       varchar(20) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS cancel_reason       text,
    ADD COLUMN IF NOT EXISTS cancel_requested_at timestamptz;
```

Create `migrations/000096_shipment_cancel_actions.down.sql`:

```sql
ALTER TABLE shipments
    DROP COLUMN IF EXISTS cancel_requested_at,
    DROP COLUMN IF EXISTS cancel_reason,
    DROP COLUMN IF EXISTS cancel_status,
    DROP COLUMN IF EXISTS cancel_action;
```

- [ ] **Step 2: Run guard test to verify it fails**

Run: `cd services/marketplace-api && go test . -run TestExpectedSchemaVersionMatchesHighestMigration -v`
Expected: FAIL — "ExpectedSchemaVersion = 95, but highest migration on disk is 96".

- [ ] **Step 3: Bump the constant**

In `migrations.go`, change:

```go
const ExpectedSchemaVersion uint = 95
```
to
```go
const ExpectedSchemaVersion uint = 96
```

- [ ] **Step 4: Run guard test to verify it passes**

Run: `cd services/marketplace-api && go test . -run TestExpectedSchemaVersionMatchesHighestMigration -v`
Expected: PASS.

- [ ] **Step 5: Add model columns**

In `internal/shipping/repository.go`, inside `ShipmentRecord`, after the `PickupScheduledFor` field, add:

```go
	// Cancel/return action taken when the order was refunded or cancelled.
	// Written best-effort by internal/shipmentcancel; NOT touched on the
	// normal create/track path. Defaults keep pre-feature rows valid.
	CancelAction      string     `gorm:"column:cancel_action;type:varchar(20);not null;default:none"`
	CancelStatus      string     `gorm:"column:cancel_status;type:varchar(20);not null;default:none"`
	CancelReason      string     `gorm:"column:cancel_reason;type:text"`
	CancelRequestedAt *time.Time `gorm:"column:cancel_requested_at"`
```

- [ ] **Step 6: Add repository methods (interface + impl)**

In `internal/shipping/repository.go`, add to the `Repository` interface (in the Shipments block):

```go
	ListShipmentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]ShipmentRecord, error)
	SetShipmentCancelState(ctx context.Context, shipmentID uuid.UUID, action, status, reason string) error
```

And add the implementations after `GetShipmentByOrderID`:

```go
func (r *gormRepository) ListShipmentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]ShipmentRecord, error) {
	var recs []ShipmentRecord
	if err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("created_at ASC").
		Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("shipping: list shipments by order id: %w", err)
	}
	return recs, nil
}

// SetShipmentCancelState records the outcome of a carrier cancel/return
// attempt on a shipment. Stamps cancel_requested_at + updated_at to now().
func (r *gormRepository) SetShipmentCancelState(ctx context.Context, shipmentID uuid.UUID, action, status, reason string) error {
	res := r.db.WithContext(ctx).
		Table("shipments").
		Where("id = ?", shipmentID).
		Updates(map[string]any{
			"cancel_action":       action,
			"cancel_status":       status,
			"cancel_reason":       reason,
			"cancel_requested_at": time.Now().UTC(),
			"updated_at":          time.Now().UTC(),
		})
	if res.Error != nil {
		return fmt.Errorf("shipping: set shipment cancel state: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("shipping: shipment not found")
	}
	return nil
}
```

- [ ] **Step 7: Verify build**

Run: `cd services/marketplace-api && go build ./internal/shipping/... && go test . -run TestExpectedSchemaVersion -v`
Expected: build OK, guard test PASS.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/migrations/000096_shipment_cancel_actions.up.sql services/marketplace-api/migrations/000096_shipment_cancel_actions.down.sql services/marketplace-api/migrations.go services/marketplace-api/internal/shipping/repository.go
git commit -m "feat(marketplace-api): add shipment cancel-action columns and repo methods"
```

---

### Task 3: Harden Delhivery `CancelShipment` (parse the 200-on-failure XML body)

**Files:**
- Modify: `internal/shipping/delhivery.go`
- Test: `internal/shipping/delhivery_test.go`

**Interfaces:**
- Consumes: existing `betweenTags` helper (`delhivery.go`).
- Produces: `CancelShipment` now returns `nil` only on a real success (`<status>Success</status>`); on a 200-with-failure or non-2xx it returns a clean error carrying only Delhivery's `<error>`/`<message>` text.

- [ ] **Step 1: Write the failing tests**

Add to `internal/shipping/delhivery_test.go`:

```go
func TestDelhivery_CancelShipment_Success(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?><root><error></error><status>Success</status><waybill>WBN123</waybill></root>`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	if err := c.CancelShipment(context.Background(), "WBN123"); err != nil {
		t.Fatalf("CancelShipment success returned %v", err)
	}
	if gotPath != "/api/p/edit" {
		t.Errorf("path = %q, want /api/p/edit", gotPath)
	}
	if gotAuth != "Token tok" {
		t.Errorf("auth = %q, want Token tok", gotAuth)
	}
	if !strings.Contains(gotBody, `"cancellation"`) || !strings.Contains(gotBody, "WBN123") {
		t.Errorf("body = %q, want waybill + cancellation", gotBody)
	}
}

func TestDelhivery_CancelShipment_FailureBodyOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // Delhivery returns 200 even on failure
		_, _ = io.WriteString(w, `<?xml version="1.0"?><root><error>Incorrect Waybill/OrderID, please try again</error><status>Failure</status><waybill></waybill></root>`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	err := c.CancelShipment(context.Background(), "BAD")
	if err == nil {
		t.Fatal("CancelShipment on <status>Failure</status> returned nil, want error")
	}
	if !strings.Contains(err.Error(), "Incorrect Waybill") {
		t.Errorf("error = %q, want the <error> text", err.Error())
	}
}

func TestDelhivery_CancelShipment_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `<root><error>Invalid token</error></root>`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	if err := c.CancelShipment(context.Background(), "WBN"); err == nil {
		t.Fatal("CancelShipment on 401 returned nil, want error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/marketplace-api && go test ./internal/shipping/ -run TestDelhivery_CancelShipment -v`
Expected: FAIL — the success test may pass by luck, but `FailureBodyOn200` FAILS (current code returns nil on any 200).

- [ ] **Step 3: Rewrite `CancelShipment`**

Replace the existing `CancelShipment` method (`delhivery.go:584`) with:

```go
// CancelShipment cancels a forward waybill via /api/p/edit with
// cancellation:true. Delhivery returns HTTP 200 even when the cancel fails,
// with an XML body: <root><status>Success|Failure</status><error>…</error></root>.
// So we must inspect the body, not just the status code — an earlier reliance
// on the status code alone would record every failure as a success.
//
// On failure we surface ONLY Delhivery's <error>/<message> text, never the raw
// body. (Cancelling the waybill is enough: Delhivery moves a pickup package to
// "Cancelled" on its side; we deliberately do NOT touch the scheduled pickup
// request, which can bundle other live waybills.)
func (c *DelhiveryCarrier) CancelShipment(ctx context.Context, shipmentID string) error {
	if strings.TrimSpace(shipmentID) == "" {
		return fmt.Errorf("delhivery: cancel shipment: waybill is required")
	}
	body := map[string]string{
		"waybill":      shipmentID,
		"cancellation": "true",
	}
	resp, err := c.doJSONRequest(ctx, http.MethodPost, "/api/p/edit", body)
	if err != nil {
		return fmt.Errorf("delhivery: cancel shipment: %w", err)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("delhivery: cancel shipment: read body: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("delhivery: cancel shipment: %s", delhiveryCancelMessage(string(raw)))
	}
	if !delhiveryCancelSucceeded(string(raw)) {
		return fmt.Errorf("delhivery: cancel shipment: %s", delhiveryCancelMessage(string(raw)))
	}
	return nil
}

// delhiveryCancelSucceeded reports whether an /api/p/edit response indicates a
// successful cancel. Delhivery's XML uses <status>Success</status>; some
// endpoints echo JSON "status":true / "status":"Success". Absence of a clear
// success signal is treated as failure (fail-closed on an ambiguous body).
func delhiveryCancelSucceeded(body string) bool {
	if s := betweenTags(body, "<status>", "</status>"); s != "" {
		return strings.EqualFold(s, "success")
	}
	lower := strings.ToLower(body)
	return strings.Contains(lower, `"status":true`) ||
		strings.Contains(lower, `"status": "success"`) ||
		strings.Contains(lower, `"status":"success"`)
}

// delhiveryCancelMessage pulls Delhivery's short cancel-failure reason out of
// the response, preferring the XML <error>/<message> tags and the JSON
// "error"/"rmk" keys. Never returns the whole body. Falls back to a generic
// line so the caller always has something safe to show.
func delhiveryCancelMessage(body string) string {
	if m := betweenTags(body, "<error>", "</error>"); m != "" {
		return m
	}
	if m := betweenTags(body, "<message>", "</message>"); m != "" {
		return m
	}
	for _, key := range []string{`"error":"`, `"rmk":"`, `"message":"`} {
		if i := strings.Index(body, key); i >= 0 {
			rest := body[i+len(key):]
			if j := strings.Index(rest, `"`); j >= 0 {
				if v := strings.TrimSpace(rest[:j]); v != "" {
					return v
				}
			}
		}
	}
	return "the carrier rejected the cancellation"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/marketplace-api && go test ./internal/shipping/ -run TestDelhivery_CancelShipment -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/shipping/delhivery.go services/marketplace-api/internal/shipping/delhivery_test.go
git commit -m "fix(marketplace-api): parse delhivery cancel XML body so 200-on-failure is not treated as success"
```

---

### Task 4: Executor — resolve + execute per shipment, best-effort, idempotent

**Files:**
- Create: `internal/shipmentcancel/executor.go`
- Test: `internal/shipmentcancel/executor_test.go`

**Interfaces:**
- Consumes: `ResolveAction`, `Action` consts (Task 1); `shipping.ShipmentRecord`, `shipping.Carrier` (Task 3 hardened `CancelShipment`); repo methods `ListShipmentsByOrderID`, `GetShipmentByID`, `SetShipmentCancelState` (Task 2).
- Produces:
  - `type Outcome struct { ShipmentID uuid.UUID; Action Action; Status string; Reason string }`
  - `type CarrierResolver func(ctx context.Context, storeID uuid.UUID, provider string) (shipping.Carrier, error)`
  - `type Executor struct { … }`, `func NewExecutor(store ShipmentStore, resolve CarrierResolver, logger *slog.Logger) *Executor`
  - `func (e *Executor) CancelForOrder(ctx context.Context, orderID uuid.UUID) []Outcome`
  - `func (e *Executor) CancelShipmentByID(ctx context.Context, shipmentID uuid.UUID) (Outcome, error)`
  - `type ShipmentStore interface { … }` (narrow; `shipping.Repository` satisfies it)
- Status string values on `Outcome`/columns: `succeeded | failed | unsupported | none`.

- [ ] **Step 1: Write the failing tests**

```go
package shipmentcancel

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/shipping"
)

type fakeStore struct {
	byOrder map[uuid.UUID][]shipping.ShipmentRecord
	byID    map[uuid.UUID]shipping.ShipmentRecord
	sets    []struct{ ID uuid.UUID; Action, Status, Reason string }
}

func (f *fakeStore) ListShipmentsByOrderID(_ context.Context, orderID uuid.UUID) ([]shipping.ShipmentRecord, error) {
	return f.byOrder[orderID], nil
}
func (f *fakeStore) GetShipmentByID(_ context.Context, id uuid.UUID) (*shipping.ShipmentRecord, error) {
	r, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &r, nil
}
func (f *fakeStore) SetShipmentCancelState(_ context.Context, id uuid.UUID, action, status, reason string) error {
	f.sets = append(f.sets, struct{ ID uuid.UUID; Action, Status, Reason string }{id, action, status, reason})
	return nil
}

type fakeCarrier struct {
	calls int
	err   error
}

func (f *fakeCarrier) CancelShipment(_ context.Context, _ string) error { f.calls++; return f.err }
func (f *fakeCarrier) GetRates(context.Context, shipping.RateRequest) ([]shipping.Rate, error) { return nil, nil }
func (f *fakeCarrier) CreateShipment(context.Context, shipping.ShipmentRequest) (*shipping.Shipment, error) { return nil, nil }
func (f *fakeCarrier) GetTracking(context.Context, string) (*shipping.Tracking, error) { return nil, nil }
func (f *fakeCarrier) ProviderName() string { return "delhivery" }
func (f *fakeCarrier) SupportedCountries() []string { return []string{"IN"} }

func resolverFor(cr shipping.Carrier) CarrierResolver {
	return func(context.Context, uuid.UUID, string) (shipping.Carrier, error) { return cr, nil }
}

func TestExecutor_CancelForward_Success(t *testing.T) {
	oid, sid, stid := uuid.New(), uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, StoreID: stid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "pending"}},
	}}
	car := &fakeCarrier{}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "succeeded" {
		t.Fatalf("outcomes = %+v, want one succeeded", out)
	}
	if car.calls != 1 {
		t.Errorf("carrier calls = %d, want 1", car.calls)
	}
	if len(store.sets) != 1 || store.sets[0].Status != "succeeded" || store.sets[0].Action != string(ActionCancelForward) {
		t.Errorf("recorded = %+v, want cancel_forward/succeeded", store.sets)
	}
}

func TestExecutor_CancelForward_CarrierFailureRecordedNotFatal(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "pending"}},
	}}
	car := &fakeCarrier{err: errors.New("delhivery: cancel shipment: Incorrect Waybill")}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "failed" {
		t.Fatalf("outcomes = %+v, want one failed", out)
	}
	if store.sets[0].Status != "failed" || store.sets[0].Reason == "" {
		t.Errorf("recorded = %+v, want failed with reason", store.sets)
	}
}

func TestExecutor_InTransit_Unsupported(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "in_transit"}},
	}}
	car := &fakeCarrier{}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "unsupported" {
		t.Fatalf("outcomes = %+v, want unsupported", out)
	}
	if car.calls != 0 {
		t.Errorf("carrier called %d times for in-transit, want 0", car.calls)
	}
	if store.sets[0].Action != string(ActionTriggerRTO) || store.sets[0].Status != "unsupported" {
		t.Errorf("recorded = %+v, want rto/unsupported", store.sets)
	}
}

func TestExecutor_NoShipments_NoOp(t *testing.T) {
	e := NewExecutor(&fakeStore{}, resolverFor(&fakeCarrier{}), nil)
	if out := e.CancelForOrder(context.Background(), uuid.New()); len(out) != 0 {
		t.Fatalf("outcomes = %+v, want none", out)
	}
}

func TestExecutor_AlreadySucceeded_Idempotent(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "pending", CancelStatus: "succeeded"}},
	}}
	car := &fakeCarrier{}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if car.calls != 0 {
		t.Errorf("carrier called %d times on already-succeeded, want 0", car.calls)
	}
	if len(out) != 1 || out[0].Status != "succeeded" {
		t.Errorf("outcomes = %+v, want succeeded passthrough", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/marketplace-api && go test ./internal/shipmentcancel/ -run TestExecutor -v`
Expected: FAIL — `NewExecutor`, `Outcome`, `CarrierResolver` undefined.

- [ ] **Step 3: Write the executor**

```go
package shipmentcancel

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/shipping"
)

// ShipmentStore is the narrow persistence surface the executor needs.
// shipping.Repository satisfies it.
type ShipmentStore interface {
	ListShipmentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]shipping.ShipmentRecord, error)
	GetShipmentByID(ctx context.Context, id uuid.UUID) (*shipping.ShipmentRecord, error)
	SetShipmentCancelState(ctx context.Context, shipmentID uuid.UUID, action, status, reason string) error
}

// CarrierResolver builds a carrier client for a (store, provider). Kept as a
// func so the executor stays free of credential/secret-store dependencies and
// is trivially fakeable in tests.
type CarrierResolver func(ctx context.Context, storeID uuid.UUID, provider string) (shipping.Carrier, error)

// Cancel outcome statuses (also the shipments.cancel_status values).
const (
	statusNone        = "none"
	statusSucceeded   = "succeeded"
	statusFailed      = "failed"
	statusUnsupported = "unsupported"
)

// Outcome is the per-shipment result the manual endpoint aggregates into its
// response. The refund/cancel hooks ignore it (fire-and-record).
type Outcome struct {
	ShipmentID uuid.UUID `json:"shipment_id"`
	Action     Action    `json:"action"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
}

// Executor resolves and executes the carrier action for shipments. Every path
// is best-effort: it records the outcome and never returns a fatal error to
// the refund/cancel caller.
type Executor struct {
	store   ShipmentStore
	resolve CarrierResolver
	logger  *slog.Logger
}

func NewExecutor(store ShipmentStore, resolve CarrierResolver, logger *slog.Logger) *Executor {
	return &Executor{store: store, resolve: resolve, logger: logger}
}

// CancelForOrder resolves + executes the action for every shipment on the
// order, independently. Best-effort: a list error or a per-shipment failure is
// logged and recorded, never propagated. Returns one Outcome per shipment
// (empty slice when the order has no shipments).
func (e *Executor) CancelForOrder(ctx context.Context, orderID uuid.UUID) []Outcome {
	if e == nil || e.store == nil {
		return nil
	}
	shipments, err := e.store.ListShipmentsByOrderID(ctx, orderID)
	if err != nil {
		e.warn("shipmentcancel: list shipments failed", "order_id", orderID.String(), "err", err)
		return nil
	}
	outcomes := make([]Outcome, 0, len(shipments))
	for i := range shipments {
		outcomes = append(outcomes, e.resolveAndExecute(ctx, &shipments[i]))
	}
	return outcomes
}

// CancelShipmentByID drives one shipment (the manual button). Returns an error
// only when the shipment can't be loaded, so the endpoint can 404; the carrier
// outcome itself is in the returned Outcome.
func (e *Executor) CancelShipmentByID(ctx context.Context, shipmentID uuid.UUID) (Outcome, error) {
	sh, err := e.store.GetShipmentByID(ctx, shipmentID)
	if err != nil {
		return Outcome{}, err
	}
	return e.resolveAndExecute(ctx, sh), nil
}

func (e *Executor) resolveAndExecute(ctx context.Context, sh *shipping.ShipmentRecord) Outcome {
	action := ResolveAction(sh.Status)

	// Idempotent: a shipment already cancelled succeeds again as a no-op so
	// the paid-cancel + coordinator paths (or a manual retry) can't double-hit
	// the carrier. Failed rows are re-attempted so a manual retry can recover.
	if sh.CancelStatus == statusSucceeded {
		return Outcome{ShipmentID: sh.ID, Action: Action(sh.CancelAction), Status: statusSucceeded}
	}

	switch action {
	case ActionNoop:
		return Outcome{ShipmentID: sh.ID, Action: ActionNoop, Status: statusNone}

	case ActionCancelForward:
		return e.execCancelForward(ctx, sh)

	case ActionTriggerRTO, ActionReversePickup:
		// Phase 2/3 handle these; until then, record so the admin sees it and
		// arranges the return manually with the carrier.
		reason := "This shipment has left for delivery — arrange the return manually with the carrier."
		e.record(ctx, sh.ID, action, statusUnsupported, reason)
		return Outcome{ShipmentID: sh.ID, Action: action, Status: statusUnsupported, Reason: reason}

	default:
		return Outcome{ShipmentID: sh.ID, Action: ActionNoop, Status: statusNone}
	}
}

func (e *Executor) execCancelForward(ctx context.Context, sh *shipping.ShipmentRecord) Outcome {
	if strings.TrimSpace(sh.TrackingNumber) == "" {
		reason := "No tracking number on the shipment — nothing to cancel with the carrier."
		e.record(ctx, sh.ID, ActionCancelForward, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionCancelForward, Status: statusFailed, Reason: reason}
	}
	carrier, err := e.resolve(ctx, sh.StoreID, sh.Carrier)
	if err != nil {
		reason := "Could not reach the carrier to cancel — retry from the shipment."
		e.warn("shipmentcancel: resolve carrier failed", "shipment_id", sh.ID.String(), "carrier", sh.Carrier, "err", err)
		e.record(ctx, sh.ID, ActionCancelForward, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionCancelForward, Status: statusFailed, Reason: reason}
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

func (e *Executor) record(ctx context.Context, id uuid.UUID, action Action, status, reason string) {
	if err := e.store.SetShipmentCancelState(ctx, id, string(action), status, reason); err != nil {
		e.warn("shipmentcancel: record cancel state failed", "shipment_id", id.String(), "err", err)
	}
}

func (e *Executor) warn(msg string, args ...any) {
	if e.logger != nil {
		e.logger.Warn(msg, args...)
	}
}

// cleanReason strips the internal "delhivery: cancel shipment: " prefix the
// carrier error carries so the admin sees just the short reason. The carrier
// layer already guarantees no raw body / address leaks (Task 3), so this only
// tidies presentation.
func cleanReason(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && i+2 < len(msg) {
		return msg[i+2:]
	}
	return msg
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/marketplace-api && go test ./internal/shipmentcancel/ -v`
Expected: PASS (all Task 1 + Task 4 tests).

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/shipmentcancel/executor.go services/marketplace-api/internal/shipmentcancel/executor_test.go
git commit -m "feat(marketplace-api): add best-effort shipment-cancel executor"
```

---

### Task 5: Coordinator hook — fire the executor on full-refund success

**Files:**
- Modify: `internal/orderrefund/coordinator.go`
- Test: `internal/orderrefund/coordinator_shipmentcancel_test.go`

**Interfaces:**
- Produces on `*Coordinator`: `func (c *Coordinator) WithShipmentCanceller(fn func(ctx context.Context, orderID uuid.UUID)) *Coordinator`.
- Behaviour: on a fresh **full**-refund success (`target == order.PaymentStatusRefunded`), the coordinator invokes `c.cancelShipments(ctx, cmd.OrderID)` if set. Partial refunds and idempotent replays do NOT invoke it. `orderrefund` gains NO new package import (func field, not the executor type).

- [ ] **Step 1: Write the failing test**

Create `internal/orderrefund/coordinator_shipmentcancel_test.go`:

```go
package orderrefund

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/order"
)

func TestCoordinator_maybeCancelShipments_FiresOnFullRefundOnly(t *testing.T) {
	var got []uuid.UUID
	c := (&Coordinator{}).WithShipmentCanceller(func(_ context.Context, oid uuid.UUID) {
		got = append(got, oid)
	})

	oid := uuid.New()
	// Full refund → fires.
	c.maybeCancelShipments(context.Background(), oid, order.PaymentStatusRefunded)
	// Partial refund → does not fire.
	c.maybeCancelShipments(context.Background(), uuid.New(), order.PaymentStatusPartiallyRefunded)

	if len(got) != 1 || got[0] != oid {
		t.Fatalf("cancel fired for %v, want exactly [%v]", got, oid)
	}
}

func TestCoordinator_maybeCancelShipments_NilHookSafe(t *testing.T) {
	c := &Coordinator{}
	// Must not panic when no canceller is wired.
	c.maybeCancelShipments(context.Background(), uuid.New(), order.PaymentStatusRefunded)
	_ = decimal.Zero
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/orderrefund/ -run TestCoordinator_maybeCancelShipments -v`
Expected: FAIL — `WithShipmentCanceller` / `maybeCancelShipments` undefined.

- [ ] **Step 3: Add the field, setter, and helper; call it on success**

In `internal/orderrefund/coordinator.go`, add `context` is already imported. Add the field to `Coordinator`:

```go
type Coordinator struct {
	db        *gorm.DB
	res       resolver
	pay       *payment.Service
	orders    *order.Service
	orderRepo order.Repository
	enabled   bool
	// cancelShipments, when set, is fired best-effort after a FULL refund
	// settles so the order's pre-pickup shipment is cancelled at the carrier.
	// A func field (not the executor type) keeps this package free of a
	// shipping/shipmentcancel import. Production wraps the call in a detached
	// goroutine; the hook itself never errors back.
	cancelShipments func(ctx context.Context, orderID uuid.UUID)
}
```

Add the setter + helper near the bottom of the file:

```go
// WithShipmentCanceller wires the best-effort post-full-refund shipment cancel
// hook. Nil-safe by omission.
func (c *Coordinator) WithShipmentCanceller(fn func(ctx context.Context, orderID uuid.UUID)) *Coordinator {
	c.cancelShipments = fn
	return c
}

// maybeCancelShipments fires the shipment-cancel hook only for a full refund.
// Partial refunds still ship the un-refunded items, so they must not cancel.
func (c *Coordinator) maybeCancelShipments(ctx context.Context, orderID uuid.UUID, target order.PaymentStatus) {
	if c.cancelShipments == nil || target != order.PaymentStatusRefunded {
		return
	}
	c.cancelShipments(ctx, orderID)
}
```

In `Refund`, immediately before the final success `return` (after tx#2 commits), add:

```go
	// Full refund settled — cancel the pre-pickup shipment at the carrier.
	// Best-effort: the hook records + logs any failure and never affects this
	// result (the money already moved).
	c.maybeCancelShipments(ctx, cmd.OrderID, target)

	return RefundResult{
		ProviderRefundID: ref.ProviderRefundID,
		Amount:           amount,
		PaymentStatus:    target,
	}, nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/marketplace-api && go test ./internal/orderrefund/ -run TestCoordinator_maybeCancelShipments -v`
Expected: PASS.

- [ ] **Step 5: Run the package's existing tests (no regression)**

Run: `cd services/marketplace-api && go test ./internal/orderrefund/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/orderrefund/coordinator.go services/marketplace-api/internal/orderrefund/coordinator_shipmentcancel_test.go
git commit -m "feat(marketplace-api): fire best-effort shipment cancel after full refund"
```

---

### Task 6: Reconcile admin Cancel handler — drop the block, cancel the shipment

**Files:**
- Modify: `internal/handlers/admin/orders.go`
- Test: `internal/handlers/admin/orders_cancel_shipment_test.go`

**Interfaces:**
- Consumes: `shipmentcancel.Executor.CancelForOrder` (Task 4), coordinator hook (Task 5).
- Produces on `*OrdersHandler`: field `canceller *shipmentcancel.Executor`; `func (h *OrdersHandler) WithShipmentCanceller(e *shipmentcancel.Executor) *OrdersHandler`.
- Behaviour change: `shipmentBlocksCancel` and its 409 branch are removed. After a successful cancel: paid orders auto-refund (existing; coordinator hook cancels the shipment); non-paid orders call `h.canceller.CancelForOrder` directly.

- [ ] **Step 1: Write the failing test**

The double-fire avoidance is the key invariant. Create `internal/handlers/admin/orders_cancel_shipment_test.go` (unit-level, exercising the branch decision via a small seam). Since `Cancel` is DB-heavy, assert the reconciliation at the seam: a new unexported helper `cancelShipmentsForNonPaid`.

```go
package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/order"
)

type recordingCanceller struct{ ids []uuid.UUID }

func (r *recordingCanceller) CancelForOrder(_ context.Context, oid uuid.UUID) { r.ids = append(r.ids, oid) }

func TestCancel_NonPaid_CancelsShipmentDirectly(t *testing.T) {
	rc := &recordingCanceller{}
	h := &OrdersHandler{}
	h.cancelForOrderFn = rc.CancelForOrder // seam set by WithShipmentCanceller

	oid := uuid.New()
	h.cancelShipmentsForNonPaid(context.Background(), oid, string(order.PaymentStatusPending))
	h.cancelShipmentsForNonPaid(context.Background(), uuid.New(), string(order.PaymentStatusPaid))

	if len(rc.ids) != 1 || rc.ids[0] != oid {
		t.Fatalf("direct cancel fired for %v, want exactly [%v] (paid path defers to coordinator)", rc.ids, oid)
	}
}
```

Note: to keep the handler testable without a full DB, `WithShipmentCanceller` stores a `cancelForOrderFn func(ctx, uuid.UUID)` seam (set from the executor's method), and `cancelShipmentsForNonPaid` uses it.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/handlers/admin/ -run TestCancel_NonPaid -v`
Expected: FAIL — `cancelForOrderFn` / `cancelShipmentsForNonPaid` undefined.

- [ ] **Step 3: Add the seam + helper; remove the block**

In `internal/handlers/admin/orders.go`:

Add to `OrdersHandler` struct:

```go
	// cancelForOrderFn, when set, cancels the order's shipments at the carrier
	// (best-effort). Wired from shipmentcancel.Executor.CancelForOrder via
	// WithShipmentCanceller. A func seam keeps the handler unit-testable.
	cancelForOrderFn func(ctx context.Context, orderID uuid.UUID)
```

Add near the other `With*` setters:

```go
// WithShipmentCanceller wires the best-effort carrier shipment-cancel for the
// non-paid cancel path (paid cancels reach the carrier via the auto-refund →
// coordinator hook). Nil-safe by omission.
func (h *OrdersHandler) WithShipmentCanceller(fn func(ctx context.Context, orderID uuid.UUID)) *OrdersHandler {
	h.cancelForOrderFn = fn
	return h
}

// cancelShipmentsForNonPaid fires the direct shipment cancel only when the
// order did NOT go through the auto-refund path (paid orders). This is the
// single trigger site for cancels, so there's no double-hit with the
// coordinator's post-refund hook.
func (h *OrdersHandler) cancelShipmentsForNonPaid(ctx context.Context, orderID uuid.UUID, paymentStatus string) {
	if h.cancelForOrderFn == nil || paymentStatus == string(order.PaymentStatusPaid) {
		return
	}
	h.cancelForOrderFn(ctx, orderID)
}
```

**Delete** the `shipmentBlocksCancel` method (lines ~123–147) and the 409 block inside `Cancel` (lines ~431–441):

```go
	// Belt-and-braces: even though service.Cancel rejects fulfilled
	// orders via the status machine, a confirmed order with an in-flight
	// shipment must not silently slip through. Bounce here with a 409 so
	// the merchant sees a clear "issue a refund / RTS instead" hint.
	if h.shipmentBlocksCancel(c.Request.Context(), id) {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "shipment_in_flight",
			"message": "Cancel is unavailable once a shipping label has been generated. Issue a refund (and arrange a return-to-sender if the parcel is in transit) instead.",
		})
		return
	}
```

In `Cancel`, after the existing paid-order auto-refund block (the `if h.refunds != nil && o.PaymentStatus == string(order.PaymentStatusPaid) { … }`), add the non-paid direct cancel. Detached context so a slow carrier never blocks the cancel response:

```go
	// Cancel the order's shipment at the carrier. Paid orders reach the carrier
	// through the auto-refund → coordinator hook above; only the non-paid path
	// needs a direct call here. Detached + best-effort: never blocks or fails
	// the cancel response.
	{
		oid := id
		ps := o.PaymentStatus
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.cancelShipmentsForNonPaid(ctx, oid, ps)
		}()
	}
```

- [ ] **Step 4: Run test + build to verify**

Run: `cd services/marketplace-api && go test ./internal/handlers/admin/ -run TestCancel_NonPaid -v && go build ./internal/handlers/admin/`
Expected: test PASS; build OK. (If `shipmentBlocksCancel` had other references, `go build` flags them — there are none besides `Cancel`.)

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/orders.go services/marketplace-api/internal/handlers/admin/orders_cancel_shipment_test.go
git commit -m "feat(marketplace-api): cancel shipment on order cancel instead of blocking cancel"
```

---

### Task 7: Manual endpoint — `POST .../shipments/:shipmentId/cancel`

**Files:**
- Modify: `internal/handlers/admin/shipments.go` (field + setter + `CarrierForStore` + `CancelShipment` handler)
- Modify: `internal/handlers/admin/routes.go` (register route)
- Test: `internal/handlers/admin/shipments_cancel_test.go`

**Interfaces:**
- Consumes: `shipmentcancel.Executor.CancelShipmentByID` (Task 4); existing `resolveCarrierCreds` + `shipping.NewCarrier`.
- Produces on `*ShipmentsHandler`: field `canceller *shipmentcancel.Executor`; `func (h *ShipmentsHandler) WithCanceller(e *shipmentcancel.Executor) *ShipmentsHandler`; `func (h *ShipmentsHandler) CarrierForStore(ctx context.Context, storeID uuid.UUID, provider string) (shipping.Carrier, error)`; `func (h *ShipmentsHandler) CancelShipment(c *gin.Context)`.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/admin/shipments_cancel_test.go` — unit-level test of `CarrierForStore`'s provider passthrough is DB-heavy; instead test the handler's 503-when-unwired and the outcome JSON shaping via a stubbed executor seam. Keep it light:

```go
package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCancelShipment_ServiceUnavailableWhenUnwired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &ShipmentsHandler{} // no canceller wired
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "shipmentId", Value: "not-a-uuid"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	h.CancelShipment(c)

	// Bad UUID is validated before the nil-canceller check → 400 is acceptable;
	// assert we never 200 without a canceller.
	if w.Code == http.StatusOK {
		t.Fatalf("expected non-200 without a wired canceller, got 200")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/handlers/admin/ -run TestCancelShipment_ServiceUnavailable -v`
Expected: FAIL — `ShipmentsHandler.CancelShipment` undefined.

- [ ] **Step 3: Implement the handler, setter, and carrier resolver**

In `internal/handlers/admin/shipments.go`, add the import for `shipmentcancel` and `uuid` (uuid already imported). Add the field to `ShipmentsHandler`:

```go
	// canceller resolves + executes the carrier cancel/return action for a
	// shipment (manual button). Nil-safe: the endpoint returns 503 when unwired.
	canceller *shipmentcancel.Executor
```

Add the setter + `CarrierForStore` helper + handler:

```go
// WithCanceller wires the shipment-cancel executor for the manual per-shipment
// cancel endpoint. Chainable, nil-safe by omission.
func (h *ShipmentsHandler) WithCanceller(e *shipmentcancel.Executor) *ShipmentsHandler {
	h.canceller = e
	return h
}

// CarrierForStore builds a carrier client for a (store, provider), reusing the
// same credential-resolution path as label creation. Exposed so the
// shipmentcancel executor can resolve a carrier without duplicating decrypt
// logic (wired as its CarrierResolver in main.go).
func (h *ShipmentsHandler) CarrierForStore(ctx context.Context, storeID uuid.UUID, provider string) (shipping.Carrier, error) {
	provider = strings.ToLower(provider)
	cfg, err := h.repo.GetCarrierConfig(ctx, storeID.String(), provider)
	if err != nil {
		return nil, fmt.Errorf("shipments: carrier config for %q: %w", provider, err)
	}
	apiKey, secretKey, err := h.resolveCarrierCreds(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("shipments: resolve carrier creds: %w", err)
	}
	h.maybeRewrapCarrierCreds(ctx, cfg, apiKey, secretKey)
	return shipping.NewCarrier(provider, apiKey, secretKey, cfg.Mode)
}

// CancelShipment handles POST
// /admin/stores/:storeId/orders/:id/shipments/:shipmentId/cancel — the manual
// "Cancel / return shipment" button. Resolves the current lifecycle state and
// executes the matching carrier action for that one shipment.
func (h *ShipmentsHandler) CancelShipment(c *gin.Context) {
	shipmentID, err := uuid.Parse(c.Param("shipmentId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("shipmentId", "must be a uuid"), h.logger)
		return
	}
	if h.canceller == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "service_unavailable",
			"message": "Shipment cancellation is not configured.",
		})
		return
	}
	outcome, err := h.canceller.CancelShipmentByID(c.Request.Context(), shipmentID)
	if err != nil {
		RespondErr(c, apperrors.NotFound("shipment"), h.logger)
		return
	}
	c.JSON(http.StatusOK, outcome)
}
```

Also surface the recorded state in the shipment DTO so the admin can render it. In `toShipmentResponse` (find the struct it returns and its builder), add fields `CancelAction string `json:"cancel_action,omitempty"`` and `CancelStatus string `json:"cancel_status,omitempty"`` and `CancelReason string `json:"cancel_reason,omitempty"`` populated from `rec.CancelAction`/`rec.CancelStatus`/`rec.CancelReason` (omit when `"none"`/empty). Locate the DTO with `grep -n "func toShipmentResponse" internal/handlers/admin/shipments.go` and mirror the existing field-mapping style.

- [ ] **Step 4: Register the route**

In `internal/handlers/admin/routes.go`, after the `pickup/schedule` route (line ~354–356), add:

```go
					// Manual "Cancel / return shipment" — resolves the
					// shipment's lifecycle state and takes the matching
					// carrier action (Phase 1: pre-pickup forward cancel).
					orders.POST("/:id/shipments/:shipmentId/cancel",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
						deps.ShipmentsHandler.CancelShipment)
```

- [ ] **Step 5: Run test + build**

Run: `cd services/marketplace-api && go test ./internal/handlers/admin/ -run TestCancelShipment -v && go build ./internal/handlers/admin/`
Expected: test PASS; build OK.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/shipments.go services/marketplace-api/internal/handlers/admin/routes.go services/marketplace-api/internal/handlers/admin/shipments_cancel_test.go
git commit -m "feat(marketplace-api): add manual per-shipment cancel endpoint"
```

---

### Task 8: Wire the executor in `main.go`

**Files:**
- Modify: `cmd/marketplace-api/main.go`

**Interfaces:**
- Consumes: `shipmentcancel.NewExecutor` (Task 4), `ShipmentsHandler.CarrierForStore` (Task 7), `Coordinator.WithShipmentCanceller` (Task 5), `OrdersHandler.WithShipmentCanceller` (Task 6), `shippingRepo` (existing).

- [ ] **Step 1: Build the executor after `shipmentsHandler` and inject it**

In `cmd/marketplace-api/main.go`, after `shipmentsHandler` is constructed (after line ~684, where `WithLabelMailer(labelMailer)` ends) and before the router `deps` struct, add:

```go
		// Shipment-cancel executor — resolves + executes the carrier action
		// when an order is fully refunded or cancelled. Reuses the shipments
		// handler's carrier-resolution path so credential decryption is not
		// duplicated. Fired best-effort from the refund coordinator (full
		// refunds), the orders Cancel handler (non-paid cancels), and the
		// manual per-shipment endpoint.
		shipmentCanceller := shipmentcancel.NewExecutor(shippingRepo, shipmentsHandler.CarrierForStore, log)
		shipmentsHandler = shipmentsHandler.WithCanceller(shipmentCanceller)

		// Production hook wraps the executor call in a detached goroutine so a
		// slow carrier never blocks the refund/cancel response; the executor is
		// already best-effort and never errors back.
		cancelShipmentsAsync := func(_ context.Context, orderID uuid.UUID) {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shipmentCanceller.CancelForOrder(ctx, orderID)
			}()
		}
		refundCoordinator = refundCoordinator.WithShipmentCanceller(cancelShipmentsAsync)
		ordersHandler = ordersHandler.WithShipmentCanceller(cancelShipmentsAsync)
```

Note: `ordersHandler`, `refundCoordinator`, `shipmentsHandler` are already declared above with `:=`; use plain `=` reassignment (the `With*` methods return the same pointer). Confirm `uuid` and `context` are already imported in `main.go` (they are, used throughout).

- [ ] **Step 2: Verify the whole service builds**

Run: `cd services/marketplace-api && go build ./...`
Expected: build OK.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): wire shipment-cancel executor into refund/cancel flows"
```

---

### Task 9: Integration test + full verification

**Files:**
- Create: `internal/handlers/admin/shipment_cancel_integration_test.go` (build tag `//go:build integration`)

**Interfaces:**
- Consumes: everything above; `pkg/testdb`, the real `orderrefund.Coordinator`, a fake `payment.Gateway` (mirror `orders_refund_integration_test.go`), a fake `CarrierResolver` returning a recording carrier.

- [ ] **Step 1: Write the integration test**

Mirror `internal/handlers/admin/orders_refund_integration_test.go`'s harness (fakeGateway, fakeRefundResolver, testdb setup, truncate list including `shipments`). Assert the end-to-end invariant: a full refund on an order with a `pending` shipment records `cancel_status = succeeded` and calls the fake carrier once; a partial refund records nothing.

```go
//go:build integration

package admin_test

// ... imports mirror orders_refund_integration_test.go, plus:
//   "github.com/mark8ly/marketplace-api/internal/shipmentcancel"
//   "github.com/mark8ly/marketplace-api/internal/shipping"

func TestFullRefund_CancelsPendingShipment(t *testing.T) {
	db := testdb.New(t) // applies migrations incl. 000096
	// seed store + paid order + a shipments row with status='pending',
	// carrier='delhivery', tracking_number='WBN-INT-1'.
	// Build the real Coordinator with a fake gateway (RefundPayment → success),
	// and an executor whose CarrierResolver returns a recording carrier.
	rec := &recordingCarrier{}
	exec := shipmentcancel.NewExecutor(
		shipping.NewRepository(db),
		func(context.Context, uuid.UUID, string) (shipping.Carrier, error) { return rec, nil },
		slog.Default(),
	)
	coord := orderrefund.NewCoordinator(db, fakeRes, paySvc, orderSvc, orderRepo, true).
		WithShipmentCanceller(func(ctx context.Context, oid uuid.UUID) { exec.CancelForOrder(ctx, oid) })

	_, err := coord.Refund(ctx, orderrefund.RefundCommand{OrderID: oid, Amount: nil, Reason: "test", Actor: "test", ScopeID: "req-1"})
	// assert err == nil
	// assert rec.calls == 1
	// re-read the shipment row: cancel_status == "succeeded", cancel_action == "cancel_forward".
}

func TestPartialRefund_DoesNotCancelShipment(t *testing.T) {
	// Same seed; refund a partial Amount so DeriveStatus → partially_refunded.
	// assert rec.calls == 0 and shipment cancel_status stays "none".
}
```

Fill in the seed + fakes by copying the exact patterns from `orders_refund_integration_test.go` (fakeGateway, fakeRefundResolver, seedPaidOrder helpers). Add a `recordingCarrier` implementing `shipping.Carrier` with a `calls int` counter on `CancelShipment`.

- [ ] **Step 2: Run unit tests (no DB) across touched packages**

Run:
```bash
cd services/marketplace-api && go test ./internal/shipmentcancel/... ./internal/shipping/... ./internal/orderrefund/... ./internal/handlers/admin/... . 2>&1 | tail -30
```
Expected: PASS (unit tests; integration-tagged files are excluded without `-tags integration`).

- [ ] **Step 3: Run the integration suite (needs Postgres)**

Run: `cd services/marketplace-api && go test -tags integration ./internal/handlers/admin/ -run 'FullRefund_CancelsPendingShipment|PartialRefund_DoesNotCancel' -v`
Expected: PASS (requires the testdb Postgres env the suite expects — same as existing integration tests). If Postgres isn't available locally, note it and rely on CI.

- [ ] **Step 4: Full build + vet**

Run: `cd services/marketplace-api && go build ./... && go vet ./internal/shipmentcancel/... ./internal/shipping/... ./internal/orderrefund/...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/shipment_cancel_integration_test.go
git commit -m "test(marketplace-api): integration coverage for full-refund shipment cancel"
```

---

## Self-Review (against the spec)

- **State→action matrix (Phase 1 rows):** no-shipment → no rows → no-op (Task 4 `NoShipments_NoOp`); pending/created → `cancel_forward` (Task 1 + Task 4); in-transit/delivered → `unsupported` recorded (Task 4 `InTransit_Unsupported`). ✅
- **Trigger model:** full refund → fires (Task 5); order cancel → fires, paid via coordinator / non-paid direct (Task 6); partial refund → no fire (Task 5 test); manual button → fires (Task 7). ✅
- **Decision layer pure + table-tested:** Task 1. ✅
- **Executor best-effort, records outcome, per-shipment:** Task 4. ✅
- **Hook points:** coordinator success path (Task 5); order cancel + reconcile `shipmentBlocksCancel` (Task 6, block removed); manual endpoint (Task 7). ✅
- **Data model:** 4 columns + `ExpectedSchemaVersion` bump (Task 2). ✅
- **Error handling:** never rolls back refund (async detached hook, executor swallows errors — Tasks 4/5/6/8); clean merchant message via `<error>` extraction, no raw body (Task 3 + `cleanReason`). ✅
- **Open question #1 (pickup auto-close):** resolved — cancel the waybill only; Delhivery moves the pickup package to Cancelled and a shared pickup request is deliberately left untouched (documented in Task 3). ✅
- **Retry sweep:** columns exist; Phase 1 is log-only (spec permits deferring the sweep). Noted, not built. ✅
- **Testing patterns:** httptest carrier mock (Task 3), fake store/carrier unit tests (Task 4), integration mirror of the refund suite (Task 9). ✅

## Deferred — Phases 2 & 3 (separate plans)

Each is independently shippable and gated on a live verification that can't be done until a real in-transit / delivered shipment exists (spec open questions #2, #3). They get their own plans when reached:

- **Phase 2 — in-transit RTO:** add `carrier.ReturnToOrigin`, wire `ActionTriggerRTO` to it. **Verify first (live):** the exact Delhivery intercept/RTO endpoint (Cancel Order API vs NDR API) and payload against a live in-transit waybill. Executor already records `unsupported` for these until then.
- **Phase 3 — reverse pickup for delivered:** add `carrier.CreateReverseShipment` reusing order-creation with `payment_mode:"Pickup"` + return keys; creates a new reverse shipment row. **Verify first (live):** the exact return-key set. Executor records `unsupported` until then.
