# Refund / Cancel → Shipment Lifecycle — Phase 3 Implementation Plan (reverse pickup)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** When an order with a **delivered** shipment is fully refunded or cancelled, create a Delhivery reverse pickup (collect from the customer, return to the warehouse) and record a new reverse-leg shipment row — gated behind an env flag defaulting OFF until the payload is live-verified.

**Architecture:** New optional carrier capability `ReverseShipmentCreator`, which Delhivery implements by reusing `/api/cmu/create.json` with `payment_mode:"Pickup"` + `return_*` keys (auto-schedules). The executor's `ActionReversePickup` (currently `unsupported`) resolves the carrier, builds a reverse request from the delivered shipment's stored addresses (pickup-from = customer/`ship_to`, return-to = warehouse/`ship_from`), calls the carrier, inserts a new reverse-leg `shipments` row, and records the forward shipment's outcome. A single kill switch (`Executor.reversePickupEnabled`, wired from `REVERSE_PICKUP_ENABLED`) records `unsupported` when off — mirroring `REFUND_GATEWAY_ENABLED`.

**Tech Stack:** Go 1.26.5, httptest. Service: `services/marketplace-api`. No migration.

## Open-question resolution (spec Q3 — verified against docs 2026-07-18)

- Endpoint: **same** `POST https://track.delhivery.com/api/cmu/create.json` as forward.
- `payment_mode: "Pickup"` for reverse (vs "Pre-paid"/"COD" forward).
- Shipment entry fields include `return_name, return_add, return_pin, return_city, return_state, return_country, return_phone` alongside the consignee fields.
- Reverse pickup = "picked from the customer and delivered to the client warehouse." ⇒ consignee fields (`name/add/pin/city/state/country/phone`) = **customer**; `return_*` = **warehouse**; `pickup_location.name` = registered warehouse name (case-sensitive).
- "Reverse shipment will be scheduled automatically" — no separate pickup-request call.
- **Not live-verified** (no delivered shipment to test non-destructively): exact `return_country` format and per-entry weight are doc-derived. Hence the kill switch — see below.

## Global Constraints

- Best-effort: a reverse-pickup failure never affects the refund/cancel (executor swallows + records).
- **Kill switch:** `REVERSE_PICKUP_ENABLED` (default `false`). Off ⇒ delivered shipments record `unsupported` (unchanged from Phase 1/2). Mirrors `REFUND_GATEWAY_ENABLED` in `cmd/marketplace-api/main.go`.
- Merchant-facing reason via the carrier's clean error (reuses the hardened create-error classifier).
- Optional capability interface (type-assert) — do NOT add to the base `Carrier` interface.
- Single-line conventional commits, no signatures, no PRs.

## File Structure

- `internal/shipping/carrier.go` — MODIFY: add `ReverseShipmentCreator` interface + `ReverseShipmentRequest` type; add `return_*` (omitempty) fields to `dlShipmentEntry`.
- `internal/shipping/delhivery.go` — MODIFY: extract a shared create-response parser from `CreateShipment`; add `CreateReverseShipment`.
- `internal/shipping/delhivery_test.go` — MODIFY: reverse-create tests (payment_mode, return_* keys, address mapping, waybill, failure body).
- `internal/shipmentcancel/executor.go` — MODIFY: `ShipmentStore` gains `CreateShipment`; `Executor` gains `reversePickupEnabled` + `WithReversePickup`; `ActionReversePickup` → `execReversePickup`.
- `internal/shipmentcancel/executor_test.go` — MODIFY: fakeStore gains `CreateShipment`; reverse tests (flag on/off, capable/incapable carrier, failure).
- `cmd/marketplace-api/main.go` — MODIFY: read `REVERSE_PICKUP_ENABLED`, `.WithReversePickup(...)` on the executor.
- `internal/handlers/admin/shipment_cancel_integration_test.go` — MODIFY: recordingCarrier gains `CreateReverseShipment`; delivered → reverse integration test.

---

### Task 1: Delhivery `CreateReverseShipment` capability

**Files:**
- Modify: `internal/shipping/carrier.go`, `internal/shipping/delhivery.go`
- Test: `internal/shipping/delhivery_test.go`

