# Order Refund → Payment Gateway Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every order-refund path (admin refund, return refund, paid self-cancel) actually move money through the customer's original payment gateway, with provider idempotency and a retry sweeper so a refund is never lost and never doubled.

**Architecture:** A single `RefundCoordinator` (`internal/orderrefund/`) that all triggers call. It resolves the order's gateway from `payment_transactions.provider` + `payment_gateway_configs`, runs a ledger-first / idempotency-keyed saga (reserve pending `refund_transactions` row → call gateway → finalize row + `order.RecordRefund` atomically), and is backstopped by a standalone Cloud Run Job sweeper that re-runs stuck `pending` refunds with the same idempotency key.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL, golang-migrate, shopspring/decimal, existing `payment.Gateway` (Stripe/Razorpay/PayPal), robfig/cron (existing standalone-job pattern).

## Global Constraints

- Go 1.26; no stack changes; no new heavy dependencies (reuse existing gateways + repos).
- Service: `services/marketplace-api`. All paths below are relative to it.
- Immutable style: return new values, do not mutate shared structs.
- Error envelope: `{"error": code, "message": human}`; handlers use `RespondErr` / `apperrors`.
- Conventional commits, single-line messages, no signatures. Commit directly to `main`.
- **Prod service: no task is complete until its tests are written and green.** Target ≥80% coverage on new/changed packages. Run `go test -race` on touched packages.
- Feature flag `REFUND_GATEWAY_ENABLED` (env, default `false`) gates every real gateway call and the auto-cancel-refund. Off ⇒ current behavior.
- Migrations: golang-migrate, `migrations/NNNNNN_<name>.{up,down}.sql`, next number `000092`.
- Money never moves inside a DB transaction. Gateway call sits between two DB txns.

---

### Task 1: Migration — extend `refund_transactions` for the saga

**Files:**
- Create: `migrations/000092_refund_transactions_saga.up.sql`
- Create: `migrations/000092_refund_transactions_saga.down.sql`
- Modify: `internal/payment/repository.go` (RefundTransaction struct: add `OrderID`, keep `Status`, add `IdempotencyKey`)

**Interfaces:**
- Produces: `refund_transactions` columns `order_id uuid`, `status varchar(30)`, `idempotency_key varchar(255)` with `UNIQUE(idempotency_key)` and index `(status, created_at)`. Struct field `RefundTransaction.OrderID string`, `.IdempotencyKey string`.

- [ ] **Step 1: Write the up migration (defensive — nullable then tighten)**

`migrations/000092_refund_transactions_saga.up.sql`:
```sql
-- Extend refund_transactions to be the saga ledger (spec §5).
-- Columns added nullable first so the migration is safe even if rows exist,
-- then tightened. No real gateway refunds have moved money yet, so backfill
-- of order_id is best-effort (left NULL for any legacy rows).
ALTER TABLE refund_transactions
    ADD COLUMN IF NOT EXISTS order_id        uuid,
    ADD COLUMN IF NOT EXISTS idempotency_key varchar(255);

-- status already referenced by the webhook handler; ensure it exists + default.
ALTER TABLE refund_transactions
    ALTER COLUMN status SET DEFAULT 'pending';

-- Backfill legacy rows to a stable synthetic key so the UNIQUE index can be
-- created without collisions.
UPDATE refund_transactions
   SET idempotency_key = 'legacy:' || id::text
 WHERE idempotency_key IS NULL;

ALTER TABLE refund_transactions
    ALTER COLUMN idempotency_key SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_refund_transactions_idempotency_key
    ON refund_transactions (idempotency_key);

CREATE INDEX IF NOT EXISTS ix_refund_transactions_status_created
    ON refund_transactions (status, created_at);

CREATE INDEX IF NOT EXISTS ix_refund_transactions_order_id
    ON refund_transactions (order_id);
```

- [ ] **Step 2: Write the down migration**

`migrations/000092_refund_transactions_saga.down.sql`:
```sql
DROP INDEX IF EXISTS ix_refund_transactions_order_id;
DROP INDEX IF EXISTS ix_refund_transactions_status_created;
DROP INDEX IF EXISTS ux_refund_transactions_idempotency_key;
ALTER TABLE refund_transactions ALTER COLUMN status DROP DEFAULT;
ALTER TABLE refund_transactions
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS order_id;
```

- [ ] **Step 3: Update the GORM model**

In `internal/payment/repository.go`, add to `RefundTransaction` (after `StoreID`):
```go
	OrderID           string          `gorm:"column:order_id;type:uuid;index"                json:"order_id"`
	IdempotencyKey    string          `gorm:"column:idempotency_key;type:varchar(255);uniqueIndex" json:"idempotency_key"`
```
(`Status` already exists.)

- [ ] **Step 4: Run the migration up + down locally to verify**

Run: `make migrate-up && make migrate-down && make migrate-up` (see `Makefile`; falls back to `migrate -path migrations -database "$DATABASE_URL" up`)
Expected: no errors; `\d refund_transactions` shows the three columns + indexes.

- [ ] **Step 5: Commit**

```bash
git add migrations/000092_refund_transactions_saga.up.sql migrations/000092_refund_transactions_saga.down.sql internal/payment/repository.go
git commit -m "feat(refund): extend refund_transactions with order_id, status, idempotency_key"
```

---

### Task 2: Add `refund_unavailable` app error

**Files:**
- Modify: `pkg/apperrors/errors.go`
- Modify: `pkg/apperrors/codes.go` (or wherever `Code*` consts live — grep `CodeRefundExceedsTotal`)
- Test: `pkg/apperrors/errors_test.go`

**Interfaces:**
- Produces: `apperrors.ErrRefundUnavailable` (sentinel, `errors.Is`-comparable), `apperrors.RefundUnavailable(reason string) *Error`, HTTP 422.

- [ ] **Step 1: Write the failing test**

`pkg/apperrors/errors_test.go` (add):
```go
func TestRefundUnavailable(t *testing.T) {
	err := apperrors.RefundUnavailable("no captured payment")
	if !errors.Is(err, apperrors.ErrRefundUnavailable) {
		t.Fatalf("want ErrRefundUnavailable, got %v", err)
	}
	if got := apperrors.HTTPStatus(err); got != 422 {
		t.Fatalf("want 422, got %d", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/apperrors/ -run TestRefundUnavailable -v`
Expected: FAIL — `undefined: apperrors.RefundUnavailable`.

- [ ] **Step 3: Implement**

