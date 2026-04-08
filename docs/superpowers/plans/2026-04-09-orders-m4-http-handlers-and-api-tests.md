# Orders M4 — admin HTTP handlers, DTOs, and API integration tests

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose every method from Orders M2 through a typed, authz-gated admin HTTP surface nested under `/api/v1/admin/stores/:storeId/`. Add the two DTO families (`AdminOrderListItem`, `AdminOrderResponse`, `AdminOrderEvent`, `AdminReturnResponse`, `AdminAbandonedCartListItem`) with strict projection hygiene, map typed service errors to the spec §6.6 error envelope, register routes into the existing marketplace-api Gin engine with `StoreMiddleware` + `fgaMw.Require(...)`, and prove the full lifecycle via HTTP integration tests. At M4 exit, a merchant admin user can drive the entire order → return → refund flow via curl against the running service.

**Architecture:** One handler struct per module (`OrdersHandler`, `ReturnsHandler`, `AbandonedCartsHandler`) that composes the corresponding service from M2. Handlers parse and validate request bodies via Gin's `ShouldBindJSON`, call the service, and map typed errors to the shared error envelope with `details` population. Routes are registered in a single `RegisterAdminRoutes(router *gin.RouterGroup, deps Deps)` function colocated with the handlers, called from `cmd/marketplace-api/main.go` only when `MODE=admin|both`. Each admin route runs the chain `GIPAuth → TenantMiddleware → StoreMiddleware → fgaMw.Require(...)` from the products middleware factory — M4 does not introduce new middleware.

**Tech Stack:** Gin v1.11, GORM v1.25, Postgres 15, OpenFGA, `github.com/shopspring/decimal`, `github.com/google/uuid`, `github.com/stretchr/testify`, `net/http/httptest`. No new external deps.

**Spec reference:** §6.1 (orders routes), §6.2 (returns routes), §6.3 (abandoned cart routes), §6.4 (DTO families), §6.6 (error envelope codes), §9 M4 exit criteria, §14 DoD items 8, 10.

**Out of scope for M4** (handled later):
- Storefront checkout HTTP path (internal API between storefront and marketplace-api) → M5
- Prometheus metrics wiring → M5
- Admin UI consumption of these endpoints → separate plan series

---

## Hard prerequisites

1. **Orders M1, M2, M3 all landed.** Service layer, state machine, outbox, and authz constants all exist on the branch.
2. **Products slice 1 HTTP scaffold landed.** Specifically, the middleware factory (`GIPAuth`, `TenantMiddleware`, `StoreMiddleware`, `fgaMw.Require`), the shared error envelope helper (`httpx.WriteError` or equivalent), and the admin route mounting pattern all exist in `marketplace-api`.
3. **Error envelope helper has a populated-details path.** M4 needs to put `allowed` arrays, `grand_total`/`already_refunded`/`requested` numbers, and `converted_order_id` into the `details` field. Task 0 verifies the helper supports this.

---

## Decisions locked for this milestone

1. **DTO families are distinct Go structs.** `AdminOrderListItem` (list projection), `AdminOrderResponse` (full detail) — matches spec §6.4. Nested types (`AdminOrderItem`, `AdminOrderAddress`, `AdminOrderEvent`, `AdminOrderCustomer`) live in the same file as their parent.
2. **Mappers are pure functions named `toAdminOrderResponse(o *order.Order, items []order.OrderItem, ...) AdminOrderResponse`.** No method receivers, no hidden DB calls — the handler prepares the aggregated data then calls the mapper.
3. **Money fields round-trip as JSON strings.** `decimal.Decimal` marshals to a JSON string by default. This preserves precision and matches what the admin UI expects.
4. **Route path style matches products.** `/api/v1/admin/stores/:storeId/orders` — `storeId` is a URL path parameter, not a header. The `StoreMiddleware` extracts it and validates it against the caller's tenant. Any `404` on a store mismatch returns `not_found`, not `forbidden`, to avoid existence leaks.
5. **List endpoint accepts query params: `tab`, `q`, `status`, `date_from`, `date_to`, `customer_id`, `page`, `per_page`.** `page` defaults to 1, `per_page` defaults to 25, max 100. The abandoned carts list endpoint is a separate route — `tab=abandoned` on the orders list returns a 400 `invalid_tab` so clients never accidentally hit the wrong path.
6. **Refund endpoint accepts an optional `Idempotency-Key` HTTP header.** If present, the key is passed to `service.RecordRefund` as the `idempotencyKey` argument. Replaying the same key is a no-op and returns 200 with the existing state. If the key is reused with a different amount or reason, the spec §6.6 `idempotency_conflict` error is NOT used in M4 — the service just returns the prior state regardless (the M2 implementation does not compare payloads). M4 documents this limitation in the handler comment; hardening is deferred.
7. **Error mapping is explicit in a `mapServiceError(err) (status int, code string, details map[string]any)` helper.** Adding new errors means adding a case; a missing case falls through to HTTP 500 with `internal_error`, which is loud in logs.
8. **Request validation uses Gin binding tags + a separate `validate()` method on the request struct.** Gin's binding handles `required`, `min`, `max`; the `validate()` method handles cross-field rules (e.g. "at least one item in a return request"). This two-step pattern keeps the error envelope populated without reinventing a validator framework.
9. **Response envelope matches products exactly.** On success: `200 {"data": {...}}`. On list: `200 {"data": [...], "meta": {"total": N, "page": P, "per_page": PP}}`. On error: spec §6.6 envelope with `error`, `message`, `details`.
10. **API integration tests use a test-scoped Gin engine**, not the production bootstrap. A small `newTestServer(t) *httptest.Server` helper constructs a fresh engine with the full middleware chain (including FGA) and returns a `*httptest.Server` whose handler is the Gin engine. The test helper seeds tenant membership tuples via the M3 harness before each test case.
11. **Storefront and admin handlers are in the same package.** Storefront handlers don't land until M5, but M4 creates `internal/order/http_admin.go` and leaves `http_storefront.go` as a TODO placeholder (empty file with a package comment) so M5's import lines are predictable.
12. **No handler reaches into the repository directly.** Handlers call services. Any temptation to "just grab the event list via the repo" should be resolved by adding a service method, not a repo call in the handler.

---

## File structure produced by M4

```
services/marketplace-api/
├── internal/
│   └── order/
│       ├── http_admin.go                 # NEW — RegisterAdminRoutes, handlers
│       ├── http_admin_test.go            # NEW — full HTTP integration tests
│       ├── http_storefront.go            # NEW — package-only stub for M5
│       ├── dto_admin.go                  # NEW — Admin* DTO types
│       ├── dto_mapper.go                 # NEW — pure mapping functions
│       ├── dto_mapper_test.go            # NEW — projection hygiene + precision tests
│       └── http_errors.go                # NEW — mapServiceError helper
└── cmd/
    └── marketplace-api/
        └── main.go                       # MODIFY — mount admin routes
```

No other files are created or modified.

---

## Task decomposition

### Task 0: Verify M1–M3 prerequisites + error envelope helper

- [ ] **Step 1: Build everything**

```bash
cd services/marketplace-api && go build -tags=testing ./...
```
Expected: exits 0.

- [ ] **Step 2: Verify M2 service exports + M3 authz constants**

```bash
for sym in \
  'order.NewService' \
  'order.NewReturnService' \
  'order.NewAbandonedCartService' \
  'authz.MarketplaceCanViewOrders' \
  'authz.MarketplaceCanEditOrders' \
  'authz.MarketplaceCanRefundOrders'; do
  # Quick sanity — each should appear in at least one source file
  pkg=$(echo "$sym" | cut -d. -f1)
  name=$(echo "$sym" | cut -d. -f2)
  grep -rq "func $name\\|var $name\\|$name\\s*=" services/marketplace-api/internal/$pkg/ || {
    echo "MISSING: $sym"; exit 1; }
done
echo "symbols OK"
```

- [ ] **Step 3: Verify products middleware factory exports**