**Interfaces:**
- Produces in `shipping`:
  ```go
  type ReverseShipmentRequest struct {
      OrderID       string
      PickupFrom    Address // customer (forward destination) — where Delhivery collects
      ReturnTo      Address // warehouse (forward origin) — where the parcel is returned
      WarehouseName string  // registered pickup_location.name (case-sensitive)
      Items         []ParcelItem
      CurrencyCode  string
  }
  type ReverseShipmentCreator interface {
      CreateReverseShipment(ctx context.Context, in ReverseShipmentRequest) (*Shipment, error)
  }
  ```
- Produces `func (c *DelhiveryCarrier) CreateReverseShipment(ctx, in ReverseShipmentRequest) (*Shipment, error)`.

- [ ] **Step 1: Write the failing test**

Add to `internal/shipping/delhivery_test.go`:

```go
func TestDelhivery_CreateReverseShipment_PickupPayload(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotBody = r.FormValue("data")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"packages":[{"waybill":"REV123","status":"Success","serviceable":true,"remarks":[]}]}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	sh, err := c.CreateReverseShipment(context.Background(), ReverseShipmentRequest{
		OrderID:       "ORD-1-RET",
		PickupFrom:    Address{Name: "Jane Doe", Line1: "42 Example Lane", City: "Mumbai", Region: "MH", PostalCode: "400001", CountryCode: "IN", Phone: "9111111111"},
		ReturnTo:      Address{Name: "Warehouse A", Line1: "1 Store Rd", City: "Bengaluru", Region: "Karnataka", PostalCode: "560001", CountryCode: "IN", Phone: "9000000000"},
		WarehouseName: "Warehouse A",
		Items:         []ParcelItem{{Title: "Mug", Quantity: 1, WeightGrams: 500}},
	})
	if err != nil {
		t.Fatalf("CreateReverseShipment: %v", err)
	}
	if sh.TrackingNumber != "REV123" {
		t.Errorf("waybill = %q, want REV123", sh.TrackingNumber)
	}
	if gotPath != "/api/cmu/create.json" {
		t.Errorf("path = %q, want /api/cmu/create.json", gotPath)
	}
	// payment_mode Pickup + consignee = customer + return_* = warehouse.
	for _, want := range []string{`"payment_mode":"Pickup"`, `"name":"Jane Doe"`, `"pin":"400001"`,
		`"return_add":"1 Store Rd"`, `"return_pin":"560001"`, `"return_phone":"9000000000"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("data payload missing %s\ngot: %s", want, gotBody)
		}
	}
}