In `codes.go` add const near `CodeRefundExceedsTotal`:
```go
	CodeRefundUnavailable = "refund_unavailable"
```
In `errors.go` add sentinel near `ErrRefundExceedsTotal`:
```go
	ErrRefundUnavailable = &Error{Code: CodeRefundUnavailable}
```
Add constructor (mirror existing constructors like `ValidationFailed`):
```go
// RefundUnavailable is returned when an order cannot be refunded through the
// gateway (no captured payment transaction — COD/manual/authorized-only).
func RefundUnavailable(reason string) *Error {
	return &Error{Code: CodeRefundUnavailable, Message: reason}
}
```
Ensure the code→HTTP map returns 422 for `CodeRefundUnavailable` (mirror `CodeRefundExceedsTotal`'s mapping — grep it and add the same entry).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/apperrors/ -run TestRefundUnavailable -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/apperrors/
git commit -m "feat(refund): add refund_unavailable (422) app error"
```

---

### Task 3: Add `IdempotencyKey` to `RefundInput` + Stripe header

**Files:**
- Modify: `internal/payment/gateway.go` (RefundInput struct)
- Modify: `internal/payment/stripe.go:RefundPayment`
- Test: `internal/payment/stripe_test.go`

**Interfaces:**
- Produces: `payment.RefundInput.IdempotencyKey string`. Stripe sends `Idempotency-Key: <key>` header when non-empty.

- [ ] **Step 1: Add the field**

In `internal/payment/gateway.go`, `RefundInput`:
```go
type RefundInput struct {
	ProviderPaymentID string
	Amount            decimal.Decimal
	CurrencyCode      string
	Reason            string
	IdempotencyKey    string // provider idempotency key; retries with the same key never double-refund
}
```

- [ ] **Step 2: Write the failing test**

`internal/payment/stripe_test.go` (add):
```go
func TestStripeRefund_SendsIdempotencyKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"re_1","status":"succeeded","amount":5000}`))
	}))
	defer srv.Close()

	gw := payment.NewStripeGatewayWithBaseURL("sk_test", "", "test", srv.URL) // see Step 3
	_, err := gw.RefundPayment(context.Background(), payment.RefundInput{
		ProviderPaymentID: "pi_1", Amount: decimal.NewFromInt(50), CurrencyCode: "usd",
		IdempotencyKey: "refund:order-123:cancel",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if gotKey != "refund:order-123:cancel" {
		t.Fatalf("Idempotency-Key = %q, want the key", gotKey)
	}
}
```

- [ ] **Step 3: Run test to verify it fails, then implement**

Run: `go test ./internal/payment/ -run TestStripeRefund_SendsIdempotencyKey -v`
Expected: FAIL (`NewStripeGatewayWithBaseURL` undefined and/or header empty).

Implement:
1. If a base-URL test seam does not already exist, add one in `stripe.go` mirroring the existing constructor:
```go
// NewStripeGatewayWithBaseURL is the test seam for pointing at an httptest server.
func NewStripeGatewayWithBaseURL(apiKey, secretKey, mode, baseURL string) *StripeGateway {
	g := NewStripeGateway(apiKey, secretKey, mode)
	g.baseURL = baseURL
	return g
}
```
(If a seam already exists — grep `baseURL` in `stripe.go` — reuse it.)

2. In `StripeGateway.RefundPayment`, after building `req` and before `s.client.Do(req)`:
```go
	if in.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", in.IdempotencyKey)
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/payment/ -run TestStripeRefund_SendsIdempotencyKey -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/payment/gateway.go internal/payment/stripe.go internal/payment/stripe_test.go
git commit -m "feat(refund): Stripe refund sends Idempotency-Key header"
```

---

### Task 4: Razorpay refund idempotency + notes

**Files:**
- Modify: `internal/payment/razorpay.go:RefundPayment`
- Test: `internal/payment/razorpay_test.go`

**Interfaces:**
- Consumes: `RefundInput.IdempotencyKey` (Task 3).
- Produces: Razorpay refund sends header `X-Razorpay-Idempotency: <key>` and `notes[order_reason]=<reason>` (mirrors Home-Chef `RefundRequest.Notes`).

- [ ] **Step 1: Write the failing test**