```bash
grep -l 'func.*StoreMiddleware\|func.*Require(' services/marketplace-api/ -r | head
```
Expected: finds `pkg/middleware/` or `internal/middleware/` files with `StoreMiddleware` and `Require`. If missing, products slice 1 is incomplete — **STOP**.

- [ ] **Step 4: Verify error envelope helper supports `details`**

```bash
grep -A5 'func.*WriteError' services/marketplace-api/pkg/httpx/*.go 2>/dev/null || \
  grep -A5 'func.*WriteError' services/marketplace-api/internal/httpx/*.go 2>/dev/null
```
Expected: the signature includes a `details` parameter (either `map[string]any` or a typed struct). If not, file a follow-up on products — M4 can't emit populated error envelopes without it, and inlining the error envelope here would create drift.

- [ ] **Step 5: Verify FGA test harness from M3 is importable from other packages**

```bash
grep -l 'newAuthzHarness\|AuthzHarness' services/marketplace-api/internal/authz/*.go
```
Expected: at least one match. The test helpers are in `_test.go` files with the `_test` package, which M4's HTTP tests CANNOT import directly. The M4 HTTP test needs its own harness OR products needs to promote the helper to a non-`_test` test package. Decide here:

- **Option A:** M4 duplicates the minimal harness (~30 lines) in `http_admin_test.go`.
- **Option B:** Promote the products harness to `internal/authz/authztest/harness.go` (non-`_test` package with a clear name).

Task 0 does not write code — pick the option and mark the follow-up. Default recommendation: **Option A** (duplicate in M4) to avoid a cross-slice promotion during M4 execution.

No commit. Task 0 is verification only.

---

### Task 1: Admin DTO types

**Files:**
- Create: `services/marketplace-api/internal/order/dto_admin.go`

- [ ] **Step 1: Write the types**

```go
package order

import (
	"time"

	"github.com/shopspring/decimal"
)

// AdminOrderListItem is the thin projection returned by GET /orders.
// Deliberately omits notes, events, addresses — see dto_mapper.go.
type AdminOrderListItem struct {
	ID                string          `json:"id"`
	OrderNumber       string          `json:"order_number"`
	CustomerName      *string         `json:"customer_name,omitempty"`
	CustomerEmail     string          `json:"customer_email"`
	Status            string          `json:"status"`
	PaymentStatus     string          `json:"payment_status"`
	FulfillmentStatus string          `json:"fulfillment_status"`
	GrandTotal        decimal.Decimal `json:"grand_total"`
	RefundedAmount    decimal.Decimal `json:"refunded_amount"`
	CurrencyCode      string          `json:"currency_code"`
	ItemCount         int             `json:"item_count"`
	PlacedAt          time.Time       `json:"placed_at"`
	HasOpenReturn     bool            `json:"has_open_return"`
}

// AdminOrderResponse is the full detail projection returned by GET /orders/:id.
type AdminOrderResponse struct {
	ID                string                 `json:"id"`
	OrderNumber       string                 `json:"order_number"`
	Status            string                 `json:"status"`
	PaymentStatus     string                 `json:"payment_status"`
	FulfillmentStatus string                 `json:"fulfillment_status"`
	Customer          AdminOrderCustomer     `json:"customer"`
	Items             []AdminOrderItem       `json:"items"`
	Shipping          *AdminOrderAddress     `json:"shipping,omitempty"`
	Billing           *AdminOrderAddress     `json:"billing,omitempty"`
	Subtotal          decimal.Decimal        `json:"subtotal"`
	ShippingTotal     decimal.Decimal        `json:"shipping_total"`
	TaxTotal          decimal.Decimal        `json:"tax_total"`
	DiscountTotal     decimal.Decimal        `json:"discount_total"`
	GrandTotal        decimal.Decimal        `json:"grand_total"`
	RefundedAmount    decimal.Decimal        `json:"refunded_amount"`
	CurrencyCode      string                 `json:"currency_code"`
	PaymentProvider   *string                `json:"payment_provider,omitempty"`
	Notes             *string                `json:"notes,omitempty"`
	PlacedAt          time.Time              `json:"placed_at"`
	CancelledAt       *time.Time             `json:"cancelled_at,omitempty"`
	FulfilledAt       *time.Time             `json:"fulfilled_at,omitempty"`
	Events            []AdminOrderEvent      `json:"events"`
	Returns           []AdminReturnSummary   `json:"returns"`
}

type AdminOrderCustomer struct {
	ID    *string `json:"id,omitempty"`
	Email string  `json:"email"`
	Name  *string `json:"name,omitempty"`
}

type AdminOrderItem struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	SKU           string          `json:"sku"`
	OptionSummary *string         `json:"option_summary,omitempty"`
	UnitPrice     decimal.Decimal `json:"unit_price"`
	Quantity      int             `json:"quantity"`
	LineTotal     decimal.Decimal `json:"line_total"`
	ImageURL      *string         `json:"image_url,omitempty"`
	ProductID     *string         `json:"product_id,omitempty"`
	VariantID     *string         `json:"variant_id,omitempty"`
}

type AdminOrderAddress struct {
	Name        string  `json:"name"`
	Line1       string  `json:"line1"`
	Line2       *string `json:"line2,omitempty"`
	City        string  `json:"city"`
	Region      *string `json:"region,omitempty"`
	PostalCode  *string `json:"postal_code,omitempty"`
	CountryCode string  `json:"country_code"`
	Phone       *string `json:"phone,omitempty"`
}

type AdminOrderEvent struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	ActorEmail *string        `json:"actor_email,omitempty"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AdminReturnSummary struct {
	ID           string           `json:"id"`
	ReturnNumber string           `json:"return_number"`
	Status       string           `json:"status"`
	RefundAmount *decimal.Decimal `json:"refund_amount,omitempty"`
	RequestedAt  time.Time        `json:"requested_at"`
}

// AdminReturnResponse is returned by GET /returns/:id with the full return graph.
type AdminReturnResponse struct {
	ID           string               `json:"id"`
	ReturnNumber string               `json:"return_number"`
	OrderID      string               `json:"order_id"`
	OrderNumber  string               `json:"order_number"`
	Status       string               `json:"status"`
	Reason       *string              `json:"reason,omitempty"`
	Notes        *string              `json:"notes,omitempty"`
	RefundAmount *decimal.Decimal     `json:"refund_amount,omitempty"`
	CurrencyCode string               `json:"currency_code"`
	Items        []AdminReturnItemRow `json:"items"`
	RequestedAt  time.Time            `json:"requested_at"`
	ReceivedAt   *time.Time           `json:"received_at,omitempty"`
	RefundedAt   *time.Time           `json:"refunded_at,omitempty"`
}

type AdminReturnItemRow struct {
	ID            string  `json:"id"`
	OrderItemID   string  `json:"order_item_id"`
	TitleSnapshot string  `json:"title"`
	SKU           string  `json:"sku"`
	Quantity      int     `json:"quantity"`
	Reason        *string `json:"reason,omitempty"`
}

type AdminAbandonedCartListItem struct {
	ID             string          `json:"id"`
	CustomerEmail  *string         `json:"customer_email,omitempty"`
	CustomerName   *string         `json:"customer_name,omitempty"`
	ItemCount      int             `json:"item_count"`
	Subtotal       decimal.Decimal `json:"subtotal"`
	CurrencyCode   string          `json:"currency_code"`
	LastActiveAt   time.Time       `json:"last_active_at"`
	RecoverySentAt *time.Time      `json:"recovery_sent_at,omitempty"`
}

type AdminAbandonedCartResponse struct {
	AdminAbandonedCartListItem
	Items []AdminAbandonedCartItem `json:"items"`
}