func TestDelhivery_CreateReverseShipment_FailureRemark(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"packages":[{"waybill":"","status":"Fail","serviceable":true,"remarks":["Return pin not serviceable"]}]}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	_, err := c.CreateReverseShipment(context.Background(), ReverseShipmentRequest{
		OrderID: "ORD-2", PickupFrom: Address{PostalCode: "400001", Phone: "9"}, ReturnTo: Address{PostalCode: "560001"},
		WarehouseName: "W", Items: []ParcelItem{{Quantity: 1, WeightGrams: 500}},
	})
	if err == nil || !strings.Contains(err.Error(), "serviceable") {
		t.Fatalf("err = %v, want the remark surfaced", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/shipping/ -run TestDelhivery_CreateReverseShipment -v`
Expected: FAIL — `CreateReverseShipment` / `ReverseShipmentRequest` undefined.

- [ ] **Step 3: Add interface + type + return_* fields**

In `internal/shipping/carrier.go`, after `ReturnToOriginer`, add:

```go
// ReverseShipmentRequest describes a return pickup: Delhivery collects FROM the
// customer (PickupFrom, the forward destination) and returns TO the warehouse
// (ReturnTo, the forward origin). WarehouseName is the registered pickup
// location name (case-sensitive) sent as pickup_location.name.
type ReverseShipmentRequest struct {
	OrderID       string
	PickupFrom    Address
	ReturnTo      Address
	WarehouseName string
	Items         []ParcelItem
	CurrencyCode  string
}

// ReverseShipmentCreator is implemented by carriers that can create a reverse
// (return) pickup. Delhivery reuses its order-creation call with
// payment_mode:"Pickup" + return_* keys; the reverse leg auto-schedules.
// Carriers that don't implement this record an "unsupported" outcome.
type ReverseShipmentCreator interface {
	CreateReverseShipment(ctx context.Context, in ReverseShipmentRequest) (*Shipment, error)
}
```

In `internal/shipping/delhivery.go`, extend `dlShipmentEntry` (after `TotalAmount`) with omitempty return_* fields so the forward path (which never sets them) is unchanged:

```go
	// Reverse-pickup only (payment_mode:"Pickup"). Omitted on forward shipments.
	ReturnName    string `json:"return_name,omitempty"`
	ReturnAdd     string `json:"return_add,omitempty"`
	ReturnPin     string `json:"return_pin,omitempty"`
	ReturnCity    string `json:"return_city,omitempty"`
	ReturnState   string `json:"return_state,omitempty"`
	ReturnCountry string `json:"return_country,omitempty"`
	ReturnPhone   string `json:"return_phone,omitempty"`
```

- [ ] **Step 4: Extract a shared response parser + add `CreateReverseShipment`**

In `CreateShipment`, the block from `bodyBytes, readErr := io.ReadAll(resp.Body)` through the final `return &Shipment{…}` builds the result from the response. Extract it verbatim into a helper and call it from both methods:

```go
// parseDelhiveryCreateResponse turns a /api/cmu/create.json HTTP response into
// a Shipment or a classified error. Shared by forward CreateShipment and
// reverse CreateReverseShipment — both use the identical response schema.
func (c *DelhiveryCarrier) parseDelhiveryCreateResponse(resp *http.Response, warehouseName, fromPin, toPin string) (*Shipment, error) {
	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("delhivery: create shipment: read body: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("delhivery: create shipment: status=%d body=%s",
			resp.StatusCode, truncate(string(bodyBytes), 400))
	}
	var cr dlCreateResponse
	if err := json.Unmarshal(bodyBytes, &cr); err != nil {
		return nil, fmt.Errorf("delhivery: create shipment: decode: %w (body=%s)",
			err, truncate(string(bodyBytes), 400))
	}
	if len(cr.Packages) == 0 {
		if remark := strings.TrimSpace(cr.Rmk); remark != "" {
			return nil, classifyDelhiveryCreateError(remark, warehouseName, fromPin, toPin, false)
		}
		return nil, fmt.Errorf("delhivery: create shipment: empty packages (body=%s)",
			truncate(string(bodyBytes), 400))
	}
	pkg := cr.Packages[0]
	if strings.EqualFold(pkg.Status, "Fail") || pkg.Waybill == "" {
		remark := joinRemarks(pkg.Remarks)
		return nil, classifyDelhiveryCreateError(remark, warehouseName, fromPin, toPin, pkg.Serviceable)
	}
	return &Shipment{
		ProviderShipmentID: pkg.Waybill,
		TrackingNumber:     pkg.Waybill,
		Carrier:            "delhivery",
		Service:            "standard",
	}, nil
}
```

Then replace that inline block in `CreateShipment` with:

```go
	resp, err := c.doForm(ctx, "/api/cmu/create.json", form)
	if err != nil {
		return nil, fmt.Errorf("delhivery: create shipment: %w", err)
	}
	defer resp.Body.Close()
	return c.parseDelhiveryCreateResponse(resp, in.FromAddress.Name, in.FromAddress.PostalCode, in.ToAddress.PostalCode)
```

Add `CreateReverseShipment` (after `CreateShipment`):

```go
// CreateReverseShipment creates a Delhivery reverse (return) pickup, reusing
// /api/cmu/create.json with payment_mode:"Pickup". The consignee fields carry
// the customer (PickupFrom) — where Delhivery collects — and the return_* keys
// carry the warehouse (ReturnTo) — where the parcel is returned. Delhivery
// auto-schedules the reverse leg, so no separate pickup request is needed.
//
// NOTE (unverified): the exact return_country format and default parcel weight
// are doc-derived, not verified against a live delivered shipment. Gated behind
// REVERSE_PICKUP_ENABLED at the executor.
func (c *DelhiveryCarrier) CreateReverseShipment(ctx context.Context, in ReverseShipmentRequest) (*Shipment, error) {
	totalWeightGrams := 0
	for _, item := range in.Items {
		totalWeightGrams += item.WeightGrams * item.Quantity
	}
	if totalWeightGrams == 0 {
		totalWeightGrams = 500 // Delhivery rejects zero weight; same fallback as forward.
	}
	productDesc := "Return"

	shipmentData := dlShipmentData{}
	shipmentData.PickupLocation.Name = in.WarehouseName
	shipmentData.PickupLocation.AddLine1 = in.ReturnTo.Line1
	shipmentData.PickupLocation.City = in.ReturnTo.City
	shipmentData.PickupLocation.PinCode = in.ReturnTo.PostalCode
	shipmentData.PickupLocation.Country = in.ReturnTo.CountryCode
	shipmentData.PickupLocation.Phone = in.ReturnTo.Phone
	shipmentData.PickupLocation.State = in.ReturnTo.Region
	shipmentData.Shipments = []dlShipmentEntry{
		{
			Name:          in.PickupFrom.Name,
			Add:           in.PickupFrom.Line1,
			City:          in.PickupFrom.City,
			Pin:           in.PickupFrom.PostalCode,
			State:         in.PickupFrom.Region,
			Country:       in.PickupFrom.CountryCode,
			Phone:         in.PickupFrom.Phone,
			OrderID:       in.OrderID,
			PaymentMode:   "Pickup",
			ProductDesc:   productDesc,
			Weight:        float64(totalWeightGrams),
			ReturnName:    firstNonEmpty(in.ReturnTo.Name, in.WarehouseName),
			ReturnAdd:     in.ReturnTo.Line1,
			ReturnPin:     in.ReturnTo.PostalCode,
			ReturnCity:    in.ReturnTo.City,
			ReturnState:   in.ReturnTo.Region,
			ReturnCountry: delhiveryCountryName(in.ReturnTo.CountryCode),
			ReturnPhone:   in.ReturnTo.Phone,
		},
	}

	dataJSON, err := json.Marshal(shipmentData)
	if err != nil {
		return nil, fmt.Errorf("delhivery: create reverse shipment: marshal data: %w", err)
	}
	form := url.Values{"format": {"json"}, "data": {string(dataJSON)}}
	resp, err := c.doForm(ctx, "/api/cmu/create.json", form)
	if err != nil {
		return nil, fmt.Errorf("delhivery: create reverse shipment: %w", err)
	}
	defer resp.Body.Close()
	return c.parseDelhiveryCreateResponse(resp, in.WarehouseName, in.ReturnTo.PostalCode, in.PickupFrom.PostalCode)
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `cd services/marketplace-api && go test ./internal/shipping/`
Expected: PASS (new reverse tests + all existing forward tests via the shared parser).

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/shipping/carrier.go services/marketplace-api/internal/shipping/delhivery.go services/marketplace-api/internal/shipping/delhivery_test.go
git commit -m "feat(marketplace-api): add delhivery reverse-pickup (return) shipment creation"
```

---

### Task 2: Executor `ActionReversePickup` (flag-gated)

**Files:**
- Modify: `internal/shipmentcancel/executor.go`
- Test: `internal/shipmentcancel/executor_test.go`

**Interfaces:**
- `ShipmentStore` gains `CreateShipment(ctx context.Context, rec *shipping.ShipmentRecord) error` (shipping.Repository already satisfies it).
- `*Executor` gains `func (e *Executor) WithReversePickup(enabled bool) *Executor` and internal `reversePickupEnabled bool`.
- Behaviour: `ActionReversePickup` → if flag off → `unsupported` ("automatic reverse pickup is off"); else resolve carrier, type-assert `ReverseShipmentCreator` (else `unsupported`), build the request from the delivered shipment's swapped addresses, call the carrier, insert a reverse-leg row, record forward `reverse_pickup`/`succeeded`|`failed`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/shipmentcancel/executor_test.go`. First, add `CreateShipment` to `fakeStore`:

```go
func (f *fakeStore) CreateShipment(_ context.Context, rec *shipping.ShipmentRecord) error {
	f.created = append(f.created, *rec)
	return nil
}
```
(add `created []shipping.ShipmentRecord` to the `fakeStore` struct.)

Then a reverse-capable carrier + tests:

```go
type fakeReverseCarrier struct {
	fakeCarrier
	revCalls int
	revReq   shipping.ReverseShipmentRequest
	revErr   error
}

func (f *fakeReverseCarrier) CreateReverseShipment(_ context.Context, in shipping.ReverseShipmentRequest) (*shipping.Shipment, error) {
	f.revCalls++
	f.revReq = in
	if f.revErr != nil {
		return nil, f.revErr
	}
	return &shipping.Shipment{TrackingNumber: "REV-NEW", ProviderShipmentID: "REV-NEW", Carrier: "delhivery"}, nil
}

func deliveredShipment(sid, stid uuid.UUID) shipping.ShipmentRecord {
	return shipping.ShipmentRecord{
		ID: sid, StoreID: stid, Carrier: "delhivery", TrackingNumber: "WBN-DLV", Status: "delivered",
		ShipFrom: []byte(`{"name":"Warehouse A","line1":"1 Store Rd","city":"Bengaluru","region":"KA","postal_code":"560001","country_code":"IN","phone":"9000000000"}`),
		ShipTo:   []byte(`{"name":"Jane Doe","line1":"42 Lane","city":"Mumbai","region":"MH","postal_code":"400001","country_code":"IN","phone":"9111111111"}`),
	}
}

func TestExecutor_Delivered_ReversePickup_Disabled(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{oid: {deliveredShipment(sid, uuid.New())}}}
	car := &fakeReverseCarrier{}
	e := NewExecutor(store, resolverFor(car), nil) // reverse pickup off by default

	out := e.CancelForOrder(context.Background(), oid)
	if out[0].Status != "unsupported" || out[0].Action != ActionReversePickup {
		t.Fatalf("out = %+v, want reverse_pickup/unsupported when disabled", out)
	}
	if car.revCalls != 0 || len(store.created) != 0 {
		t.Errorf("reverse created while disabled (calls=%d rows=%d)", car.revCalls, len(store.created))
	}
}

func TestExecutor_Delivered_ReversePickup_Enabled(t *testing.T) {
	oid, sid, stid := uuid.New(), uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{oid: {deliveredShipment(sid, stid)}}}
	car := &fakeReverseCarrier{}
	e := NewExecutor(store, resolverFor(car), nil).WithReversePickup(true)

	out := e.CancelForOrder(context.Background(), oid)
	if out[0].Status != "succeeded" || out[0].Action != ActionReversePickup {
		t.Fatalf("out = %+v, want reverse_pickup/succeeded", out)
	}
	if car.revCalls != 1 {
		t.Fatalf("CreateReverseShipment calls = %d, want 1", car.revCalls)
	}
	// pickup FROM customer, return TO warehouse.
	if car.revReq.PickupFrom.PostalCode != "400001" || car.revReq.ReturnTo.PostalCode != "560001" {
		t.Errorf("address mapping wrong: pickup=%q return=%q", car.revReq.PickupFrom.PostalCode, car.revReq.ReturnTo.PostalCode)
	}
	if car.revReq.WarehouseName != "Warehouse A" {
		t.Errorf("warehouse name = %q, want Warehouse A", car.revReq.WarehouseName)
	}
	// new reverse-leg row inserted, marked so a re-run skips it.
	if len(store.created) != 1 {
		t.Fatalf("reverse-leg rows = %d, want 1", len(store.created))
	}
	rev := store.created[0]
	if rev.TrackingNumber != "REV-NEW" || rev.OrderID != store.byOrder[oid][0].OrderID {
		t.Errorf("reverse-leg row wrong: %+v", rev)
	}
	if rev.CancelStatus != "succeeded" || rev.CancelAction != string(ActionReversePickup) {
		t.Errorf("reverse-leg not marked (status=%q action=%q) — re-run would try to cancel it", rev.CancelStatus, rev.CancelAction)
	}
	// forward shipment recorded reverse_pickup/succeeded.
	if store.sets[0].Action != string(ActionReversePickup) || store.sets[0].Status != "succeeded" {
		t.Errorf("forward record = %+v, want reverse_pickup/succeeded", store.sets)
	}
}

func TestExecutor_Delivered_ReversePickup_CarrierFailure(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{oid: {deliveredShipment(sid, uuid.New())}}}
	car := &fakeReverseCarrier{revErr: errors.New("delhivery: create shipment: Return pin not serviceable")}
	e := NewExecutor(store, resolverFor(car), nil).WithReversePickup(true)

	out := e.CancelForOrder(context.Background(), oid)
	if out[0].Status != "failed" {
		t.Fatalf("out = %+v, want failed on carrier error", out)
	}
	if len(store.created) != 0 {
		t.Errorf("reverse-leg row created despite carrier failure")
	}
	if store.sets[0].Reason != "Return pin not serviceable" {
		t.Errorf("reason = %q, want cleaned carrier reason", store.sets[0].Reason)
	}
}

func TestExecutor_Delivered_NoReverseCapability_Unsupported(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{oid: {deliveredShipment(sid, uuid.New())}}}
	car := &fakeCarrier{} // no CreateReverseShipment
	e := NewExecutor(store, resolverFor(car), nil).WithReversePickup(true)

	out := e.CancelForOrder(context.Background(), oid)
	if out[0].Status != "unsupported" || out[0].Action != ActionReversePickup {
		t.Fatalf("out = %+v, want reverse_pickup/unsupported for a carrier without the capability", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/shipmentcancel/ -run 'TestExecutor_Delivered' -v`
Expected: FAIL — `WithReversePickup` / `fakeStore.CreateShipment` / `fakeReverseCarrier` wiring undefined, and reverse currently returns `unsupported` always.

- [ ] **Step 3: Implement**

In `internal/shipmentcancel/executor.go`:

Add `CreateShipment` to `ShipmentStore`:

```go
type ShipmentStore interface {
	ListShipmentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]shipping.ShipmentRecord, error)
	GetShipmentByID(ctx context.Context, id uuid.UUID) (*shipping.ShipmentRecord, error)
	SetShipmentCancelState(ctx context.Context, shipmentID uuid.UUID, action, status, reason string) error
	CreateShipment(ctx context.Context, rec *shipping.ShipmentRecord) error
}
```

Add the flag + setter to `Executor`:

```go
type Executor struct {
	store                ShipmentStore
	resolve              CarrierResolver
	logger               *slog.Logger
	reversePickupEnabled bool
}

// WithReversePickup enables the Phase 3 delivered-shipment reverse pickup. OFF
// by default: creating a reverse pickup dispatches a real courier to the
// customer and the Delhivery payload is not yet live-verified. Mirrors the
// REFUND_GATEWAY_ENABLED kill switch. When off, delivered shipments record
// `unsupported`.
func (e *Executor) WithReversePickup(enabled bool) *Executor {
	e.reversePickupEnabled = enabled
	return e
}
```

Replace the `ActionReversePickup` switch arm:

```go
	case ActionReversePickup:
		return e.execReversePickup(ctx, sh)
```

Add `execReversePickup` + the address helper:

```go
// execReversePickup creates a reverse (return) pickup for a delivered shipment
// and inserts a new reverse-leg row. Gated by the reversePickupEnabled kill
// switch. Carriers without the capability, or a disabled flag, record
// `unsupported` (the manual notice); a carrier rejection records `failed`.
func (e *Executor) execReversePickup(ctx context.Context, sh *shipping.ShipmentRecord) Outcome {
	if !e.reversePickupEnabled {
		reason := "Automatic reverse pickup is turned off — arrange the return manually with the carrier."
		e.record(ctx, sh.ID, ActionReversePickup, statusUnsupported, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusUnsupported, Reason: reason}
	}
	carrier, failReason := e.resolveCarrier(ctx, sh)
	if failReason != "" {
		e.record(ctx, sh.ID, ActionReversePickup, statusFailed, failReason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusFailed, Reason: failReason}
	}
	creator, ok := carrier.(shipping.ReverseShipmentCreator)
	if !ok {
		reason := "This carrier can't create a reverse pickup automatically — arrange the return manually with the carrier."
		e.record(ctx, sh.ID, ActionReversePickup, statusUnsupported, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusUnsupported, Reason: reason}
	}
	// Reverse the stored addresses: pickup FROM the customer (forward ship_to),
	// return TO the warehouse (forward ship_from).
	warehouse, err := parseShipmentAddress(sh.ShipFrom)
	if err != nil {
		reason := "Could not read the shipment's warehouse address — arrange the return manually."
		e.warn("shipmentcancel: parse ship_from failed", "shipment_id", sh.ID.String(), "err", err)
		e.record(ctx, sh.ID, ActionReversePickup, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusFailed, Reason: reason}
	}
	customer, err := parseShipmentAddress(sh.ShipTo)
	if err != nil {
		reason := "Could not read the shipment's delivery address — arrange the return manually."
		e.warn("shipmentcancel: parse ship_to failed", "shipment_id", sh.ID.String(), "err", err)
		e.record(ctx, sh.ID, ActionReversePickup, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusFailed, Reason: reason}
	}
	rev, err := creator.CreateReverseShipment(ctx, shipping.ReverseShipmentRequest{
		OrderID:       sh.OrderID.String(),
		PickupFrom:    customer,
		ReturnTo:      warehouse,
		WarehouseName: warehouse.Name,
		CurrencyCode:  sh.CurrencyCode,
	})
	if err != nil {
		reason := cleanReason(err)
		e.warn("shipmentcancel: reverse pickup failed", "shipment_id", sh.ID.String(), "carrier", sh.Carrier, "err", err)
		e.record(ctx, sh.ID, ActionReversePickup, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusFailed, Reason: reason}
	}
	// Insert the reverse leg as a new shipment row. Its addresses are swapped
	// (pickup from customer → return to warehouse). Marked cancel_action/status
	// so a repeat CancelForOrder skips it (it is itself a reverse action, not a
	// forward shipment to be cancelled). A DB failure here doesn't undo the
	// carrier pickup that already succeeded — log and still record success.
	revRec := &shipping.ShipmentRecord{
		TenantID:       sh.TenantID,
		StoreID:        sh.StoreID,
		OrderID:        sh.OrderID,
		Carrier:        sh.Carrier,
		TrackingNumber: rev.TrackingNumber,
		Status:         "pending",
		ShipFrom:       sh.ShipTo,   // reverse origin = customer
		ShipTo:         sh.ShipFrom, // reverse destination = warehouse
		CurrencyCode:   sh.CurrencyCode,
		CancelAction:   string(ActionReversePickup),
		CancelStatus:   statusSucceeded,
	}
	if err := e.store.CreateShipment(ctx, revRec); err != nil {
		e.warn("shipmentcancel: persist reverse-leg row failed", "shipment_id", sh.ID.String(), "reverse_waybill", rev.TrackingNumber, "err", err)
	}
	e.record(ctx, sh.ID, ActionReversePickup, statusSucceeded, "Reverse pickup created: "+rev.TrackingNumber)
	return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusSucceeded, Reason: "Reverse pickup created: " + rev.TrackingNumber}
}

// parseShipmentAddress decodes a shipments.ship_from/ship_to JSONB blob into a
// carrier Address. The keys match what handlers/admin/shipments.go writes on
// label create.
func parseShipmentAddress(raw []byte) (shipping.Address, error) {
	if len(raw) == 0 {
		return shipping.Address{}, fmt.Errorf("empty address")
	}
	var a struct {
		Name        string `json:"name"`
		Line1       string `json:"line1"`
		Line2       string `json:"line2"`
		City        string `json:"city"`
		Region      string `json:"region"`
		PostalCode  string `json:"postal_code"`
		CountryCode string `json:"country_code"`
		Phone       string `json:"phone"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return shipping.Address{}, err
	}
	return shipping.Address{
		Name: a.Name, Line1: a.Line1, Line2: a.Line2, City: a.City,
		Region: a.Region, PostalCode: a.PostalCode, CountryCode: a.CountryCode, Phone: a.Phone,
	}, nil
}
```

Add `"encoding/json"` to the executor imports.

- [ ] **Step 4: Run to verify it passes**

Run: `cd services/marketplace-api && go test ./internal/shipmentcancel/ -v`
Expected: PASS (all reverse tests + existing tests unchanged).

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/shipmentcancel/executor.go services/marketplace-api/internal/shipmentcancel/executor_test.go
git commit -m "feat(marketplace-api): reverse pickup for delivered shipments behind a kill switch"
```