`internal/payment/razorpay_test.go` (add):
```go
func TestRazorpayRefund_SendsIdempotencyHeaderAndNotes(t *testing.T) {
	var gotKey string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Razorpay-Idempotency")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"rfnd_1","status":"processed","amount":5000}`))
	}))
	defer srv.Close()

	gw := payment.NewRazorpayGatewayWithBaseURL("key", "secret", "test", srv.URL)
	_, err := gw.RefundPayment(context.Background(), payment.RefundInput{
		ProviderPaymentID: "pay_1", Amount: decimal.NewFromInt(50), CurrencyCode: "INR",
		Reason: "cancelled", IdempotencyKey: "refund:order-9:cancel",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if gotKey != "refund:order-9:cancel" {
		t.Fatalf("idempotency header = %q", gotKey)
	}
	if !strings.Contains(string(body), "cancelled") {
		t.Fatalf("reason not in notes: %s", body)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/payment/ -run TestRazorpayRefund_SendsIdempotencyHeaderAndNotes -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Add `NewRazorpayGatewayWithBaseURL` seam (mirror Task 3 Step 3 if not present). In `RefundPayment`, include notes in the JSON body and set the header before `Do`:
```go
	if in.IdempotencyKey != "" {
		req.Header.Set("X-Razorpay-Idempotency", in.IdempotencyKey)
	}
```
When constructing the refund request body, add:
```go
	"notes": map[string]string{"order_reason": in.Reason},
```
(match the existing body-building style in this method — it may use `url.Values` or JSON; keep it consistent).

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/payment/ -run TestRazorpayRefund_SendsIdempotencyHeaderAndNotes -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/payment/razorpay.go internal/payment/razorpay_test.go
git commit -m "feat(refund): Razorpay refund sends idempotency header + notes"
```

---

### Task 5: PayPal refund idempotency (`PayPal-Request-Id`)

**Files:**
- Modify: `internal/payment/paypal.go:RefundPayment`
- Test: `internal/payment/paypal_test.go`

**Interfaces:**
- Consumes: `RefundInput.IdempotencyKey`.
- Produces: PayPal refund sends header `PayPal-Request-Id: <key>`.

- [ ] **Step 1: Write the failing test**

`internal/payment/paypal_test.go` (add):
```go
func TestPayPalRefund_SendsRequestId(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth2/token") {
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600}`))
			return
		}
		gotKey = r.Header.Get("PayPal-Request-Id")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"RF-1","status":"COMPLETED"}`))
	}))
	defer srv.Close()

	gw := payment.NewPayPalGatewayWithBaseURL("id", "secret", "test", srv.URL)
	_, err := gw.RefundPayment(context.Background(), payment.RefundInput{
		ProviderPaymentID: "CAP-1", Amount: decimal.NewFromInt(50), CurrencyCode: "USD",
		IdempotencyKey: "refund:order-7:ret-2",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if gotKey != "refund:order-7:ret-2" {
		t.Fatalf("PayPal-Request-Id = %q", gotKey)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/payment/ -run TestPayPalRefund_SendsRequestId -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Add `NewPayPalGatewayWithBaseURL` seam (mirror; PayPal has both token + api base — set both to `baseURL` for the test). In `RefundPayment`, before `Do`:
```go
	if in.IdempotencyKey != "" {
		req.Header.Set("PayPal-Request-Id", in.IdempotencyKey)
	}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/payment/ -run TestPayPalRefund_SendsRequestId -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/payment/paypal.go internal/payment/paypal_test.go
git commit -m "feat(refund): PayPal refund sends PayPal-Request-Id header"
```

---

### Task 6: Payment saga primitives (reserve / execute / finalize)

**Files:**
- Modify: `internal/payment/repository.go` (tx-aware ledger methods)
- Modify: `internal/payment/service.go` (saga methods)
- Test: `internal/payment/service_test.go`

**Interfaces:**
- Consumes: `RefundInput.IdempotencyKey`, `Gateway.RefundPayment`.
- Produces:
  - `payment.Service.ReserveRefund(ctx, tx *gorm.DB, in ReserveRefundInput) (*RefundTransaction, bool, error)` — inserts a `pending` row `ON CONFLICT (idempotency_key) DO NOTHING`; bool `created`=false when the key already existed (returns the existing row).
  - `payment.Service.ExecuteGatewayRefund(ctx, gw Gateway, in RefundInput) (*Refund, error)` — pure gateway call, no DB.
  - `payment.Service.FinalizeRefund(ctx, tx *gorm.DB, ledgerID, providerRefundID, status string) error`.
  - `ReserveRefundInput{TenantID, StoreID, OrderID, Provider, ProviderPaymentID, Amount, CurrencyCode, Reason, IdempotencyKey}`.

- [ ] **Step 1: Add tx-aware repo methods**

In `internal/payment/repository.go`, add to the `Repository` interface and `gormRepository`:
```go
	// InsertRefundPending inserts a pending refund row inside tx. Returns
	// (row, created). created=false ⇒ a row with this idempotency_key already
	// existed (returned instead) — the saga re-entry guard.
	InsertRefundPending(tx *gorm.DB, r *RefundTransaction) (*RefundTransaction, bool, error)
	// UpdateRefundOutcome sets status + provider_refund_id on a ledger row.
	UpdateRefundOutcome(tx *gorm.DB, ledgerID, providerRefundID, status string) error
	// GetRefundByIdempotencyKey reads a ledger row by its unique key.
	GetRefundByIdempotencyKey(ctx context.Context, key string) (*RefundTransaction, error)
```
Implementation:
```go
func (r *gormRepository) InsertRefundPending(tx *gorm.DB, row *RefundTransaction) (*RefundTransaction, bool, error) {
	if row.Status == "" {
		row.Status = "pending"
	}
	res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(row)
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected == 1 {
		return row, true, nil
	}
	var existing RefundTransaction
	if err := tx.Where("idempotency_key = ?", row.IdempotencyKey).First(&existing).Error; err != nil {
		return nil, false, err
	}
	return &existing, false, nil
}

func (r *gormRepository) UpdateRefundOutcome(tx *gorm.DB, ledgerID, providerRefundID, status string) error {
	return tx.Model(&RefundTransaction{}).Where("id = ?", ledgerID).
		Updates(map[string]any{"provider_refund_id": providerRefundID, "status": status, "updated_at": gorm.Expr("now()")}).Error
}

func (r *gormRepository) GetRefundByIdempotencyKey(ctx context.Context, key string) (*RefundTransaction, error) {
	var row RefundTransaction
	if err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
```
Add `import "gorm.io/gorm/clause"`.

- [ ] **Step 2: Write the failing service test**

`internal/payment/service_test.go` (add; uses the package's existing fake `Repository` + fake `Gateway` — mirror existing tests in this file):
```go
func TestReserveRefund_Idempotent(t *testing.T) {
	// second reserve with same key returns created=false + same row
	// (exercised against the fake repo; see existing fakes in this file)
}

func TestExecuteGatewayRefund_PassesIdempotencyKey(t *testing.T) {
	fg := &fakeGateway{refund: &payment.Refund{ProviderRefundID: "re_9", Status: "succeeded"}}
	svc := payment.NewService(&fakeRepo{})
	_, err := svc.ExecuteGatewayRefund(context.Background(), fg, payment.RefundInput{
		ProviderPaymentID: "pi_1", Amount: decimal.NewFromInt(10), IdempotencyKey: "k1",
	})
	if err != nil { t.Fatal(err) }
	if fg.lastIn.IdempotencyKey != "k1" {
		t.Fatalf("gateway got key %q", fg.lastIn.IdempotencyKey)
	}
}
```
(Extend the file's existing `fakeGateway` to record `lastIn RefundInput`; if no fake exists yet, add one implementing `payment.Gateway` with the four other methods returning zero values.)

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/payment/ -run 'TestReserveRefund_Idempotent|TestExecuteGatewayRefund' -v`
Expected: FAIL — methods undefined.

- [ ] **Step 4: Implement the service methods**

In `internal/payment/service.go`:
```go
type ReserveRefundInput struct {
	TenantID, StoreID, OrderID  string
	Provider, ProviderPaymentID string
	Amount                      decimal.Decimal
	CurrencyCode, Reason        string
	IdempotencyKey              string
}

func (s *Service) ReserveRefund(ctx context.Context, tx *gorm.DB, in ReserveRefundInput) (*RefundTransaction, bool, error) {
	return s.repo.InsertRefundPending(tx, &RefundTransaction{
		TenantID: in.TenantID, StoreID: in.StoreID, OrderID: in.OrderID,
		Provider: in.Provider, ProviderPaymentID: in.ProviderPaymentID,
		Amount: in.Amount, Reason: in.Reason, Status: "pending",
		IdempotencyKey: in.IdempotencyKey,
	})
}

func (s *Service) ExecuteGatewayRefund(ctx context.Context, gw Gateway, in RefundInput) (*Refund, error) {
	r, err := gw.RefundPayment(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("payment service: gateway refund: %w", err)
	}
	return r, nil
}

func (s *Service) FinalizeRefund(ctx context.Context, tx *gorm.DB, ledgerID, providerRefundID, status string) error {
	return s.repo.UpdateRefundOutcome(tx, ledgerID, providerRefundID, status)
}
```
Add `import "gorm.io/gorm"` to service.go. Leave the legacy `RefundPayment` in place (now unused by handlers) or mark deprecated.

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/payment/ -race -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/payment/repository.go internal/payment/service.go internal/payment/service_test.go
git commit -m "feat(refund): payment saga primitives (reserve/execute/finalize)"
```

---

### Task 7: Gateway + payment-txn resolver for orders

**Files:**
- Create: `internal/orderrefund/resolver.go`
- Test: `internal/orderrefund/resolver_test.go`

**Interfaces:**
- Produces:
  - `PaymentContext{Provider, ProviderPaymentID string; CapturedTotal decimal.Decimal; Found bool}`.
  - `Resolver.PaymentContextForOrder(ctx, orderID uuid.UUID) (PaymentContext, error)` — reads `payment_transactions`; `Found=false` when no captured txn.
  - `Resolver.GatewayFor(ctx, storeID uuid.UUID, provider string) (payment.Gateway, error)` — reads `payment_gateway_configs`, calls `payment.NewGateway`.

- [ ] **Step 1: Write the failing test**

`internal/orderrefund/resolver_test.go` (integration-style against a test DB helper — reuse the pattern in `internal/order/testhelpers_integration_test.go`):
```go
//go:build integration
func TestPaymentContextForOrder_CapturedTotal(t *testing.T) {
	db := testDB(t) // shared helper
	orderID := uuid.New()
	seedPaymentTxn(t, db, orderID, "stripe", "pi_1", "100.00", "captured")
	r := orderrefund.NewResolver(db)
	pc, err := r.PaymentContextForOrder(context.Background(), orderID)
	if err != nil { t.Fatal(err) }
	if !pc.Found || pc.Provider != "stripe" || !pc.CapturedTotal.Equal(decimal.RequireFromString("100.00")) {
		t.Fatalf("unexpected: %+v", pc)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -tags integration ./internal/orderrefund/ -run TestPaymentContextForOrder_CapturedTotal -v`
Expected: FAIL — package/functions undefined.

- [ ] **Step 3: Implement**

`internal/orderrefund/resolver.go`:
```go
package orderrefund

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/payment"
)

type PaymentContext struct {
	Provider          string
	ProviderPaymentID string
	CapturedTotal     decimal.Decimal
	Found             bool
}

type Resolver struct{ db *gorm.DB }

func NewResolver(db *gorm.DB) *Resolver { return &Resolver{db: db} }

// PaymentContextForOrder returns the captured payment for an order. Only rows
// in a captured/paid state count toward CapturedTotal so authorized-only
// orders are treated as non-refundable (Found=false).
func (r *Resolver) PaymentContextForOrder(ctx context.Context, orderID uuid.UUID) (PaymentContext, error) {
	type row struct {
		Provider          string
		ProviderIntentID  string
		Amount            decimal.Decimal
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("payment_transactions").
		Select("provider", "provider_intent_id", "amount").
		Where("order_id = ? AND status IN ('captured','paid','succeeded')", orderID).
		Order("created_at ASC").
		Scan(&rows).Error; err != nil {
		return PaymentContext{}, err
	}
	if len(rows) == 0 {
		return PaymentContext{Found: false}, nil
	}
	total := decimal.Zero
	for _, x := range rows {
		total = total.Add(x.Amount)
	}
	return PaymentContext{
		Provider:          rows[0].Provider,
		ProviderPaymentID: rows[0].ProviderIntentID,
		CapturedTotal:     total,
		Found:             true,
	}, nil
}

type gatewayConfigRow struct {
	Provider  string `gorm:"column:provider"`
	APIKey    string `gorm:"column:api_key_encrypted"`
	SecretKey string `gorm:"column:secret_key_encrypted"`
	Mode      string `gorm:"column:mode"`
}

func (gatewayConfigRow) TableName() string { return "payment_gateway_configs" }

func (r *Resolver) GatewayFor(ctx context.Context, storeID uuid.UUID, provider string) (payment.Gateway, error) {
	var cfg gatewayConfigRow
	if err := r.db.WithContext(ctx).
		Where("store_id = ? AND provider = ? AND is_active = true", storeID, provider).
		First(&cfg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("orderrefund: no active %q gateway config for store %s", provider, storeID)
		}
		return nil, err
	}
	return payment.NewGateway(provider, cfg.APIKey, cfg.SecretKey, cfg.Mode)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test -tags integration ./internal/orderrefund/ -run TestPaymentContextForOrder_CapturedTotal -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/orderrefund/resolver.go internal/orderrefund/resolver_test.go
git commit -m "feat(refund): order payment-context + gateway resolver"
```

---

### Task 8: RefundCoordinator core (amount/status derivation + saga)

**Files:**
- Create: `internal/orderrefund/coordinator.go`
- Test: `internal/orderrefund/coordinator_test.go`

**Interfaces:**
- Consumes: `Resolver` (Task 7), `payment.Service` saga methods (Task 6), `order.Service.RecordRefund` (existing), `order.Repository.GetByID` (existing).
- Produces:
  - `RefundCommand{OrderID uuid.UUID; Amount *decimal.Decimal; Reason string; Actor string; ScopeID string}` — `Amount==nil` ⇒ full remaining; `ScopeID` sets the idempotency scope (`return_id`, `"cancel:"+orderID`, or a refund-request UUID).
  - `RefundResult{ProviderRefundID string; Amount decimal.Decimal; PaymentStatus order.PaymentStatus; AlreadyDone bool}`.
  - `Coordinator.Refund(ctx, cmd RefundCommand) (RefundResult, error)` — returns `apperrors.ErrRefundUnavailable` when no captured txn; `apperrors.ErrRefundExceedsTotal` when over cap.
  - `NewCoordinator(db *gorm.DB, res *Resolver, pay *payment.Service, orders *order.Service, orderRepo order.Repository, enabled bool) *Coordinator`.

- [ ] **Step 1: Write the failing unit tests (fakes, no DB)**

`internal/orderrefund/coordinator_test.go`:
```go
func TestDeriveStatus(t *testing.T) {
	gt := decimal.RequireFromString("120.00")
	cases := []struct{ refunded, amount string; want order.PaymentStatus }{
		{"0", "50.00", order.PaymentStatusPartiallyRefunded},
		{"0", "120.00", order.PaymentStatusRefunded},
		{"60.00", "60.00", order.PaymentStatusRefunded},
		{"60.00", "30.00", order.PaymentStatusPartiallyRefunded},
	}
	for _, c := range cases {
		got := orderrefund.DeriveStatus(decimal.RequireFromString(c.refunded), decimal.RequireFromString(c.amount), gt)
		if got != c.want {
			t.Fatalf("refunded=%s amount=%s => %s, want %s", c.refunded, c.amount, got, c.want)
		}
	}
}
```
Plus (with fake resolver/payment/order via small interfaces — see Step 3 for the interface seams):
- `TestRefund_NoCapturedTxn_Unavailable` → `errors.Is(err, apperrors.ErrRefundUnavailable)`, gateway never called.
- `TestRefund_OverCap_Rejected` → `errors.Is(err, apperrors.ErrRefundExceedsTotal)`, gateway never called.
- `TestRefund_FullNilAmount_RefundsRemaining` → amount == grand_total − refunded.
- `TestRefund_GatewayError_LeavesPending` → coordinator returns error, no `RecordRefund` call.
- `TestRefund_Disabled_SkipsGateway` → `enabled=false` ⇒ returns a clear error (does NOT silently bookkeep).
- `TestRefund_IdempotencyKeyStable` → same `(OrderID, ScopeID)` ⇒ identical key string.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/orderrefund/ -run TestRefund -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

`internal/orderrefund/coordinator.go`:
```go
package orderrefund

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

type RefundCommand struct {
	OrderID uuid.UUID
	Amount  *decimal.Decimal // nil ⇒ full remaining
	Reason  string
	Actor   string
	ScopeID string // idempotency scope: return_id | "cancel:"+orderID | refund_request_id
}

type RefundResult struct {
	ProviderRefundID string
	Amount           decimal.Decimal
	PaymentStatus    order.PaymentStatus
	AlreadyDone      bool
}

type Coordinator struct {
	db        *gorm.DB
	res       *Resolver
	pay       *payment.Service
	orders    *order.Service
	orderRepo order.Repository
	enabled   bool
}

func NewCoordinator(db *gorm.DB, res *Resolver, pay *payment.Service, orders *order.Service, orderRepo order.Repository, enabled bool) *Coordinator {
	return &Coordinator{db: db, res: res, pay: pay, orders: orders, orderRepo: orderRepo, enabled: enabled}
}

// DeriveStatus picks partially_refunded vs refunded from the post-refund total.
func DeriveStatus(refunded, amount, grandTotal decimal.Decimal) order.PaymentStatus {
	if refunded.Add(amount).GreaterThanOrEqual(grandTotal) {
		return order.PaymentStatusRefunded
	}
	return order.PaymentStatusPartiallyRefunded
}

func idempotencyKey(orderID uuid.UUID, scopeID string) string {
	return "refund:" + orderID.String() + ":" + scopeID
}

func (c *Coordinator) Refund(ctx context.Context, cmd RefundCommand) (RefundResult, error) {
	if !c.enabled {
		return RefundResult{}, fmt.Errorf("orderrefund: gateway refunds disabled (REFUND_GATEWAY_ENABLED=false)")
	}
	o, _, _, err := c.orderRepo.GetByID(ctx, c.db, cmd.OrderID)
	if err != nil || o == nil {
		return RefundResult{}, apperrors.NotFound("order")
	}
	pc, err := c.res.PaymentContextForOrder(ctx, cmd.OrderID)
	if err != nil {
		return RefundResult{}, err
	}
	if !pc.Found {
		return RefundResult{}, apperrors.RefundUnavailable("no captured payment for this order")
	}

	remaining := o.GrandTotal.Sub(o.RefundedAmount)
	amount := remaining
	if cmd.Amount != nil {
		amount = *cmd.Amount
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return RefundResult{}, apperrors.ValidationFailed("amount", "refund amount must be positive")
	}
	cap := decimal.Min(o.GrandTotal, pc.CapturedTotal)
	if o.RefundedAmount.Add(amount).GreaterThan(cap) {
		return RefundResult{}, apperrors.ErrRefundExceedsTotal
	}
	target := DeriveStatus(o.RefundedAmount, amount, o.GrandTotal)
	key := idempotencyKey(cmd.OrderID, cmd.ScopeID)

	gw, err := c.res.GatewayFor(ctx, o.StoreID, pc.Provider)
	if err != nil {
		return RefundResult{}, err
	}

	// tx #1 — reserve pending ledger row (idempotent).
	var ledger *payment.RefundTransaction
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, _, rErr := c.pay.ReserveRefund(ctx, tx, payment.ReserveRefundInput{
			TenantID: o.TenantID.String(), StoreID: o.StoreID.String(), OrderID: cmd.OrderID.String(),
			Provider: pc.Provider, ProviderPaymentID: pc.ProviderPaymentID,
			Amount: amount, CurrencyCode: o.CurrencyCode, Reason: cmd.Reason, IdempotencyKey: key,
		})
		ledger = row
		return rErr
	}); err != nil {
		return RefundResult{}, err
	}
	if ledger.Status == "succeeded" {
		return RefundResult{ProviderRefundID: ledger.ProviderRefundID, Amount: amount, PaymentStatus: target, AlreadyDone: true}, nil
	}

	// gateway call — real money, retry-safe via key.
	ref, err := c.pay.ExecuteGatewayRefund(ctx, gw, payment.RefundInput{
		ProviderPaymentID: pc.ProviderPaymentID, Amount: amount,
		CurrencyCode: o.CurrencyCode, Reason: cmd.Reason, IdempotencyKey: key,
	})
	if err != nil {
		return RefundResult{}, err // ledger stays pending → sweeper retries
	}

	// tx #2 — finalize ledger + bookkeeping, atomic.
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.pay.FinalizeRefund(ctx, tx, ledger.ID, ref.ProviderRefundID, "succeeded"); err != nil {
			return err
		}
		return c.orders.RecordRefund(ctx, tx, cmd.OrderID, amount, target, cmd.Reason)
	}); err != nil {
		return RefundResult{}, err
	}
	return RefundResult{ProviderRefundID: ref.ProviderRefundID, Amount: amount, PaymentStatus: target}, nil
}
```
Note the unit tests in Step 1 need small interface seams. Refactor `res`, `pay`, `orders` dependencies behind narrow interfaces defined in this package (e.g. `paymentContexter`, `gatewayResolver`, `refundLedger`, `bookkeeper`) so the fakes can be injected. Define those interfaces and have `NewCoordinator` accept them; the concrete `*Resolver`/`*payment.Service`/`*order.Service` satisfy them. (This keeps `DeriveStatus`, cap, and unavailable logic unit-testable without a DB.)

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/orderrefund/ -race -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/orderrefund/coordinator.go internal/orderrefund/coordinator_test.go
git commit -m "feat(refund): RefundCoordinator core saga + amount/status derivation"
```

---

### Task 9: Wire admin order refund handler → coordinator

**Files:**
- Modify: `internal/handlers/admin/orders.go:Refund` (~line 467)
- Modify: `internal/handlers/admin/orders_dto.go` (RefundOrderRequest: add `RefundRequestID`, deprecate `PaymentStatus`)
- Modify: `internal/handlers/admin/routes.go` (inject coordinator into OrdersHandler)
- Test: `internal/handlers/admin/orders_refund_integration_test.go`

**Interfaces:**
- Consumes: `orderrefund.Coordinator.Refund`.
- Produces: admin `POST /admin/stores/:storeId/orders/:id/refund` now moves money; response includes `provider_refund_id` + derived `payment_status`.

- [ ] **Step 1: Update the request DTO**

In `orders_dto.go`:
```go
type RefundOrderRequest struct {
	Amount          *decimal.Decimal `json:"amount"`                       // omit ⇒ full remaining
	RefundRequestID string           `json:"refund_request_id,omitempty"`  // idempotency scope; server generates if empty
	Reason          string           `json:"reason"`
	// Deprecated: server derives partial/full from amount. Ignored.
	PaymentStatus string `json:"payment_status,omitempty"`
}
```

- [ ] **Step 2: Write the failing integration test**

`internal/handlers/admin/orders_refund_integration_test.go` (build tag `integration`; reuse the admin test harness — grep an existing `*_integration_test.go` in this package for setup). Seed a paid order with a captured stripe `payment_transactions` row and an active `payment_gateway_configs` row; point the gateway at an httptest Stripe stub. Assert:
```go
// POST refund amount=50 on a 120 order
// → 200, body.provider_refund_id != "", body.payment_status == "partially_refunded"
// → refund_transactions row status='succeeded', order_id set, unique key
// → orders.refunded_amount == 50.00
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test -tags integration ./internal/handlers/admin/ -run Refund -v`
Expected: FAIL.

- [ ] **Step 4: Implement the handler change**

Replace the body of `Refund` that calls `h.svc.RecordRefund(...)` with:
```go
	scope := req.RefundRequestID
	if scope == "" {
		scope = uuid.NewString()
	}
	res, rerr := h.refunds.Refund(c.Request.Context(), orderrefund.RefundCommand{
		OrderID: id, Amount: req.Amount, Reason: req.Reason,
		Actor: "user:" + c.GetString("user_id"), ScopeID: scope,
	})
	if rerr != nil {
		RespondErr(c, rerr, h.logger) // maps ErrRefundUnavailable→422, ErrRefundExceedsTotal→422, ErrInvalidTransition→409
		return
	}
```
Keep the existing audit emit + `dispatchRefundEmail`, but source the amount/total from `res` and a fresh `GetByID`. Add `h.refunds *orderrefund.Coordinator` to `OrdersHandler` + its constructor; wire it in `routes.go` and `main.go` (deps struct).

- [ ] **Step 5: Run to verify it passes**

Run: `go test -tags integration ./internal/handlers/admin/ -run Refund -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/admin/orders.go internal/handlers/admin/orders_dto.go internal/handlers/admin/routes.go internal/handlers/admin/orders_refund_integration_test.go
git commit -m "feat(refund): admin order refund moves money via coordinator"
```

---

### Task 10: Wire return refund → coordinator

**Files:**
- Modify: `internal/order/return_service.go:MarkRefunded`
- Modify: `internal/handlers/admin/returns.go:MarkRefunded` (pass through; unchanged surface) + its constructor/deps if the coordinator is injected at handler level
- Test: `internal/order/return_refund_integration_test.go`

**Interfaces:**
- Consumes: `orderrefund.Coordinator.Refund`.
- Produces: return happy path (`requested→approved→received→refunded`) moves money and stamps the return, atomically consistent.

**Design note:** to avoid an import cycle (`order` importing `orderrefund` which imports `order`), the coordinator call stays in the **handler** layer, not inside `order.ReturnService`. Change `ReturnsHandler.MarkRefunded` to: (1) call `coordinator.Refund` with `ScopeID = return_id`, then (2) call a slimmed `ReturnService.MarkRefunded` that only stamps the return state + writes the return event (the `RecordRefund` money-bookkeeping now happens inside the coordinator's tx #2). Alternatively pass the return-stamp as a hook. Simpler: coordinator does money + `RecordRefund`; `ReturnService.StampRefundedOnly(ctx, returnID, amount)` stamps `returns` and appends the `return_refunded` event.

- [ ] **Step 1: Add `ReturnService.StampRefundedOnly`**

In `return_service.go`, add a method that does what `MarkRefunded` did **minus** the `orderSvc.RecordRefund` call (that moves to the coordinator):
```go
// StampRefundedOnly transitions received→refunded and stamps the return, WITHOUT
// touching orders.refunded_amount (the coordinator's tx already did that). Used
// by the handler after orderrefund.Coordinator.Refund succeeds.
func (s *ReturnService) StampRefundedOnly(ctx context.Context, returnID uuid.UUID, amount decimal.Decimal, reason string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, _, err := s.repo.GetByID(ctx, tx, returnID)
		if err != nil {
			return err
		}
		if !ReturnStatus(r.Status).CanTransitionTo(ReturnStatusRefunded) {
			return apperrors.InvalidTransition("return", r.Status, string(ReturnStatusRefunded))
		}
		if err := s.repo.UpdateStatus(tx, returnID, ReturnStatusRefunded); err != nil {
			return err
		}
		if err := s.repo.StampRefunded(tx, returnID, amount); err != nil {
			return err
		}
		return s.orderSvc.RecordReturnEvent(ctx, tx, r.OrderID, EventKindReturnRefunded, outbox.EventReturnRefunded, ReturnEventPayload{
			ReturnID: r.ID.String(), ReturnNumber: r.ReturnNumber, Amount: amount.String(), Reason: reason,
		})
	})
}
```
Keep the old `MarkRefunded` for now but stop calling it from the handler.

- [ ] **Step 2: Write the failing integration test**

`internal/order/return_refund_integration_test.go` (tag `integration`): drive request→approve→received, then simulate the handler flow (`coordinator.Refund(ScopeID=returnID)` with a fake gateway + seeded captured txn, then `StampRefundedOnly`). Assert `refund_transactions.succeeded` + `orders.refunded_amount` bumped once + `returns.status='refunded'` + `returns.refund_amount` set.

- [ ] **Step 3: Run to verify it fails**

Run: `go test -tags integration ./internal/order/ -run ReturnRefund -v`
Expected: FAIL.

- [ ] **Step 4: Implement handler change**

In `internal/handlers/admin/returns.go:MarkRefunded`, replace the single `h.svc.MarkRefunded(...)` call with: load the return to get `order_id` + amount, call `h.refunds.Refund(ctx, RefundCommand{OrderID, Amount:&req.Amount, ScopeID:returnID.String(), Reason:req.Reason})`, then `h.svc.StampRefundedOnly(ctx, id, req.Amount, req.Reason)`. Map coordinator errors via `RespondErr`. Inject `h.refunds` into `ReturnsHandler`.

- [ ] **Step 5: Run to verify it passes**

Run: `go test -tags integration ./internal/order/ ./internal/handlers/admin/ -run 'ReturnRefund|Refunded' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/order/return_service.go internal/handlers/admin/returns.go internal/order/return_refund_integration_test.go
git commit -m "feat(refund): return refund moves money via coordinator"
```

---

### Task 11: Paid self-cancel auto-refund

**Files:**
- Modify: `internal/handlers/storefront/order_detail.go:Cancel` (~line 584)
- Modify: `internal/handlers/admin/orders.go:Cancel` (~line 435) — same hook
- Test: `internal/handlers/storefront/cancel_autorefund_integration_test.go`

**Interfaces:**
- Consumes: `orderrefund.Coordinator.Refund` with `Amount=nil` (full), `ScopeID="cancel:"+orderID`.
- Produces: paid, un-shipped self-cancel → order cancelled + full gateway refund + `payment_status=refunded`.

- [ ] **Step 1: Write the failing integration test**

`internal/handlers/storefront/cancel_autorefund_integration_test.go` (tag `integration`): seed a paid, unshipped order for a signed-in customer + captured stripe txn + active config + httptest stub. POST `/cancel`. Assert: order `status=cancelled`, a `refund_transactions.succeeded` full-amount row, `orders.payment_status=refunded`, `orders.refunded_amount == grand_total`. Also add a case: **unpaid** order → cancelled, **no** refund row.

- [ ] **Step 2: Run to verify it fails**

Run: `go test -tags integration ./internal/handlers/storefront/ -run CancelAutoRefund -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `storefront/order_detail.go:Cancel`, after the successful `h.orderSvc.Cancel(...)` and before the email dispatch, add:
```go
	// Auto-refund a paid order on cancellation (spec §4). Best-effort: a
	// gateway blip leaves the order cancelled + a pending ledger row for the
	// sweeper, so the customer is never blocked.
	if h.refunds != nil && o.PaymentStatus == string(order.PaymentStatusPaid) {
		if _, rerr := h.refunds.Refund(c.Request.Context(), orderrefund.RefundCommand{
			OrderID: orderID, Amount: nil, Reason: "order cancelled",
			Actor: "customer", ScopeID: "cancel:" + orderID.String(),
		}); rerr != nil {
			h.logger.Warn("cancel auto-refund deferred to sweeper", "order_id", orderID, "err", rerr)
		}
	}
```
Add `refunds *orderrefund.Coordinator` to `OrderDetailHandler` (+ a `WithRefunds` setter, nil-safe like `WithReturns`). Mirror the same block in `admin/orders.go:Cancel` after its cancel succeeds (Actor = `"user:"+userID`). Wire in `main.go`.

- [ ] **Step 4: Run to verify it passes**

Run: `go test -tags integration ./internal/handlers/storefront/ -run CancelAutoRefund -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/storefront/order_detail.go internal/handlers/admin/orders.go internal/handlers/storefront/cancel_autorefund_integration_test.go
git commit -m "feat(refund): auto-refund paid orders on cancellation"
```

---

### Task 12: Retry sweeper (`cmd/refund-sweep-cron`)

**Files:**
- Create: `cmd/refund-sweep-cron/main.go`
- Modify: `internal/orderrefund/coordinator.go` (add `ResumePending`)
- Test: `internal/orderrefund/sweeper_integration_test.go`

**Interfaces:**
- Consumes: `Coordinator` + `payment.Service.GetRefundByIdempotencyKey`.
- Produces: `Coordinator.ResumePending(ctx, olderThan time.Duration, limit int) (resumed int, err error)` — re-runs `pending` ledger rows with the same key (provider no-op if already refunded), then finalizes.

- [ ] **Step 1: Write the failing integration test**

`internal/orderrefund/sweeper_integration_test.go` (tag `integration`): insert a `pending` `refund_transactions` row (as if tx #2 never ran) with a known key + seeded order/txn/config + httptest stub that records how many times `/v1/refunds` is hit. Call `ResumePending`. Assert: the fake gateway received the **same idempotency key**, the ledger flips to `succeeded`, `orders.refunded_amount` is bumped exactly once.

- [ ] **Step 2: Run to verify it fails**

Run: `go test -tags integration ./internal/orderrefund/ -run Sweeper -v`
Expected: FAIL.

- [ ] **Step 3: Implement `ResumePending`**

In `coordinator.go`:
```go
// ResumePending re-drives refund ledger rows stuck in 'pending' — the never-lost
// guarantee. Safe to run repeatedly: the same idempotency_key means the gateway
// call is a no-op if the money already moved.
func (c *Coordinator) ResumePending(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	var rows []payment.RefundTransaction
	if err := c.db.WithContext(ctx).
		Where("status = 'pending' AND created_at < now() - (? || ' seconds')::interval", int(olderThan.Seconds())).
		Order("created_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return 0, err
	}
	resumed := 0
	for i := range rows {
		row := rows[i]
		oid, err := uuid.Parse(row.OrderID)
		if err != nil {
			continue
		}
		o, _, _, err := c.orderRepo.GetByID(ctx, c.db, oid)
		if err != nil || o == nil {
			continue
		}
		gw, err := c.res.GatewayFor(ctx, o.StoreID, row.Provider)
		if err != nil {
			continue
		}
		ref, err := c.pay.ExecuteGatewayRefund(ctx, gw, payment.RefundInput{
			ProviderPaymentID: row.ProviderPaymentID, Amount: row.Amount,
			CurrencyCode: o.CurrencyCode, Reason: row.Reason, IdempotencyKey: row.IdempotencyKey,
		})
		if err != nil {
			continue // try again next sweep
		}
		target := DeriveStatus(o.RefundedAmount, row.Amount, o.GrandTotal)
		if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := c.pay.FinalizeRefund(ctx, tx, row.ID, ref.ProviderRefundID, "succeeded"); err != nil {
				return err
			}
			return c.orders.RecordRefund(ctx, tx, oid, row.Amount, target, row.Reason)
		}); err != nil {
			continue
		}
		resumed++
	}
	return resumed, nil
}
```
Add `import "time"`.

**Guard against double-bookkeeping:** `order.Service.RecordRefund` re-adds to `refunded_amount`. Since tx #2 either fully committed (row=`succeeded`, not selected here) or fully rolled back (row=`pending`, `refunded_amount` untouched), a `pending` row always means bookkeeping did NOT happen — so re-adding is correct. Add an assertion comment and a test case where tx #2 partially… (not possible — it's one tx). Document this invariant inline.

- [ ] **Step 4: Write the cron entrypoint**

`cmd/refund-sweep-cron/main.go` (mirror `cmd/reconciliation-cron/main.go` structure — DB open from `DATABASE_URL`, build repos, construct `Coordinator` with `enabled=true`, run once, exit):
```go
package main

// Command refund-sweep-cron re-drives stuck 'pending' refund_transactions rows.
// Runs as a Cloud Run Job on a Cloud Scheduler trigger (every 5 min).

import (
	"context"
	"os"
	"time"
	// ... same imports/log setup as reconciliation-cron ...
)

func main() {
	log := newLogger()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Error("refund-sweep-cron: DATABASE_URL not set")
		os.Exit(1)
	}
	db := mustOpen(dsn) // same helper shape as reconciliation-cron
	res := orderrefund.NewResolver(db)
	pay := payment.NewService(payment.NewRepository(db))
	orders := order.NewService(db, order.NewRepository(), outbox.NewRepository())
	coord := orderrefund.NewCoordinator(db, res, pay, orders, order.NewRepository(), true)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	n, err := coord.ResumePending(ctx, 5*time.Minute, 200)
	if err != nil {
		log.Error("refund-sweep-cron: run failed", "err", err)
		os.Exit(1)
	}
	log.Info("refund-sweep-cron: done", "resumed", n)
}
```
(Match the exact logger/db helpers used by `reconciliation-cron`; do not invent new ones.)

- [ ] **Step 5: Run to verify it passes + build the cmd**

Run: `go test -tags integration ./internal/orderrefund/ -run Sweeper -v && go build ./cmd/refund-sweep-cron/`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**

```bash
git add internal/orderrefund/coordinator.go cmd/refund-sweep-cron/main.go internal/orderrefund/sweeper_integration_test.go
git commit -m "feat(refund): retry sweeper for stuck pending refunds"
```

---

### Task 13: Feature flag + main wiring + full regression

**Files:**
- Modify: `cmd/marketplace-api/main.go` (read `REFUND_GATEWAY_ENABLED`, construct `Coordinator`, inject into admin OrdersHandler, ReturnsHandler, storefront OrderDetailHandler)
- Modify: infra deploy config for the new Cloud Run Job + Scheduler (note-only; see spec §10)

**Interfaces:**
- Consumes: everything above.
- Produces: a single `*orderrefund.Coordinator` shared by all handlers, gated by the env flag.

- [ ] **Step 1: Construct the coordinator in main.go**

Near the payment service construction in `main.go`:
```go
	refundEnabled := os.Getenv("REFUND_GATEWAY_ENABLED") == "true"
	refundResolver := orderrefund.NewResolver(db)
	refundCoordinator := orderrefund.NewCoordinator(db, refundResolver, paymentService, orderService, orderRepo, refundEnabled)
	log.Info("refund coordinator wired", "gateway_enabled", refundEnabled)
```
Inject `refundCoordinator` into the three handlers (admin OrdersHandler, admin ReturnsHandler, storefront OrderDetailHandler via `WithRefunds`). Grep their existing construction sites in `main.go` / `routes.go`.

- [ ] **Step 2: Manual flag-off safety check (test)**

Add `internal/orderrefund/coordinator_test.go` case (already in Task 8): `enabled=false` ⇒ `Refund` returns the disabled error and never calls the gateway. Confirm the storefront cancel path treats that error as non-fatal (order still cancelled) — covered by a `enabled=false` variant of the Task 11 test.

- [ ] **Step 3: Full regression**

Run:
```bash
go build ./...
go test ./... 2>&1 | tail -30
go test -tags integration ./internal/orderrefund/ ./internal/order/ ./internal/payment/ ./internal/handlers/... -race
```
Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add cmd/marketplace-api/main.go
git commit -m "feat(refund): wire RefundCoordinator behind REFUND_GATEWAY_ENABLED flag"
```

- [ ] **Step 5: Infra note (separate follow-up, not code)**

Document in the PR/deploy notes: add `cmd/refund-sweep-cron` as a Cloud Run Job + Cloud Scheduler (every 5 min) in `tesserix-k8s` / infra, and set `REFUND_GATEWAY_ENABLED` per environment (dark in prod until the integration suite passes against test-mode keys).

---

## Self-Review

**Spec coverage:**
- §1 problem (orphaned gateway) → Tasks 8–11. ✅
- §3 decision 1 auto-refund paid cancel → Task 11. ✅
- §3 decision 2 no-gateway blocked (422) → Task 8 (`RefundUnavailable`) + Task 2. ✅
- §3 decision 3 derive status → Task 8 `DeriveStatus`. ✅
- §3 decision 4 idempotency + ledger + sweeper → Tasks 6, 8, 12. ✅
- §3 decision 5 standalone Job → Task 12. ✅
- §3 decision 6 feature flag → Tasks 8, 13. ✅
- §4.1 coordinator → Task 8. §4.2 saga → Tasks 6, 8. §4.3 sweeper → Task 12. §4.4 triggers → Tasks 9–11. ✅
- §5 schema → Task 1. §6 API (refund_request_id, refund_unavailable) → Tasks 2, 9. §7 gateway idempotency → Tasks 3–5. §8 validation → Task 8. §9 tests → every task + Task 13 regression. §10 rollout → Tasks 12–13. ✅

**Placeholder scan:** No TBD/TODO left; each code step shows real code. Test bodies for integration tasks reference the existing package harness (grep-to-find) rather than reinventing — acceptable since the harness exists; the assertions are concrete.

**Type consistency:** `RefundCommand{OrderID, Amount *decimal.Decimal, Reason, Actor, ScopeID}`, `RefundResult{ProviderRefundID, Amount, PaymentStatus, AlreadyDone}`, `ReserveRefundInput`, `PaymentContext`, `DeriveStatus(refunded, amount, grandTotal)`, `Coordinator.Refund/ResumePending`, `payment.Service.ReserveRefund/ExecuteGatewayRefund/FinalizeRefund`, `RefundInput.IdempotencyKey` — used consistently across Tasks 3–13. `order.Service.RecordRefund(ctx, tx, orderID, amount, target, reason)` matches the real signature (`service.go:335`).

**Known risk to flag at execution:** Task 8's unit tests require narrow interface seams for the coordinator's deps (so fakes inject without a DB). If introducing those interfaces balloons scope, fall back to DB-backed integration tests for the coordinator and keep only `DeriveStatus` + idempotency-key as pure unit tests.