type AdminAbandonedCartItem struct {
	Title         string          `json:"title"`
	SKU           string          `json:"sku"`
	OptionSummary *string         `json:"option_summary,omitempty"`
	UnitPrice     decimal.Decimal `json:"unit_price"`
	Quantity      int             `json:"quantity"`
	ImageURL      *string         `json:"image_url,omitempty"`
}
```

- [ ] **Step 2: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/dto_admin.go
git commit -m "feat(marketplace-api): admin DTO types for orders, returns, abandoned carts"
```

---

### Task 2: DTO mappers with projection hygiene tests

**Files:**
- Create: `services/marketplace-api/internal/order/dto_mapper.go`
- Create: `services/marketplace-api/internal/order/dto_mapper_test.go`

- [ ] **Step 1: Write pure mapping functions**

```go
package order

import (
	"encoding/json"
)

func toAdminOrderListItem(r ListRow) AdminOrderListItem {
	o := r.Order
	item := AdminOrderListItem{
		ID:                o.ID.String(),
		OrderNumber:       o.OrderNumber,
		CustomerName:      o.CustomerName,
		CustomerEmail:     o.CustomerEmail,
		Status:            o.Status,
		PaymentStatus:     o.PaymentStatus,
		FulfillmentStatus: o.FulfillmentStatus,
		GrandTotal:        o.GrandTotal,
		RefundedAmount:    o.RefundedAmount,
		CurrencyCode:      o.CurrencyCode,
		ItemCount:         r.ItemCount,
		PlacedAt:          o.PlacedAt,
		HasOpenReturn:     r.HasOpenReturn,
	}
	return item
}

func toAdminOrderResponse(
	o *Order,
	items []OrderItem,
	addrs []OrderAddress,
	events []OrderEvent,
	returns []Return,
) AdminOrderResponse {
	resp := AdminOrderResponse{
		ID:                o.ID.String(),
		OrderNumber:       o.OrderNumber,
		Status:            o.Status,
		PaymentStatus:     o.PaymentStatus,
		FulfillmentStatus: o.FulfillmentStatus,
		Customer: AdminOrderCustomer{
			Email: o.CustomerEmail,
			Name:  o.CustomerName,
		},
		Subtotal:       o.Subtotal,
		ShippingTotal:  o.ShippingTotal,
		TaxTotal:       o.TaxTotal,
		DiscountTotal:  o.DiscountTotal,
		GrandTotal:     o.GrandTotal,
		RefundedAmount: o.RefundedAmount,
		CurrencyCode:   o.CurrencyCode,
		PaymentProvider: o.PaymentProvider,
		Notes:          o.Notes,
		PlacedAt:       o.PlacedAt,
		CancelledAt:    o.CancelledAt,
		FulfilledAt:    o.FulfilledAt,
	}
	if o.CustomerID != nil {
		s := o.CustomerID.String()
		resp.Customer.ID = &s
	}
	for _, it := range items {
		resp.Items = append(resp.Items, AdminOrderItem{
			ID:            it.ID.String(),
			Title:         it.TitleSnapshot,
			SKU:           it.SKUSnapshot,
			OptionSummary: it.OptionSummary,
			UnitPrice:     it.UnitPrice,
			Quantity:      it.Quantity,
			LineTotal:     it.LineTotal,
			ImageURL:      it.ImageURL,
			ProductID:     stringPtr(it.ProductID),
			VariantID:     stringPtr(it.VariantID),
		})
	}
	for _, a := range addrs {
		addr := AdminOrderAddress{
			Name: a.Name, Line1: a.Line1, Line2: a.Line2,
			City: a.City, Region: a.Region, PostalCode: a.PostalCode,
			CountryCode: a.CountryCode, Phone: a.Phone,
		}
		switch a.Kind {
		case "shipping":
			resp.Shipping = &addr
		case "billing":
			resp.Billing = &addr
		}
	}
	for _, e := range events {
		var payload map[string]any
		_ = json.Unmarshal(e.Payload, &payload)
		resp.Events = append(resp.Events, AdminOrderEvent{
			ID:         e.ID.String(),
			Kind:       e.Kind,
			ActorEmail: e.ActorEmail,
			Payload:    payload,
			CreatedAt:  e.CreatedAt,
		})
	}
	for _, r := range returns {
		resp.Returns = append(resp.Returns, AdminReturnSummary{
			ID:           r.ID.String(),
			ReturnNumber: r.ReturnNumber,
			Status:       r.Status,
			RefundAmount: r.RefundAmount,
			RequestedAt:  r.RequestedAt,
		})
	}
	return resp
}

func toAdminReturnResponse(r *Return, order *Order, items []ReturnItem, orderItems []OrderItem) AdminReturnResponse {
	itemByID := map[uuid.UUID]OrderItem{}
	for _, oi := range orderItems {
		itemByID[oi.ID] = oi
	}
	resp := AdminReturnResponse{
		ID:           r.ID.String(),
		ReturnNumber: r.ReturnNumber,
		OrderID:      r.OrderID.String(),
		OrderNumber:  order.OrderNumber,
		Status:       r.Status,
		Reason:       r.Reason,
		Notes:        r.Notes,
		RefundAmount: r.RefundAmount,
		CurrencyCode: r.CurrencyCode,
		RequestedAt:  r.RequestedAt,
		ReceivedAt:   r.ReceivedAt,
		RefundedAt:   r.RefundedAt,
	}
	for _, ri := range items {
		oi := itemByID[ri.OrderItemID]
		resp.Items = append(resp.Items, AdminReturnItemRow{
			ID:            ri.ID.String(),
			OrderItemID:   ri.OrderItemID.String(),
			TitleSnapshot: oi.TitleSnapshot,
			SKU:           oi.SKUSnapshot,
			Quantity:      ri.Quantity,
			Reason:        ri.Reason,
		})
	}
	return resp
}

func toAdminAbandonedCartListItem(c AbandonedCart) AdminAbandonedCartListItem {
	return AdminAbandonedCartListItem{
		ID:             c.ID.String(),
		CustomerEmail:  c.CustomerEmail,
		CustomerName:   c.CustomerName,
		ItemCount:      c.ItemCount,
		Subtotal:       c.Subtotal,
		CurrencyCode:   c.CurrencyCode,
		LastActiveAt:   c.LastActiveAt,
		RecoverySentAt: c.RecoverySentAt,
	}
}

func toAdminAbandonedCartResponse(c *AbandonedCart) AdminAbandonedCartResponse {
	resp := AdminAbandonedCartResponse{
		AdminAbandonedCartListItem: toAdminAbandonedCartListItem(*c),
	}
	// Decode items_snapshot JSON into typed items
	var snapshot struct {
		Version int                      `json:"version"`
		Items   []AdminAbandonedCartItem `json:"items"`
	}
	if err := json.Unmarshal(c.ItemsSnapshot, &snapshot); err == nil {
		resp.Items = snapshot.Items
	}
	return resp
}

func stringPtr(u *uuid.UUID) *string {
	if u == nil {
		return nil
	}
	s := u.String()
	return &s
}
```

Add `uuid` import.

- [ ] **Step 2: Write projection hygiene tests**

```go
package order_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/order"
)

func TestAdminOrderListItem_MoneyRoundTripsAsString(t *testing.T) {
	item := order.AdminOrderListItem{
		ID:             uuid.New().String(),
		OrderNumber:    "M-TEST-260409-00001",
		CustomerEmail:  "buyer@example.com",
		Status:         "fulfilled",
		PaymentStatus:  "paid",
		GrandTotal:     decimal.RequireFromString("42.50"),
		RefundedAmount: decimal.Zero,
		CurrencyCode:   "EUR",
		PlacedAt:       time.Now(),
	}
	b, err := json.Marshal(item)
	require.NoError(t, err)
	var back order.AdminOrderListItem
	require.NoError(t, json.Unmarshal(b, &back))
	require.True(t, item.GrandTotal.Equal(back.GrandTotal))
	require.Contains(t, string(b), `"grand_total":"42.5"`) // decimal string serialization
}

func TestAdminOrderResponse_NoNotesLeakageInListProjection(t *testing.T) {
	// The list item struct must NOT contain a Notes field — it's a projection.
	// This test fails at compile time if someone adds Notes to AdminOrderListItem.
	item := order.AdminOrderListItem{}
	// Reflect over the struct via JSON to confirm no `notes` field
	b, _ := json.Marshal(item)
	require.NotContains(t, string(b), "notes", "AdminOrderListItem must not serialize a notes field")
}

func TestAdminOrderResponse_EventsAreIncluded(t *testing.T) {
	// Full detail MUST contain events (opposite of list projection)
	resp := order.AdminOrderResponse{Events: []order.AdminOrderEvent{{Kind: "status_changed"}}}
	b, _ := json.Marshal(resp)
	require.Contains(t, string(b), "events")
	require.Contains(t, string(b), "status_changed")
}

func TestAdminOrderResponse_TimestampsOmittedWhenNil(t *testing.T) {
	resp := order.AdminOrderResponse{ID: "abc"}
	b, _ := json.Marshal(resp)
	require.NotContains(t, string(b), "cancelled_at")
	require.NotContains(t, string(b), "fulfilled_at")
}
```