---

### Task 3: Wire the flag + integration + full verify

**Files:**
- Modify: `cmd/marketplace-api/main.go`
- Modify: `internal/handlers/admin/shipment_cancel_integration_test.go`

- [ ] **Step 1: Wire `REVERSE_PICKUP_ENABLED` in main.go**

Where the executor is constructed (after `shipmentCanceller := shipmentcancel.NewExecutor(...)`), add the flag:

```go
		reversePickupEnabled := os.Getenv("REVERSE_PICKUP_ENABLED") == "true"
		shipmentCanceller = shipmentCanceller.WithReversePickup(reversePickupEnabled)
		log.Info("shipment-cancel executor wired", "reverse_pickup_enabled", reversePickupEnabled)
```

(`shipmentCanceller` was declared with `:=` above; reassign with `=`. `os` is already imported.)

- [ ] **Step 2: Add the integration test + carrier method**

In `internal/handlers/admin/shipment_cancel_integration_test.go`, add a `CreateReverseShipment` method to `recordingCarrier` (a `revCalls` counter, returns a fixed waybill), then:

```go
func (r *recordingCarrier) CreateReverseShipment(_ context.Context, in shipping.ReverseShipmentRequest) (*shipping.Shipment, error) {
	r.revCalls++
	return &shipping.Shipment{TrackingNumber: "REV-INT", ProviderShipmentID: "REV-INT", Carrier: "delhivery"}, nil
}
```
(add `revCalls int` to the struct.)