- [ ] **Step 3: Run**

```bash
cd services/marketplace-api && go test -run TestAdmin -v ./internal/order/
```

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/order/dto_mapper.go \
        services/marketplace-api/internal/order/dto_mapper_test.go
git commit -m "feat(marketplace-api): admin DTO mappers with projection hygiene tests"
```

---

### Task 3: Error envelope helper `mapServiceError`

**Files:**
- Create: `services/marketplace-api/internal/order/http_errors.go`

- [ ] **Step 1: Write the helper**

```go
package order

import (
	"errors"
	"net/http"
)

// mapServiceError converts a typed service error into an HTTP status code,
// spec §6.6 error envelope code, and a details map.
//
// Every new typed error from errors.go must be added here; unknown errors
// fall through to 500 `internal_error`.
func mapServiceError(err error) (int, string, map[string]any) {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "not_found", nil
	case errors.Is(err, ErrInvalidTransition):
		// The error message from transitionStatusTx encodes from/to/allowed;
		// extract via a helper or a typed error struct. For slice 1 we pass
		// the raw message in details and add structured data in a follow-up.
		return http.StatusConflict, "invalid_transition", map[string]any{
			"message": err.Error(),
		}
	case errors.Is(err, ErrRefundExceedsTotal):
		return http.StatusConflict, "refund_exceeds_total", nil
	case errors.Is(err, ErrIdempotencyConflict):
		return http.StatusConflict, "idempotency_conflict", nil
	case errors.Is(err, ErrOrderNotCancellable):
		return http.StatusConflict, "order_not_cancellable", nil
	case errors.Is(err, ErrReturnItemsExceedOrdered):
		return http.StatusConflict, "return_items_exceed_ordered", nil
	case errors.Is(err, ErrRecoveryTooRecent):
		return http.StatusConflict, "recovery_too_recent", nil
	case errors.Is(err, ErrAbandonedCartAlreadyConverted):
		return http.StatusConflict, "abandoned_cart_already_converted", nil
	default:
		return http.StatusInternalServerError, "internal_error", nil
	}
}
```

- [ ] **Step 2: Build + commit**

```bash
cd services/marketplace-api && go build ./internal/order/...
git add services/marketplace-api/internal/order/http_errors.go
git commit -m "feat(marketplace-api): mapServiceError helper for order envelope codes"
```

---

### Task 4: Orders admin handler — List + Detail

**Files:**
- Create: `services/marketplace-api/internal/order/http_admin.go`

- [ ] **Step 1: Write the handler struct, Deps, and `RegisterAdminRoutes`**

```go
package order

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	// ... products middleware imports
	"github.com/mark8ly/marketplace-api/internal/authz"
)

// Deps is the dependency bundle the main.go passes into RegisterAdminRoutes.
// Services are constructed once and reused across handlers.
type Deps struct {
	OrderSvc         *Service
	ReturnSvc        *ReturnService
	AbandonedCartSvc *AbandonedCartService
	FGAMiddleware    FGAMiddleware // interface from products middleware factory
}

// FGAMiddleware is the interface products exposes. Declared locally to keep
// this package from importing the full products middleware package.
type FGAMiddleware interface {
	Require(perm authz.Permission) gin.HandlerFunc
}

// RegisterAdminRoutes wires all admin order/return/abandoned-cart routes under
// the passed-in router group. The caller is responsible for attaching
// GIPAuth + TenantMiddleware + StoreMiddleware upstream.
func RegisterAdminRoutes(rg *gin.RouterGroup, d Deps) {
	h := newOrdersHandler(d)
	rh := newReturnsHandler(d)
	ah := newAbandonedCartsHandler(d)

	// Orders
	rg.GET("/orders", d.FGAMiddleware.Require(authz.MarketplaceCanViewOrders), h.List)
	rg.GET("/orders/:id", d.FGAMiddleware.Require(authz.MarketplaceCanViewOrders), h.Detail)
	rg.PATCH("/orders/:id/status", d.FGAMiddleware.Require(authz.MarketplaceCanEditOrders), h.SetStatus)
	rg.POST("/orders/:id/fulfill", d.FGAMiddleware.Require(authz.MarketplaceCanEditOrders), h.Fulfill)
	rg.POST("/orders/:id/cancel", d.FGAMiddleware.Require(authz.MarketplaceCanEditOrders), h.Cancel)
	rg.POST("/orders/:id/refund", d.FGAMiddleware.Require(authz.MarketplaceCanRefundOrders), h.Refund)
	rg.POST("/orders/:id/notes", d.FGAMiddleware.Require(authz.MarketplaceCanEditOrders), h.AddNote)
	rg.POST("/orders/:id/resend-confirmation", d.FGAMiddleware.Require(authz.MarketplaceCanEditOrders), h.ResendConfirmation)

	// Returns
	rg.GET("/returns", d.FGAMiddleware.Require(authz.MarketplaceCanViewReturns), rh.List)
	rg.GET("/returns/:id", d.FGAMiddleware.Require(authz.MarketplaceCanViewReturns), rh.Detail)
	rg.POST("/returns", d.FGAMiddleware.Require(authz.MarketplaceCanEditReturns), rh.Create)
	rg.PATCH("/returns/:id", d.FGAMiddleware.Require(authz.MarketplaceCanEditReturns), rh.Patch)

	// Abandoned carts
	rg.GET("/abandoned-carts", d.FGAMiddleware.Require(authz.MarketplaceCanViewAbandonedCarts), ah.List)
	rg.GET("/abandoned-carts/:id", d.FGAMiddleware.Require(authz.MarketplaceCanViewAbandonedCarts), ah.Detail)
	rg.POST("/abandoned-carts/:id/recovery-email", d.FGAMiddleware.Require(authz.MarketplaceCanEditAbandonedCarts), ah.TriggerRecoveryEmail)
}

type ordersHandler struct {
	deps Deps
}

func newOrdersHandler(d Deps) *ordersHandler { return &ordersHandler{deps: d} }

// List handles GET /api/v1/admin/stores/:storeId/orders
func (h *ordersHandler) List(c *gin.Context) {
	storeID, ok := storeIDFrom(c)
	if !ok {
		return
	}
	page, perPage := paginationFrom(c)
	tab := c.DefaultQuery("tab", "all")
	if tab == "abandoned" {
		writeError(c, http.StatusBadRequest, "invalid_tab",
			"Use /abandoned-carts endpoint for abandoned tab data", nil)
		return
	}

	var dateFrom, dateTo *time.Time
	if s := c.Query("date_from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			dateFrom = &t
		}
	}
	if s := c.Query("date_to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			dateTo = &t
		}
	}
	var customerID *uuid.UUID
	if s := c.Query("customer_id"); s != "" {
		if u, err := uuid.Parse(s); err == nil {
			customerID = &u
		}
	}

	rows, total, err := h.deps.OrderSvc.Repo().List(c.Request.Context(), ListFilter{
		StoreID:    storeID,
		Tab:        tab,
		Search:     c.Query("q"),
		Status:     splitCSV(c.Query("status")),
		DateFrom:   dateFrom,
		DateTo:     dateTo,
		CustomerID: customerID,
		Limit:      perPage,
		Offset:     (page - 1) * perPage,
	})
	if err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	items := make([]AdminOrderListItem, len(rows))
	for i, r := range rows {
		items[i] = toAdminOrderListItem(r)
	}
	c.JSON(http.StatusOK, gin.H{
		"data": items,
		"meta": gin.H{"total": total, "page": page, "per_page": perPage},
	})
}

// Detail handles GET /api/v1/admin/stores/:storeId/orders/:id
func (h *ordersHandler) Detail(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	o, err := h.deps.OrderSvc.Repo().GetByID(ctx, nil, id)
	if err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	items, _ := h.deps.OrderSvc.Repo().ListItems(ctx, nil, id)
	addrs, _ := h.deps.OrderSvc.Repo().ListAddresses(ctx, nil, id)
	events, _ := h.deps.OrderSvc.Repo().ListEvents(ctx, nil, id)
	returns, _ := h.deps.OrderSvc.Repo().ListReturns(ctx, nil, id)

	c.JSON(http.StatusOK, gin.H{
		"data": toAdminOrderResponse(o, items, addrs, events, returns),
	})
}

// --- shared helpers ---

func storeIDFrom(c *gin.Context) (uuid.UUID, bool) {
	raw := c.Param("storeId")
	u, err := uuid.Parse(raw)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_store_id", "storeId is not a UUID", nil)
		return uuid.Nil, false
	}
	return u, true
}

func uuidParam(c *gin.Context, name string) (uuid.UUID, bool) {
	raw := c.Param(name)
	u, err := uuid.Parse(raw)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_"+name, name+" is not a UUID", nil)
		return uuid.Nil, false
	}
	return u, true
}