Update `coordinatorWithCanceller` to enable reverse pickup so the test exercises it:

```go
func coordinatorWithCancellerReverse(db *gorm.DB, car shipping.Carrier) *orderrefund.Coordinator {
	exec := shipmentcancel.NewExecutor(
		shipping.NewRepository(db),
		func(context.Context, uuid.UUID, string) (shipping.Carrier, error) { return car, nil },
		nil,
	).WithReversePickup(true)
	return newTestRefundCoordinator(db, &fakeGateway{}).
		WithShipmentCanceller(func(ctx context.Context, oid uuid.UUID) { exec.CancelForOrder(ctx, oid) })
}

func TestFullRefund_Delivered_CreatesReversePickup(t *testing.T) {
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
	seedShipmentWithStatus(t, env.db, tUUID, sUUID, oUUID, "WBN-DELIVERED", "delivered")

	car := &recordingCarrier{}
	coord := coordinatorWithCancellerReverse(env.db, car)

	if _, err := coord.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: oUUID, Amount: nil, Reason: "test", Actor: "test", ScopeID: "req-rev",
	}); err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if car.revCalls != 1 {
		t.Fatalf("CreateReverseShipment calls = %d, want 1", car.revCalls)
	}
	// forward shipment recorded reverse_pickup/succeeded.
	var fwd struct{ CancelAction, CancelStatus string }
	if err := env.db.Table("shipments").Select("cancel_action", "cancel_status").
		Where("order_id = ? AND tracking_number = ?", oUUID, "WBN-DELIVERED").Scan(&fwd).Error; err != nil {
		t.Fatalf("reload forward: %v", err)
	}
	if fwd.CancelAction != "reverse_pickup" || fwd.CancelStatus != "succeeded" {
		t.Fatalf("forward = %s/%s, want reverse_pickup/succeeded", fwd.CancelAction, fwd.CancelStatus)
	}
	// a new reverse-leg row exists.
	var revCount int64
	if err := env.db.Table("shipments").
		Where("order_id = ? AND tracking_number = ?", oUUID, "REV-INT").Count(&revCount).Error; err != nil {
		t.Fatalf("count reverse leg: %v", err)
	}
	if revCount != 1 {
		t.Fatalf("reverse-leg rows = %d, want 1", revCount)
	}
}
```