func paginationFrom(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "25"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func writeError(c *gin.Context, status int, code, message string, details map[string]any) {
	c.AbortWithStatusJSON(status, gin.H{
		"error":   code,
		"message": message,
		"details": details,
	})
}
```

Note: this file references `h.deps.OrderSvc.Repo().ListItems`, `ListAddresses`, `ListEvents`, `ListReturns` — these are NEW methods to add to `repository.go`. Add them in the same step as simple GORM `Where(...).Find(&rows)` methods.

- [ ] **Step 2: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/http_admin.go \
        services/marketplace-api/internal/order/repository.go
git commit -m "feat(marketplace-api): orders admin handler List and Detail routes"
```

---

### Task 5: Orders handler — SetStatus, Fulfill, Cancel

**Files:**
- Modify: `services/marketplace-api/internal/order/http_admin.go`

- [ ] **Step 1: Add the three handlers**

```go
type setStatusReq struct {
	Target string `json:"target" binding:"required"`
	Reason string `json:"reason"`
}

func (h *ordersHandler) SetStatus(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req setStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "validation_failed", err.Error(), nil)
		return
	}

	// Slice 1 service layer exposes only explicit terminal transitions
	// (MarkFulfilled, Cancel). A generic PATCH /status is a thin router onto
	// those methods.
	actor := actorFrom(c)
	ctx := c.Request.Context()
	var o *Order
	var err error
	switch OrderStatus(req.Target) {
	case OrderStatusFulfilled:
		o, err = h.deps.OrderSvc.MarkFulfilled(ctx, id, nil, actor)
	case OrderStatusCancelled:
		o, err = h.deps.OrderSvc.Cancel(ctx, id, req.Reason, actor)
	default:
		writeError(c, http.StatusBadRequest, "invalid_transition",
			"target must be fulfilled or cancelled", map[string]any{"target": req.Target})
		return
	}
	if err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": o.ID, "status": o.Status}})
}

type fulfillReq struct {
	TrackingNumber string `json:"tracking_number"`
	Carrier        string `json:"carrier"`
}

func (h *ordersHandler) Fulfill(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req fulfillReq
	_ = c.ShouldBindJSON(&req) // body is optional
	var tracking *TrackingInfo
	if req.TrackingNumber != "" || req.Carrier != "" {
		tracking = &TrackingInfo{Number: req.TrackingNumber, Carrier: req.Carrier}
	}
	o, err := h.deps.OrderSvc.MarkFulfilled(c.Request.Context(), id, tracking, actorFrom(c))
	if err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": o.ID, "status": o.Status}})
}

type cancelReq struct {
	Reason string `json:"reason" binding:"required"`
}

func (h *ordersHandler) Cancel(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req cancelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "validation_failed", err.Error(), nil)
		return
	}
	o, err := h.deps.OrderSvc.Cancel(c.Request.Context(), id, req.Reason, actorFrom(c))
	if err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": o.ID, "status": o.Status}})
}

// actorFrom extracts the admin actor identity from the auth middleware context.
func actorFrom(c *gin.Context) Actor {
	actor := Actor{}
	if v, ok := c.Get("user_id"); ok {
		if s, ok := v.(string); ok {
			if u, err := uuid.Parse(s); err == nil {
				actor.ID = &u
			}
		}
	}
	if v, ok := c.Get("user_email"); ok {
		if s, ok := v.(string); ok {
			actor.Email = &s
		}
	}
	return actor
}
```

- [ ] **Step 2: Build + commit**

```bash
cd services/marketplace-api && go build ./internal/order/...
git add services/marketplace-api/internal/order/http_admin.go
git commit -m "feat(marketplace-api): orders SetStatus/Fulfill/Cancel handlers"
```

---

### Task 6: Orders handler — Refund with Idempotency-Key header

**Files:**
- Modify: `services/marketplace-api/internal/order/http_admin.go`

- [ ] **Step 1: Add the Refund handler**

```go
type refundReq struct {
	Amount decimal.Decimal `json:"amount" binding:"required"`
	Reason string          `json:"reason"`
}

// Refund handles POST /orders/:id/refund with optional Idempotency-Key header.
//
// Slice 1: bookkeeping only. This endpoint does NOT call any payment provider.
// The response body includes a `message` field with the canonical copy the UI
// uses, so integrators see the same caveat server-side.
func (h *ordersHandler) Refund(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req refundReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "validation_failed", err.Error(), nil)
		return
	}
	if req.Amount.IsNegative() || req.Amount.IsZero() {
		writeError(c, http.StatusBadRequest, "validation_failed", "amount must be positive", nil)
		return
	}

	idempotencyKey := c.GetHeader("Idempotency-Key")

	o, err := h.deps.OrderSvc.RecordRefund(c.Request.Context(), id,
		req.Amount, req.Reason, idempotencyKey, actorFrom(c))
	if err != nil {
		status, code, details := mapServiceError(err)
		// Populate refund-specific details where possible
		if code == "refund_exceeds_total" {
			existing, _ := h.deps.OrderSvc.Repo().GetByID(c.Request.Context(), nil, id)
			if existing != nil {
				if details == nil {
					details = map[string]any{}
				}
				details["grand_total"] = existing.GrandTotal
				details["already_refunded"] = existing.RefundedAmount
				details["requested"] = req.Amount
			}
		}
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"id":              o.ID,
			"refunded_amount": o.RefundedAmount,
			"payment_status":  o.PaymentStatus,
		},
		"message": "This records the refund in your dashboard. The customer's money has not been returned.",
	})
}
```

- [ ] **Step 2: Build + commit**

```bash
cd services/marketplace-api && go build ./internal/order/...
git add services/marketplace-api/internal/order/http_admin.go
git commit -m "feat(marketplace-api): orders refund handler with Idempotency-Key support"
```

---

### Task 7: Orders handler — AddNote, ResendConfirmation, stub storefront file

**Files:**
- Modify: `services/marketplace-api/internal/order/http_admin.go`
- Create: `services/marketplace-api/internal/order/http_storefront.go`

- [ ] **Step 1: Add AddNote + ResendConfirmation**

```go
type addNoteReq struct {
	Text string `json:"text" binding:"required"`
}

func (h *ordersHandler) AddNote(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req addNoteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "validation_failed", err.Error(), nil)
		return
	}
	// M2 does not expose an AddNote service method explicitly; it writes through
	// the order_events table directly via a new Service method. If missing,
	// add a small service.AddNote(ctx, orderID, text, actor) in this task.
	if err := h.deps.OrderSvc.AddNote(c.Request.Context(), id, req.Text, actorFrom(c)); err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id}})
}

func (h *ordersHandler) ResendConfirmation(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	if err := h.deps.OrderSvc.ResendConfirmation(c.Request.Context(), id, actorFrom(c)); err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id, "resent": true}})
}
```

If `OrderSvc.AddNote` does not exist yet, add it in `service.go` (a 10-line method that Unit-wraps an `order_events` insert with `kind=note_added`). Commit that change in the same task.

- [ ] **Step 2: Create the storefront stub**

`http_storefront.go`:
```go
package order

// Storefront HTTP handlers land in Orders M5 alongside the checkout integration.
// This file exists so M5 can add handlers without restructuring imports.
```

- [ ] **Step 3: Build + commit**

```bash
cd services/marketplace-api && go build ./internal/order/...
git add services/marketplace-api/internal/order/http_admin.go \
        services/marketplace-api/internal/order/http_storefront.go \
        services/marketplace-api/internal/order/service.go
git commit -m "feat(marketplace-api): orders AddNote/ResendConfirmation handlers + storefront stub"
```

---

### Task 8: Returns admin handler

**Files:**
- Modify: `services/marketplace-api/internal/order/http_admin.go`

- [ ] **Step 1: Write the returns handlers**

```go
type returnsHandler struct {
	deps Deps
}

func newReturnsHandler(d Deps) *returnsHandler { return &returnsHandler{deps: d} }

func (h *returnsHandler) List(c *gin.Context) {
	storeID, ok := storeIDFrom(c)
	if !ok {
		return
	}
	page, perPage := paginationFrom(c)
	// Add ReturnRepository.List(ctx, storeID, limit, offset) in return_repository.go
	rows, total, err := h.deps.ReturnSvc.List(c.Request.Context(), storeID, perPage, (page-1)*perPage)
	if err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": rows, // repo rows are already plain Return structs; mapper optional in slice 1
		"meta": gin.H{"total": total, "page": page, "per_page": perPage},
	})
}

func (h *returnsHandler) Detail(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	r, err := h.deps.ReturnSvc.Get(ctx, id)
	if err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	o, _ := h.deps.OrderSvc.Repo().GetByID(ctx, nil, r.OrderID)
	items, _ := h.deps.ReturnSvc.ListItems(ctx, id)
	orderItems, _ := h.deps.OrderSvc.Repo().ListItems(ctx, nil, r.OrderID)
	c.JSON(http.StatusOK, gin.H{"data": toAdminReturnResponse(r, o, items, orderItems)})
}

type createReturnReq struct {
	OrderID uuid.UUID `json:"order_id" binding:"required"`
	Items   []struct {
		OrderItemID uuid.UUID `json:"order_item_id" binding:"required"`
		Quantity    int       `json:"quantity" binding:"required,min=1"`
		Reason      *string   `json:"reason"`
	} `json:"items" binding:"required,dive"`
	Notes    *string `json:"notes"`
	Reason   *string `json:"reason"`
	Currency string  `json:"currency" binding:"required"`
}

func (h *returnsHandler) Create(c *gin.Context) {
	storeID, ok := storeIDFrom(c)
	if !ok {
		return
	}
	tenantID := tenantIDFrom(c) // helper in products middleware
	var req createReturnReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "validation_failed", err.Error(), nil)
		return
	}
	if len(req.Items) == 0 {
		writeError(c, http.StatusBadRequest, "validation_failed", "at least one item required", nil)
		return
	}

	// storePrefix is carried on the store model; a small helper fetches it from the DB
	storePrefix, err := lookupStorePrefix(c.Request.Context(), h.deps.OrderSvc, storeID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	items := make([]RequestItemInput, len(req.Items))
	for i, it := range req.Items {
		items[i] = RequestItemInput{OrderItemID: it.OrderItemID, Quantity: it.Quantity, Reason: it.Reason}
	}
	r, err := h.deps.ReturnSvc.Request(c.Request.Context(), RequestInput{
		TenantID: tenantID, StoreID: storeID, StorePrefix: storePrefix,
		OrderID: req.OrderID, Currency: req.Currency,
		Reason: req.Reason, Notes: req.Notes, Items: items,
	}, actorFrom(c))
	if err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{"id": r.ID, "return_number": r.ReturnNumber}})
}

type patchReturnReq struct {
	Status       string           `json:"status" binding:"required"`
	RefundAmount *decimal.Decimal `json:"refund_amount"`
	Notes        *string          `json:"notes"`
	Reason       *string          `json:"reason"`
}

func (h *returnsHandler) Patch(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req patchReturnReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "validation_failed", err.Error(), nil)
		return
	}
	var err error
	actor := actorFrom(c)
	ctx := c.Request.Context()
	switch ReturnStatus(req.Status) {
	case ReturnStatusApproved:
		_, err = h.deps.ReturnSvc.Approve(ctx, id, actor)
	case ReturnStatusReceived:
		_, err = h.deps.ReturnSvc.MarkReceived(ctx, id, actor)
	case ReturnStatusRefunded:
		if req.RefundAmount == nil {
			writeError(c, http.StatusBadRequest, "validation_failed", "refund_amount required for refunded status", nil)
			return
		}
		_, err = h.deps.ReturnSvc.MarkRefunded(ctx, id, *req.RefundAmount, actor)
	case ReturnStatusRejected:
		reason := ""
		if req.Reason != nil {
			reason = *req.Reason
		}
		_, err = h.deps.ReturnSvc.Reject(ctx, id, reason, actor)
	default:
		writeError(c, http.StatusBadRequest, "invalid_transition", "unknown target status", nil)
		return
	}
	if err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id, "status": req.Status}})
}
```

Add the small helper methods (`ReturnService.List`, `ReturnService.Get`, `ReturnService.ListItems`, `OrderService.Repo().ListItems`, `lookupStorePrefix`) that the handlers reference. Each is 5-10 lines.

- [ ] **Step 2: Build + commit**

```bash
cd services/marketplace-api && go build ./internal/order/...
git add services/marketplace-api/internal/order/http_admin.go \
        services/marketplace-api/internal/order/return_service.go \
        services/marketplace-api/internal/order/repository.go
git commit -m "feat(marketplace-api): returns admin handlers (list/detail/create/patch)"
```

---

### Task 9: Abandoned carts admin handler

**Files:**
- Modify: `services/marketplace-api/internal/order/http_admin.go`

- [ ] **Step 1: Write the handlers**

```go
type abandonedCartsHandler struct {
	deps Deps
}

func newAbandonedCartsHandler(d Deps) *abandonedCartsHandler {
	return &abandonedCartsHandler{deps: d}
}

func (h *abandonedCartsHandler) List(c *gin.Context) {
	storeID, ok := storeIDFrom(c)
	if !ok {
		return
	}
	page, perPage := paginationFrom(c)
	rows, total, err := h.deps.AbandonedCartSvc.List(c.Request.Context(), storeID, perPage, (page-1)*perPage)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	items := make([]AdminAbandonedCartListItem, len(rows))
	for i, r := range rows {
		items[i] = toAdminAbandonedCartListItem(r)
	}
	c.JSON(http.StatusOK, gin.H{
		"data": items,
		"meta": gin.H{"total": total, "page": page, "per_page": perPage},
	})
}

func (h *abandonedCartsHandler) Detail(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	r, err := h.deps.AbandonedCartSvc.Get(c.Request.Context(), id)
	if err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toAdminAbandonedCartResponse(r)})
}

func (h *abandonedCartsHandler) TriggerRecoveryEmail(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	if err := h.deps.AbandonedCartSvc.TriggerRecoveryEmail(c.Request.Context(), id, actorFrom(c)); err != nil {
		status, code, details := mapServiceError(err)
		writeError(c, status, code, err.Error(), details)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id, "sent": true}})
}
```

- [ ] **Step 2: Build + commit**

```bash
cd services/marketplace-api && go build ./internal/order/...
git add services/marketplace-api/internal/order/http_admin.go
git commit -m "feat(marketplace-api): abandoned carts admin handlers"
```

---

### Task 10: Wire admin routes into `cmd/marketplace-api/main.go`

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Mount the routes under the store-scoped group**

Locate the existing admin router group from products. Add:
```go
if cfg.Mode == "admin" || cfg.Mode == "both" {
    storeGroup := adminRouter.Group("/api/v1/admin/stores/:storeId",
        middleware.GIPAuth(),
        middleware.Tenant(),
        middleware.Store(db),
    )

    orderSvc := order.NewService(db)
    returnSvc := order.NewReturnService(db, orderSvc)
    abandonedSvc := order.NewAbandonedCartService(db, orderSvc)

    order.RegisterAdminRoutes(storeGroup, order.Deps{
        OrderSvc:         orderSvc,
        ReturnSvc:        returnSvc,
        AbandonedCartSvc: abandonedSvc,
        FGAMiddleware:    fgaMiddleware, // from products bootstrap
    })
}
```

- [ ] **Step 2: Build + boot smoke test**

```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/ && \
  MODE=admin ./marketplace-api &
sleep 2
curl -s -o /dev/null -w '%{http_code}\n' localhost:8087/api/v1/admin/stores/00000000-0000-0000-0000-000000000000/orders
kill %1
```
Expected: 401 (unauthenticated, because no GIP token). A 404 means the route isn't registered. A 500 means middleware is misconfigured.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): mount orders admin routes under store-scoped group"
```

---

### Task 11: HTTP integration tests — happy path lifecycle

**Files:**
- Create: `services/marketplace-api/internal/order/http_admin_test.go`

- [ ] **Step 1: Write the test server helper**

```go
package order_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
)

type apiHarness struct {
	t      *testing.T
	server *httptest.Server
	db     *gorm.DB
	tenant uuid.UUID
	store  uuid.UUID
	asUser uuid.UUID
	asRole string // "staff" | "admin" | "owner"
}

func newAPIHarness(t *testing.T) *apiHarness {
	t.Helper()
	db := testdb.New(t)
	tenant := uuid.New()
	store := uuid.New()
	// Seed store prefix row — requires a small stores_meta or similar helper
	seedStoreMeta(t, db, store, tenant, "TEST", "EUR")

	svc := order.NewService(db)
	retSvc := order.NewReturnService(db, svc)
	abSvc := order.NewAbandonedCartService(db, svc)

	r := gin.New()
	rg := r.Group("/api/v1/admin/stores/:storeId",
		fakeAuthMiddleware(store, tenant),
	)
	order.RegisterAdminRoutes(rg, order.Deps{
		OrderSvc:         svc,
		ReturnSvc:        retSvc,
		AbandonedCartSvc: abSvc,
		FGAMiddleware:    fakeFGAMiddleware(), // bypasses FGA for most tests
	})

	return &apiHarness{
		t:      t,
		server: httptest.NewServer(r),
		db:     db,
		tenant: tenant,
		store:  store,
		asRole: "admin",
	}
}

// do performs an authenticated request and returns the HTTP response + decoded body.
func (h *apiHarness) do(method, path string, body any, headers map[string]string) (*http.Response, map[string]any) {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.server.URL+path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Fake-User-Role", h.asRole) // consumed by fakeAuthMiddleware
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer res.Body.Close()
	var out map[string]any
	json.NewDecoder(res.Body).Decode(&out)
	return res, out
}
```

Define `fakeAuthMiddleware`, `fakeFGAMiddleware`, `seedStoreMeta` as test-only helpers in the same file. `fakeAuthMiddleware` sets `user_id`, `user_email`, `tenant_id`, `store_id` on the gin context. `fakeFGAMiddleware` either allows all (for most tests) or rejects specific permissions based on the `X-Fake-User-Role` header (for Task 12's auth matrix test).

- [ ] **Step 2: Write the happy path lifecycle test**

```go
func TestAPI_HappyPath_CreateFulfillRefund(t *testing.T) {
	h := newAPIHarness(t)
	defer h.server.Close()

	// Seed an order directly (checkout endpoint lands in M5)
	o := seedOrder(t, h.db, h.tenant, h.store, "EUR", 100)
	require.NoError(t, h.db.Model(o).Update("status", "confirmed").Error)

	// 1. GET /orders → sees our order
	res, body := h.do("GET", "/api/v1/admin/stores/"+h.store.String()+"/orders?tab=open", nil, nil)
	require.Equal(t, 200, res.StatusCode)
	meta := body["meta"].(map[string]any)
	require.EqualValues(t, 1, meta["total"])

	// 2. POST /orders/:id/fulfill
	res, _ = h.do("POST", "/api/v1/admin/stores/"+h.store.String()+"/orders/"+o.ID.String()+"/fulfill",
		map[string]string{"tracking_number": "DHL1Z", "carrier": "DHL"}, nil)
	require.Equal(t, 200, res.StatusCode)

	// 3. GET /orders/:id → fulfilled
	res, body = h.do("GET", "/api/v1/admin/stores/"+h.store.String()+"/orders/"+o.ID.String(), nil, nil)
	require.Equal(t, 200, res.StatusCode)
	data := body["data"].(map[string]any)
	require.Equal(t, "fulfilled", data["status"])

	// 4. POST /orders/:id/refund (as owner)
	h.asRole = "owner"
	res, body = h.do("POST", "/api/v1/admin/stores/"+h.store.String()+"/orders/"+o.ID.String()+"/refund",
		map[string]any{"amount": "30", "reason": "damage"},
		map[string]string{"Idempotency-Key": "key-1"})
	require.Equal(t, 200, res.StatusCode)
	data = body["data"].(map[string]any)
	require.Equal(t, "30", data["refunded_amount"])
	require.Equal(t, "partially_refunded", data["payment_status"])

	// 5. Replay same refund with same Idempotency-Key → still 200, refunded_amount unchanged
	res, body = h.do("POST", "/api/v1/admin/stores/"+h.store.String()+"/orders/"+o.ID.String()+"/refund",
		map[string]any{"amount": "30", "reason": "damage"},
		map[string]string{"Idempotency-Key": "key-1"})
	require.Equal(t, 200, res.StatusCode)
	data = body["data"].(map[string]any)
	require.Equal(t, "30", data["refunded_amount"]) // NOT 60
}
```

- [ ] **Step 3: Run + commit**

```bash
cd services/marketplace-api && go test -tags=testing -run TestAPI_HappyPath -v ./internal/order/
git add services/marketplace-api/internal/order/http_admin_test.go
git commit -m "test(marketplace-api): happy path HTTP integration test for orders lifecycle"
```

---

### Task 12: HTTP integration tests — auth matrix

**Files:**
- Modify: `services/marketplace-api/internal/order/http_admin_test.go`

- [ ] **Step 1: Write the matrix**

```go
func TestAPI_AuthMatrix(t *testing.T) {
	cases := []struct {
		role     string
		method   string
		path     string
		wantCode int
	}{
		{"staff", "GET", "/orders", 200},
		{"staff", "GET", "/orders/{id}", 200},
		{"staff", "POST", "/orders/{id}/fulfill", 403},
		{"staff", "POST", "/orders/{id}/cancel", 403},
		{"staff", "POST", "/orders/{id}/refund", 403},
		{"admin", "POST", "/orders/{id}/fulfill", 200},
		{"admin", "POST", "/orders/{id}/cancel", 200},
		{"admin", "POST", "/orders/{id}/refund", 403},
		{"owner", "POST", "/orders/{id}/refund", 200},
	}

	for _, tc := range cases {
		t.Run(tc.role+" "+tc.method+" "+tc.path, func(t *testing.T) {
			h := newAPIHarness(t)
			defer h.server.Close()
			h.asRole = tc.role
			o := seedOrder(t, h.db, h.tenant, h.store, "EUR", 100)
			// For cancel/fulfill tests, pre-transition to confirmed
			if strings.Contains(tc.path, "fulfill") || strings.Contains(tc.path, "refund") {
				h.db.Model(o).Update("status", "confirmed").Error
				if strings.Contains(tc.path, "refund") {
					h.db.Model(o).Update("status", "fulfilled").Error
				}
			}
			path := strings.ReplaceAll(tc.path, "{id}", o.ID.String())
			path = "/api/v1/admin/stores/" + h.store.String() + path
			var body any
			if strings.Contains(tc.path, "cancel") {
				body = map[string]string{"reason": "test"}
			}
			if strings.Contains(tc.path, "refund") {
				body = map[string]any{"amount": "10"}
			}
			res, _ := h.do(tc.method, path, body, nil)
			require.Equal(t, tc.wantCode, res.StatusCode)
		})
	}
}
```

`fakeFGAMiddleware` must consult `X-Fake-User-Role` and map it to the products-compatible relation check. Specifically: staff → `view` only; admin → `view` + `edit`; owner → all. Update the helper to implement this.

- [ ] **Step 2: Run + commit**

```bash
cd services/marketplace-api && go test -tags=testing -run TestAPI_AuthMatrix -v ./internal/order/
git add services/marketplace-api/internal/order/http_admin_test.go
git commit -m "test(marketplace-api): auth matrix for orders admin endpoints"
```

---

### Task 13: HTTP integration tests — illegal transitions + concurrent refund

**Files:**
- Modify: `services/marketplace-api/internal/order/http_admin_test.go`

- [ ] **Step 1: Illegal transition test**

```go
func TestAPI_CancelFulfilledOrder_Returns409(t *testing.T) {
	h := newAPIHarness(t)
	defer h.server.Close()
	o := seedOrder(t, h.db, h.tenant, h.store, "EUR", 100)
	h.db.Model(o).Update("status", "fulfilled").Error

	res, body := h.do("POST", "/api/v1/admin/stores/"+h.store.String()+"/orders/"+o.ID.String()+"/cancel",
		map[string]string{"reason": "nope"}, nil)
	require.Equal(t, 409, res.StatusCode)
	require.Equal(t, "order_not_cancellable", body["error"])
}