- [ ] **Step 3: Build + unit + vet**

Run:
```bash
cd services/marketplace-api && go build ./... && go test ./internal/shipmentcancel/... ./internal/shipping/... . && go vet ./internal/shipmentcancel/... ./internal/shipping/...
```
Expected: PASS, clean.

- [ ] **Step 4: Integration against ephemeral Postgres**

Spin Postgres (pgcrypto + uuid-ossp), migrate, then:
Run: `TEST_DATABASE_URL=… go test -tags integration ./internal/handlers/admin/ -run 'FullRefund|PartialRefund' -v`
Expected: PASS (Phase 1 + 2 + the new delivered → reverse test).

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go services/marketplace-api/internal/handlers/admin/shipment_cancel_integration_test.go
git commit -m "feat(marketplace-api): wire REVERSE_PICKUP_ENABLED flag + integration coverage"
```

---

## Self-Review (against the spec, Phase 3 row)

- **Delivered → reverse pickup:** `ActionReversePickup` → carrier `CreateReverseShipment` reusing order-creation with `payment_mode:"Pickup"` + return keys (Task 1/2). ✅
- **Auto-schedules:** no separate pickup request — Delhivery reverse auto-schedules (documented). ✅
- **New reverse shipment row; forward keeps delivered state:** Task 2 inserts the reverse leg; the forward row's `status` is untouched (only its cancel_* fields are set). ✅
- **Address mapping:** pickup-from = customer (`ship_to`), return-to = warehouse (`ship_from`). ✅
- **Carrier-agnostic, Delhivery first; others unsupported:** optional `ReverseShipmentCreator`. ✅
- **Best-effort:** unchanged; refund unaffected; DB-insert failure after a successful carrier pickup logs but still records success. ✅
- **Kill switch:** `REVERSE_PICKUP_ENABLED` default off, mirroring `REFUND_GATEWAY_ENABLED`. ✅
- **Re-run safety:** reverse-leg row marked `cancel_status=succeeded` so a repeat `CancelForOrder` skips it instead of cancelling the reverse waybill. ✅

## Not live-verified (flag OFF until confirmed)

The reverse-pickup payload (exact `return_country` format, per-entry weight, address roles) is **doc-derived**. `REVERSE_PICKUP_ENABLED` defaults **off** so nothing dispatches a live courier until the payload is verified against a real delivered shipment (create one reverse pickup manually with the merchant token, confirm it appears correctly on one.delhivery.com, then enable the flag).