func TestAPI_IllegalTransition_IncludesAllowedInDetails(t *testing.T) {
	h := newAPIHarness(t)
	defer h.server.Close()
	o := seedOrder(t, h.db, h.tenant, h.store, "EUR", 100)
	h.db.Model(o).Update("status", "fulfilled").Error

	res, body := h.do("PATCH", "/api/v1/admin/stores/"+h.store.String()+"/orders/"+o.ID.String()+"/status",
		map[string]string{"target": "pending"}, nil)
	require.Equal(t, 409, res.StatusCode)
	// Message includes the illegal transition info
	require.Contains(t, body["message"], "fulfilled")
}
```

- [ ] **Step 2: Concurrent refund race test**

```go
func TestAPI_ConcurrentRefund_ExactlyOneWins(t *testing.T) {
	h := newAPIHarness(t)
	defer h.server.Close()
	h.asRole = "owner"
	o := seedOrder(t, h.db, h.tenant, h.store, "EUR", 100)
	h.db.Model(o).Update("status", "fulfilled").Error

	// Two concurrent refunds, each 60 — only one can succeed
	path := "/api/v1/admin/stores/" + h.store.String() + "/orders/" + o.ID.String() + "/refund"
	var wg sync.WaitGroup
	results := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			res, _ := h.do("POST", path, map[string]any{"amount": "60"}, nil)
			results[i] = res.StatusCode
		}()
	}
	wg.Wait()

	successes, conflicts := 0, 0
	for _, r := range results {
		switch r {
		case 200:
			successes++
		case 409:
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)

	// DB state: refunded_amount = 60 exactly
	var back order.Order
	h.db.First(&back, "id = ?", o.ID)
	require.Equal(t, "60", back.RefundedAmount.String())
}
```

- [ ] **Step 3: Run + commit**

```bash
cd services/marketplace-api && go test -tags=testing -run 'TestAPI_Cancel|TestAPI_Illegal|TestAPI_Concurrent' -v ./internal/order/
git add services/marketplace-api/internal/order/http_admin_test.go
git commit -m "test(marketplace-api): illegal transition and concurrent refund HTTP tests"
```

---

### Task 14: HTTP integration tests — cross-tenant isolation

**Files:**
- Modify: `services/marketplace-api/internal/order/http_admin_test.go`

- [ ] **Step 1: Test that tenant A cannot see tenant B's order**

```go
func TestAPI_CrossTenant_Returns404NotForbidden(t *testing.T) {
	h := newAPIHarness(t)
	defer h.server.Close()

	// Seed an order in a different tenant's store
	otherTenant := uuid.New()
	otherStore := uuid.New()
	seedStoreMeta(t, h.db, otherStore, otherTenant, "OTHR", "USD")
	foreign := seedOrder(t, h.db, otherTenant, otherStore, "USD", 50)

	// Try to read it via our store's path — should 404, not 403
	res, body := h.do("GET",
		"/api/v1/admin/stores/"+h.store.String()+"/orders/"+foreign.ID.String(), nil, nil)
	require.Equal(t, 404, res.StatusCode)
	require.Equal(t, "not_found", body["error"])
}
```

- [ ] **Step 2: Run + commit**

```bash
cd services/marketplace-api && go test -tags=testing -run TestAPI_CrossTenant -v ./internal/order/
git add services/marketplace-api/internal/order/http_admin_test.go
git commit -m "test(marketplace-api): cross-tenant isolation returns 404 not 403"
```

---

### Task 15: M4 exit checklist + handoff

**Files:**
- Modify: `services/marketplace-api/internal/order/README.md`

- [ ] **Step 1: Run the full marketplace-api test suite**

```bash
cd services/marketplace-api && go test -tags=testing ./...
```
Expected: all products + orders M1/M2/M3/M4 tests PASS.

- [ ] **Step 2: Manually curl the live API as a smoke test**

Start the service with a real DB and a fake auth middleware that injects a known user, then curl every endpoint in sequence. Document the exact curls in the README as reproducible examples.

- [ ] **Step 3: Tick the M4 exit criteria from spec §9 M4**

- [x] Full admin HTTP surface complete
- [x] StoreMiddleware enforces scoping (via products middleware)
- [x] Error envelope with new codes
- [x] API integration tests cover full lifecycle, illegal transitions, cross-tenant 404, authz 403, validation shapes
- [x] Idempotency-Key header support on refund endpoint
- [x] Concurrent refund race resolves to exactly-one-winner via HTTP

- [ ] **Step 4: Append "M4 handoff" to README and commit**

```bash
git add services/marketplace-api/internal/order/README.md
git commit -m "docs(marketplace-api): M4 handoff note for orders admin HTTP surface"
```

---

## Parallelization notes

Tasks 4–9 (the individual handler methods) can run in parallel once Tasks 1–3 (DTOs, mappers, error helper) have shipped. Dispatch up to three subagents for the orders / returns / abandoned cart handler groups. Task 10 (wire routes into main.go) depends on all three. Tasks 11–14 (integration tests) are strictly serial because they share a test file.

## Exit gate to M5

Do not start Orders M5 until:
1. Every task committed.
2. CI runs `go test -tags=testing ./...` for `services/marketplace-api` and it passes.
3. Manual curl smoke test against a real DB + FGA succeeds for every endpoint.
4. A human has reviewed the error envelope `details` shapes — M5's checkout handler and the future admin UI both depend on a stable contract.
