# Marketing M1 — Coupons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship coupon CRUD (admin), coupon validation (storefront), checkout integration, with atomic usage tracking and rate-limited public endpoints.

**Architecture:** New `internal/coupon/` package (models, repository, service) + admin handler + storefront handler + rate-limit middleware. Migration 000009. Checkout integration via the `discount.Applier` interface.

**Tech Stack:** Go 1.26, Gin, GORM, shopspring/decimal, crypto/rand. Next.js 16, React 19, Tailwind.

---

## Post-Review Amendments (2026-04-10)

> These amendments override the corresponding sections in the plan below.

### CRITICAL FIX 1: Coupon apply MUST run inside the order creation transaction

The plan's checkout integration (Task 8) validates the coupon before order creation, then calls `applier.Apply()` in a separate transaction after the order is already created. This violates spec §8.2 which requires coupon apply, usage increment, and order creation inside a **single** `db.Transaction()`.

**Required change:** In `checkout_ext.go`, the coupon validation + `applier.Apply()` + `orderSvc.Create()` must all run inside ONE `orderSvc.Unit()` call. The validate-then-apply double-read is racy (coupon could expire/hit limit between reads). Fix: validate + increment atomically inside the order transaction. If `applier.Apply()` fails, the entire order rolls back. Do NOT swallow the error as "non-fatal".

### CRITICAL FIX 2: Validate-then-apply TOCTOU race

Remove the separate `couponSvc.Validate()` call before the transaction. Instead, pass the coupon code into the transaction and do validate + apply atomically inside. The two-phase approach allows the coupon to hit its limit between validate and apply.

### HIGH FIX 3: Add `tenant_id` to `coupon_usage` queries

`ListUsage` and `CountCustomerUsage` in `repository.go` must include `tenant_id` in WHERE clauses, not just `coupon_id` / `customer_email`. Spec §9 explicitly adds `tenant_id` to `coupon_usage`.

### HIGH FIX 4: Rate limiter needs TTL-based eviction

The `sync.Map` in the rate limiter grows unboundedly. Add a cleanup goroutine that runs every 5 minutes and removes entries older than 2× the window duration.

### HIGH FIX 5: `Patch` must validate `status` field

`PatchInput.Status` is `*string` with no validation. Add enum validation: only `"active"` and `"disabled"` are valid patch targets. Never allow patching to `"expired"` (that's system-managed).

### MEDIUM FIX 6: Repository + handler tests need real Postgres integration tests

The plan's tests are trivially shallow (constructor non-nil checks). Add integration tests for:
- `IncrementUsageInTx` atomic path
- Concurrent usage race test (spec §12)
- HTTP round-trip tests for validate endpoint (expired, rate-limited, not found)

### MEDIUM FIX 7: `TargetIDs` UUID validation

`pq.StringArray` for a PostgreSQL `UUID[]` column bypasses format validation. Validate UUIDs in the service layer before write.

### LOW FIX 8: `CouponForm` state consolidation

Replace 14 individual `useState` calls with `useReducer` or a single state object.

### LOW FIX 9: Share coupon repo/service instances

`couponRepoSF` / `couponSvcSF` in `main.go` are unnecessary — `gormRepository` is stateless and `Service` is read-safe. Share the same instances between admin and storefront.

---

## Decisions Locked

1. **Atomic usage_count:** Coupon usage is incremented via a single `UPDATE coupons SET usage_count = usage_count + 1 WHERE id = $id AND (usage_limit IS NULL OR usage_count < usage_limit) RETURNING usage_count` inside the checkout transaction. Zero rows = limit reached. Never read-then-write.

2. **Rate limiting:** Per-IP sliding window (10 req/min) on `POST /storefront/.../coupons/validate`. Implemented as a Gin middleware using an in-memory token bucket. No Redis dependency. Uses `sync.Map` keyed by IP with a `rate.Limiter` from `golang.org/x/time/rate`.

3. **Progressive disclosure form:** Admin coupon create form shows 4 fields always (code, discount type, value, expiry date). Advanced options (min purchase, max discount, usage limit, per-customer limit, target type, target IDs, stackable, start date) are behind an "Advanced options" toggle.

4. **discount.Applier interface:** Shared interface in `internal/discount/applier.go`. Coupon implements it. Future gift card and loyalty redemption will also implement it. `checkout_ext.go` calls appliers in sequence inside the DB transaction.

---

## File Structure

### New files — Go backend

```
services/marketplace-api/
├── migrations/
│   ├── 000009_coupons.up.sql                          # CREATE TABLE coupons + coupon_usage
│   └── 000009_coupons.down.sql                        # DROP TABLE coupon_usage, coupons
├── internal/
│   ├── discount/
│   │   └── applier.go                                 # Applier interface
│   ├── coupon/
│   │   ├── models.go                                  # Coupon, CouponUsage GORM models + constants
│   │   ├── models_test.go                             # Model validation tests
│   │   ├── repository.go                              # Repository interface + GORM implementation
│   │   ├── repository_test.go                         # Repository tests (unit, uses mock DB)
│   │   ├── service.go                                 # Service (validate, apply, CRUD)
│   │   └── service_test.go                            # Service tests
│   ├── ratelimit/
│   │   ├── middleware.go                               # Per-IP token bucket Gin middleware
│   │   └── middleware_test.go                          # Middleware tests
│   └── handlers/
│       ├── admin/
│       │   ├── coupons.go                             # CouponHandler (List, Create, Get, Patch, Delete)
│       │   ├── coupons_dto.go                         # Request/response DTOs for coupons
│       │   └── coupons_test.go                        # Handler tests
│       └── storefront/
│           ├── coupons.go                             # CouponValidateHandler (Validate endpoint)
│           └── coupons_test.go                        # Storefront handler tests
```

### New files — Admin UI

```
apps/admin/
├── lib/api/
│   └── coupons-api.ts                                 # API client for coupon CRUD
├── app/marketing/
│   └── coupons/
│       ├── page.tsx                                   # List page
│       ├── new/
│       │   └── page.tsx                               # Create page (progressive disclosure form)
│       └── [id]/
│           └── page.tsx                               # Detail page
├── components/marketing/coupons/
│   ├── CouponsListHeader.tsx                          # Header with title + "Create coupon" CTA
│   ├── CouponsListFilters.tsx                         # Status + search filters
│   ├── CouponsList.tsx                                # Table component
│   ├── CouponsListEmpty.tsx                           # Empty state
│   ├── CouponForm.tsx                                 # Progressive disclosure create/edit form
│   ├── CouponDetailSummary.tsx                        # Config summary card on detail page
│   └── CouponUsageTable.tsx                           # Usage history table on detail page
```

### New files — Storefront UI

```
apps/storefront/
├── components/checkout/
│   └── CouponInput.tsx                                # Coupon code input with inline validation
```

### Modified files

```
services/marketplace-api/
├── pkg/apperrors/errors.go                            # Add coupon error codes + sentinels
├── internal/handlers/admin/routes.go                  # Add CouponHandler to Deps + mount routes
├── internal/handlers/admin/errors.go                  # Add coupon codes to codeStatus map
├── internal/handlers/storefront/routes.go             # Add CouponValidateHandler to Deps + mount route
├── internal/handlers/storefront/checkout_ext.go       # Add coupon_code field + call discount.Applier
├── cmd/marketplace-api/main.go                        # Wire coupon repo, service, handlers

apps/admin/
├── lib/api/marketplace-api.ts                         # (no change needed — coupons-api.ts is separate)

apps/storefront/
├── lib/api/checkout-api.ts                            # Add coupon_code to CheckoutBody + discount_total to result
├── app/checkout/page.tsx                              # Add CouponInput accordion
```

---

## Task 1 — Migration 000009: Coupons Schema

### Steps

- [ ] **1.1** Create up migration at `services/marketplace-api/migrations/000009_coupons.up.sql`
- [ ] **1.2** Create down migration at `services/marketplace-api/migrations/000009_coupons.down.sql`
- [ ] **1.3** Run the migration against the local dev database to verify
- [ ] **1.4** Commit

### 1.1 — Up migration

**File:** `services/marketplace-api/migrations/000009_coupons.up.sql`

```sql
-- 000009_coupons.up.sql
-- Marketing M1: Coupons + coupon usage tracking.

CREATE TABLE coupons (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    code            VARCHAR(50)   NOT NULL,
    title           VARCHAR(200)  NOT NULL,
    description     TEXT,
    type            VARCHAR(20)   NOT NULL,  -- 'percentage', 'fixed_amount', 'free_shipping'
    value           NUMERIC(12,2) NOT NULL,  -- percent value or fixed amount
    currency_code   CHAR(3),                 -- required for fixed_amount
    min_purchase    NUMERIC(12,2),           -- NULL = no minimum
    max_discount    NUMERIC(12,2),           -- cap for percentage coupons
    usage_limit     INT,                     -- NULL = unlimited total uses
    per_customer    INT           NOT NULL DEFAULT 1,
    target_type     VARCHAR(20)   NOT NULL DEFAULT 'all', -- 'all', 'products', 'categories'
    target_ids      UUID[],                  -- product or category IDs when targeted
    stackable       BOOLEAN       NOT NULL DEFAULT false,
    starts_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    ends_at         TIMESTAMPTZ,             -- NULL = no expiry
    status          VARCHAR(20)   NOT NULL DEFAULT 'active', -- 'active', 'disabled', 'expired'
    usage_count     INT           NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, code)
);
CREATE INDEX coupons_store_status_idx ON coupons (store_id, status);

CREATE TABLE coupon_usage (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    coupon_id       UUID          NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    order_id        UUID          NOT NULL REFERENCES orders(id),
    customer_email  VARCHAR(300)  NOT NULL,
    discount_amount NUMERIC(12,2) NOT NULL,
    currency_code   CHAR(3)       NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX coupon_usage_coupon_idx ON coupon_usage (coupon_id);
CREATE INDEX coupon_usage_email_idx ON coupon_usage (coupon_id, customer_email);
```

### 1.2 — Down migration

**File:** `services/marketplace-api/migrations/000009_coupons.down.sql`

```sql
-- 000009_coupons.down.sql
DROP TABLE IF EXISTS coupon_usage;
DROP TABLE IF EXISTS coupons;
```

### 1.3 — Verify migration

```bash
cd services/marketplace-api && go run ./cmd/migrate up
```

### 1.4 — Commit

```
feat(marketplace-api): add migration 000009 for coupons schema (M1)
```

---

## Task 2 — `internal/discount/applier.go` (Shared Interface)

### Steps

- [ ] **2.1** Create `services/marketplace-api/internal/discount/applier.go`
- [ ] **2.2** Commit

### 2.1 — Applier interface

**File:** `services/marketplace-api/internal/discount/applier.go`

```go
// Package discount defines the shared discount application interface.
// Coupon, gift card, and loyalty redemption each implement Applier.
// The checkout handler iterates []Applier in order inside a single
// DB transaction.
package discount

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ApplyInput contains the context needed to calculate and record a discount.
type ApplyInput struct {
	TenantID      uuid.UUID
	StoreID       uuid.UUID
	OrderID       uuid.UUID
	CustomerEmail string
	Subtotal      decimal.Decimal // pre-discount subtotal
	CurrencyCode  string
}

// ApplyResult contains the outcome of applying a discount.
type ApplyResult struct {
	DiscountAmount decimal.Decimal // amount deducted from the order
	Description    string          // human-readable label, e.g. "SAVE20 — 20% off"
}

// Applier applies a discount to an order inside the provided transaction.
// The caller (checkout_ext.go) owns the tx lifecycle — Applier must NOT
// commit or rollback. Returns (zero result, nil) when the discount does
// not apply (e.g. free_shipping on a digital-only order).
type Applier interface {
	Apply(ctx context.Context, tx *gorm.DB, in ApplyInput) (ApplyResult, error)
}
```

### 2.2 — Commit

```
feat(marketplace-api): add discount.Applier interface for checkout integration (M1)
```

---

## Task 3 — Domain Errors in `pkg/apperrors`

### Steps

- [ ] **3.1** Write test for new error codes in `services/marketplace-api/pkg/apperrors/errors_test.go`
- [ ] **3.2** Run test — verify it FAILS (RED)
- [ ] **3.3** Add coupon error codes, sentinels, and constructors to `services/marketplace-api/pkg/apperrors/errors.go`
- [ ] **3.4** Update `IsKnownCode` switch in `errors.go` to include new codes
- [ ] **3.5** Update `codeStatus` map in `services/marketplace-api/internal/handlers/admin/errors.go`
- [ ] **3.6** Run test — verify it PASSES (GREEN)
- [ ] **3.7** Commit

### 3.1 — Test additions

Add to the existing test file `services/marketplace-api/pkg/apperrors/errors_test.go`. Append a new test function:

```go
func TestCouponErrorCodes(t *testing.T) {
	couponCodes := []string{
		"coupon_not_found",
		"coupon_expired",
		"coupon_usage_limit_reached",
		"coupon_invalid",
		"coupon_min_purchase_not_met",
	}
	for _, code := range couponCodes {
		if !IsKnownCode(code) {
			t.Errorf("expected %q to be a known code", code)
		}
	}
}

func TestCouponNotFoundConstructor(t *testing.T) {
	err := CouponNotFound("SAVE20")
	if err.Code != CodeCouponNotFound {
		t.Errorf("expected code %q, got %q", CodeCouponNotFound, err.Code)
	}
	if err.Details["code"] != "SAVE20" {
		t.Errorf("expected detail code SAVE20, got %v", err.Details["code"])
	}
}

func TestCouponExpiredConstructor(t *testing.T) {
	err := CouponExpired("SAVE20")
	if err.Code != CodeCouponExpired {
		t.Errorf("expected code %q, got %q", CodeCouponExpired, err.Code)
	}
}

func TestCouponUsageLimitReachedConstructor(t *testing.T) {
	err := CouponUsageLimitReached("SAVE20", 100)
	if err.Code != CodeCouponUsageLimitReached {
		t.Errorf("expected code %q, got %q", CodeCouponUsageLimitReached, err.Code)
	}
	if err.Details["usage_limit"] != 100 {
		t.Errorf("expected usage_limit 100, got %v", err.Details["usage_limit"])
	}
}

func TestCouponInvalidConstructor(t *testing.T) {
	err := CouponInvalid("coupon is not stackable")
	if err.Code != CodeCouponInvalid {
		t.Errorf("expected code %q, got %q", CodeCouponInvalid, err.Code)
	}
}

func TestCouponMinPurchaseNotMetConstructor(t *testing.T) {
	err := CouponMinPurchaseNotMet("SAVE20", "50.00", "30.00")
	if err.Code != CodeCouponMinPurchaseNotMet {
		t.Errorf("expected code %q, got %q", CodeCouponMinPurchaseNotMet, err.Code)
	}
}
```

### 3.2 — Run test (RED)

```bash
cd services/marketplace-api && go test ./pkg/apperrors/ -run TestCoupon -v
```

Expect: compilation errors because codes/constructors don't exist yet.

### 3.3 — Add codes and constructors to `errors.go`

Append to the `const` block in `services/marketplace-api/pkg/apperrors/errors.go`, after the Orders slice 1 codes:

```go
	// Coupons M1.
	CodeCouponNotFound          Code = "coupon_not_found"
	CodeCouponExpired           Code = "coupon_expired"
	CodeCouponUsageLimitReached Code = "coupon_usage_limit_reached"
	CodeCouponInvalid           Code = "coupon_invalid"
	CodeCouponMinPurchaseNotMet Code = "coupon_min_purchase_not_met"
```

Append to the `var` block (sentinels):

```go
	// Coupons M1 sentinels.
	ErrCouponNotFound          = &Error{Code: CodeCouponNotFound}
	ErrCouponExpired           = &Error{Code: CodeCouponExpired}
	ErrCouponUsageLimitReached = &Error{Code: CodeCouponUsageLimitReached}
	ErrCouponInvalid           = &Error{Code: CodeCouponInvalid}
	ErrCouponMinPurchaseNotMet = &Error{Code: CodeCouponMinPurchaseNotMet}
```

Add constructors at the bottom of `errors.go`:

```go
// ---------- Coupons M1 constructors ----------

func CouponNotFound(code string) *Error {
	return &Error{Code: CodeCouponNotFound,
		Message: fmt.Sprintf("coupon %q not found", code),
		Details: map[string]any{"code": code}}
}

func CouponExpired(code string) *Error {
	return &Error{Code: CodeCouponExpired,
		Message: fmt.Sprintf("coupon %q has expired", code),
		Details: map[string]any{"code": code}}
}

func CouponUsageLimitReached(code string, limit int) *Error {
	return &Error{Code: CodeCouponUsageLimitReached,
		Message: fmt.Sprintf("coupon %q has reached its usage limit", code),
		Details: map[string]any{"code": code, "usage_limit": limit}}
}

func CouponInvalid(reason string) *Error {
	return &Error{Code: CodeCouponInvalid,
		Message: reason}
}

func CouponMinPurchaseNotMet(code, minPurchase, subtotal string) *Error {
	return &Error{Code: CodeCouponMinPurchaseNotMet,
		Message: fmt.Sprintf("coupon %q requires a minimum purchase of %s (subtotal: %s)", code, minPurchase, subtotal),
		Details: map[string]any{"code": code, "min_purchase": minPurchase, "subtotal": subtotal}}
}
```

### 3.4 — Update `IsKnownCode` switch

In `services/marketplace-api/pkg/apperrors/errors.go`, find the `IsKnownCode` function and add the new codes to the switch:

```go
		CodeReturnItemsExceedOrdered, CodeRecoveryTooRecent,
		CodeCouponNotFound, CodeCouponExpired, CodeCouponUsageLimitReached,
		CodeCouponInvalid, CodeCouponMinPurchaseNotMet:
```

### 3.5 — Update `codeStatus` map

In `services/marketplace-api/internal/handlers/admin/errors.go`, append to the `codeStatus` map:

```go
	// Coupons M1.
	apperrors.CodeCouponNotFound:          http.StatusNotFound,
	apperrors.CodeCouponExpired:           http.StatusUnprocessableEntity,
	apperrors.CodeCouponUsageLimitReached: http.StatusUnprocessableEntity,
	apperrors.CodeCouponInvalid:           http.StatusUnprocessableEntity,
	apperrors.CodeCouponMinPurchaseNotMet: http.StatusUnprocessableEntity,
```

### 3.6 — Run test (GREEN)

```bash
cd services/marketplace-api && go test ./pkg/apperrors/ -run TestCoupon -v
```

### 3.7 — Commit

```
feat(marketplace-api): add coupon domain error codes and constructors (M1)
```

---

## Task 4 — `internal/coupon/` Package (Models, Repository, Service)

### Steps

- [ ] **4.1** Create `services/marketplace-api/internal/coupon/models.go`
- [ ] **4.2** Create `services/marketplace-api/internal/coupon/models_test.go` — test model validation
- [ ] **4.3** Run test — verify RED
- [ ] **4.4** Implement model validation methods
- [ ] **4.5** Run test — verify GREEN
- [ ] **4.6** Create `services/marketplace-api/internal/coupon/repository.go`
- [ ] **4.7** Create `services/marketplace-api/internal/coupon/repository_test.go`
- [ ] **4.8** Create `services/marketplace-api/internal/coupon/service.go`
- [ ] **4.9** Create `services/marketplace-api/internal/coupon/service_test.go`
- [ ] **4.10** Run all coupon tests — verify GREEN
- [ ] **4.11** Commit

### 4.1 — Models

**File:** `services/marketplace-api/internal/coupon/models.go`

```go
// Package coupon implements coupon CRUD, validation, and atomic usage
// tracking for the marketplace-api. Part of Marketing M1.
package coupon

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

// CouponType enumerates discount types.
type CouponType string

const (
	CouponTypePercentage   CouponType = "percentage"
	CouponTypeFixedAmount  CouponType = "fixed_amount"
	CouponTypeFreeShipping CouponType = "free_shipping"
)

// CouponStatus enumerates coupon lifecycle states.
type CouponStatus string

const (
	CouponStatusActive   CouponStatus = "active"
	CouponStatusDisabled CouponStatus = "disabled"
	CouponStatusExpired  CouponStatus = "expired"
)

// CouponTargetType enumerates what a coupon targets.
type CouponTargetType string

const (
	CouponTargetAll        CouponTargetType = "all"
	CouponTargetProducts   CouponTargetType = "products"
	CouponTargetCategories CouponTargetType = "categories"
)

// Coupon is the GORM model for the coupons table.
type Coupon struct {
	ID           uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID     uuid.UUID        `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID      uuid.UUID        `gorm:"column:store_id;type:uuid;not null"`
	Code         string           `gorm:"column:code;type:varchar(50);not null"`
	Title        string           `gorm:"column:title;type:varchar(200);not null"`
	Description  *string          `gorm:"column:description;type:text"`
	Type         CouponType       `gorm:"column:type;type:varchar(20);not null"`
	Value        decimal.Decimal  `gorm:"column:value;type:numeric(12,2);not null"`
	CurrencyCode *string          `gorm:"column:currency_code;type:char(3)"`
	MinPurchase  *decimal.Decimal `gorm:"column:min_purchase;type:numeric(12,2)"`
	MaxDiscount  *decimal.Decimal `gorm:"column:max_discount;type:numeric(12,2)"`
	UsageLimit   *int             `gorm:"column:usage_limit;type:int"`
	PerCustomer  int              `gorm:"column:per_customer;type:int;not null;default:1"`
	TargetType   CouponTargetType `gorm:"column:target_type;type:varchar(20);not null;default:all"`
	TargetIDs    pq.StringArray   `gorm:"column:target_ids;type:uuid[]"`
	Stackable    bool             `gorm:"column:stackable;type:boolean;not null;default:false"`
	StartsAt     time.Time        `gorm:"column:starts_at;not null;default:now()"`
	EndsAt       *time.Time       `gorm:"column:ends_at"`
	Status       CouponStatus     `gorm:"column:status;type:varchar(20);not null;default:active"`
	UsageCount   int              `gorm:"column:usage_count;type:int;not null;default:0"`
	CreatedAt    time.Time        `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt    time.Time        `gorm:"column:updated_at;not null;default:now()"`
}

func (Coupon) TableName() string { return "coupons" }

// CouponUsage records a single use of a coupon on an order.
type CouponUsage struct {
	ID             uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID       uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	CouponID       uuid.UUID       `gorm:"column:coupon_id;type:uuid;not null"`
	OrderID        uuid.UUID       `gorm:"column:order_id;type:uuid;not null"`
	CustomerEmail  string          `gorm:"column:customer_email;type:varchar(300);not null"`
	DiscountAmount decimal.Decimal `gorm:"column:discount_amount;type:numeric(12,2);not null"`
	CurrencyCode   string          `gorm:"column:currency_code;type:char(3);not null"`
	CreatedAt      time.Time       `gorm:"column:created_at;not null;default:now()"`
}

func (CouponUsage) TableName() string { return "coupon_usage" }

// IsActive returns true if the coupon is active, started, and not expired.
func (c *Coupon) IsActive(now time.Time) bool {
	if c.Status != CouponStatusActive {
		return false
	}
	if now.Before(c.StartsAt) {
		return false
	}
	if c.EndsAt != nil && now.After(*c.EndsAt) {
		return false
	}
	return true
}

// HasUsageCapacity returns true if the coupon has not reached its total
// usage limit. Always true when usage_limit is NULL (unlimited).
func (c *Coupon) HasUsageCapacity() bool {
	if c.UsageLimit == nil {
		return true
	}
	return c.UsageCount < *c.UsageLimit
}

// CalculateDiscount computes the discount amount for a given subtotal.
// Returns decimal.Zero for free_shipping (the discount is applied as
// shipping waiver, not subtotal reduction).
func (c *Coupon) CalculateDiscount(subtotal decimal.Decimal) decimal.Decimal {
	switch c.Type {
	case CouponTypePercentage:
		discount := subtotal.Mul(c.Value).Div(decimal.NewFromInt(100))
		if c.MaxDiscount != nil && discount.GreaterThan(*c.MaxDiscount) {
			discount = *c.MaxDiscount
		}
		return discount
	case CouponTypeFixedAmount:
		if c.Value.GreaterThan(subtotal) {
			return subtotal // never discount more than subtotal
		}
		return c.Value
	case CouponTypeFreeShipping:
		return decimal.Zero
	default:
		return decimal.Zero
	}
}

// ValidateType returns an error string if the coupon type is invalid.
func ValidateType(t string) bool {
	switch CouponType(t) {
	case CouponTypePercentage, CouponTypeFixedAmount, CouponTypeFreeShipping:
		return true
	}
	return false
}
```

### 4.2 — Model tests

**File:** `services/marketplace-api/internal/coupon/models_test.go`

```go
package coupon

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestCoupon_IsActive(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	tests := []struct {
		name   string
		coupon Coupon
		want   bool
	}{
		{
			name:   "active coupon within date range",
			coupon: Coupon{Status: CouponStatusActive, StartsAt: past, EndsAt: &future},
			want:   true,
		},
		{
			name:   "active coupon no expiry",
			coupon: Coupon{Status: CouponStatusActive, StartsAt: past, EndsAt: nil},
			want:   true,
		},
		{
			name:   "disabled coupon",
			coupon: Coupon{Status: CouponStatusDisabled, StartsAt: past, EndsAt: &future},
			want:   false,
		},
		{
			name:   "not started yet",
			coupon: Coupon{Status: CouponStatusActive, StartsAt: future},
			want:   false,
		},
		{
			name:   "already expired",
			coupon: Coupon{Status: CouponStatusActive, StartsAt: past, EndsAt: &past},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.coupon.IsActive(now)
			if got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoupon_HasUsageCapacity(t *testing.T) {
	limit10 := 10
	tests := []struct {
		name   string
		coupon Coupon
		want   bool
	}{
		{
			name:   "unlimited",
			coupon: Coupon{UsageLimit: nil, UsageCount: 9999},
			want:   true,
		},
		{
			name:   "under limit",
			coupon: Coupon{UsageLimit: &limit10, UsageCount: 5},
			want:   true,
		},
		{
			name:   "at limit",
			coupon: Coupon{UsageLimit: &limit10, UsageCount: 10},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.coupon.HasUsageCapacity()
			if got != tt.want {
				t.Errorf("HasUsageCapacity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoupon_CalculateDiscount(t *testing.T) {
	maxDiscount := decimal.NewFromFloat(25.00)

	tests := []struct {
		name     string
		coupon   Coupon
		subtotal decimal.Decimal
		want     decimal.Decimal
	}{
		{
			name: "percentage 20% on $100",
			coupon: Coupon{
				Type:  CouponTypePercentage,
				Value: decimal.NewFromInt(20),
			},
			subtotal: decimal.NewFromFloat(100.00),
			want:     decimal.NewFromFloat(20.00),
		},
		{
			name: "percentage 50% on $100 capped at $25",
			coupon: Coupon{
				Type:        CouponTypePercentage,
				Value:       decimal.NewFromInt(50),
				MaxDiscount: &maxDiscount,
			},
			subtotal: decimal.NewFromFloat(100.00),
			want:     decimal.NewFromFloat(25.00),
		},
		{
			name: "fixed $15 on $100",
			coupon: Coupon{
				Type:  CouponTypeFixedAmount,
				Value: decimal.NewFromFloat(15.00),
			},
			subtotal: decimal.NewFromFloat(100.00),
			want:     decimal.NewFromFloat(15.00),
		},
		{
			name: "fixed $200 on $100 — capped at subtotal",
			coupon: Coupon{
				Type:  CouponTypeFixedAmount,
				Value: decimal.NewFromFloat(200.00),
			},
			subtotal: decimal.NewFromFloat(100.00),
			want:     decimal.NewFromFloat(100.00),
		},
		{
			name: "free shipping returns zero",
			coupon: Coupon{
				Type: CouponTypeFreeShipping,
			},
			subtotal: decimal.NewFromFloat(100.00),
			want:     decimal.Zero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.coupon.CalculateDiscount(tt.subtotal)
			if !got.Equal(tt.want) {
				t.Errorf("CalculateDiscount() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestValidateType(t *testing.T) {
	if !ValidateType("percentage") {
		t.Error("expected percentage to be valid")
	}
	if !ValidateType("fixed_amount") {
		t.Error("expected fixed_amount to be valid")
	}
	if !ValidateType("free_shipping") {
		t.Error("expected free_shipping to be valid")
	}
	if ValidateType("bogus") {
		t.Error("expected bogus to be invalid")
	}
}

func TestCoupon_TableName(t *testing.T) {
	c := Coupon{}
	if c.TableName() != "coupons" {
		t.Errorf("expected table name 'coupons', got %q", c.TableName())
	}
}

func TestCouponUsage_TableName(t *testing.T) {
	u := CouponUsage{}
	if u.TableName() != "coupon_usage" {
		t.Errorf("expected table name 'coupon_usage', got %q", u.TableName())
	}
}

// Ensure uuid fields are uuid type (compile-time check).
var _ uuid.UUID = Coupon{}.ID
var _ uuid.UUID = CouponUsage{}.CouponID
```

### 4.3 — Run test (RED)

```bash
cd services/marketplace-api && go test ./internal/coupon/ -v
```

This step runs after 4.1 is saved but before validation methods are complete.

### 4.4 — Implement model validation methods (already included in 4.1)

The model file in 4.1 includes `IsActive`, `HasUsageCapacity`, `CalculateDiscount`, and `ValidateType`.

### 4.5 — Run test (GREEN)

```bash
cd services/marketplace-api && go test ./internal/coupon/ -v
```

### 4.6 — Repository

**File:** `services/marketplace-api/internal/coupon/repository.go`

```go
package coupon

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ListFilter holds the query parameters for listing coupons.
type ListFilter struct {
	StoreID  uuid.UUID
	TenantID uuid.UUID
	Status   string // optional filter
	Search   string // optional code/title search
	Page     int
	PerPage  int
}

// ListResult holds a page of coupons and the total count.
type ListResult struct {
	Coupons []Coupon
	Total   int64
}

// Repository is the data-access surface for coupons. Mutating methods
// take an explicit *gorm.DB so callers can thread a transaction through.
type Repository interface {
	// List returns a filtered, paginated list of coupons.
	List(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error)

	// GetByID returns a single coupon by primary key, scoped to store.
	GetByID(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) (*Coupon, error)

	// GetByCode returns a coupon by its code within a store.
	GetByCode(ctx context.Context, db *gorm.DB, storeID uuid.UUID, code string) (*Coupon, error)

	// Create inserts a new coupon row.
	Create(ctx context.Context, db *gorm.DB, c *Coupon) error

	// Update patches mutable fields on a coupon.
	Update(ctx context.Context, db *gorm.DB, c *Coupon) error

	// SoftDisable sets status = 'disabled' on the coupon.
	SoftDisable(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) error

	// IncrementUsageInTx atomically increments usage_count by 1 inside the
	// supplied tx. Returns apperrors.ErrCouponUsageLimitReached when the
	// UPDATE matches zero rows (limit already hit).
	IncrementUsageInTx(tx *gorm.DB, couponID uuid.UUID) error

	// RecordUsage inserts a coupon_usage row inside the supplied tx.
	RecordUsage(tx *gorm.DB, u *CouponUsage) error

	// CountCustomerUsage returns how many times a customer email has used
	// a specific coupon.
	CountCustomerUsage(ctx context.Context, db *gorm.DB, couponID uuid.UUID, email string) (int64, error)

	// ListUsage returns the usage records for a coupon, ordered by created_at desc.
	ListUsage(ctx context.Context, db *gorm.DB, couponID uuid.UUID, page, perPage int) ([]CouponUsage, int64, error)
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) List(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error) {
	var result ListResult
	q := db.WithContext(ctx).Model(&Coupon{}).Where("store_id = ? AND tenant_id = ?", f.StoreID, f.TenantID)

	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Search != "" {
		like := "%" + strings.ToLower(f.Search) + "%"
		q = q.Where("LOWER(code) LIKE ? OR LOWER(title) LIKE ?", like, like)
	}

	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("coupon list count: %w", err)
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	perPage := f.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	if err := q.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&result.Coupons).Error; err != nil {
		return result, fmt.Errorf("coupon list: %w", err)
	}
	return result, nil
}

func (gormRepository) GetByID(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) (*Coupon, error) {
	var c Coupon
	if err := db.WithContext(ctx).Where("store_id = ? AND id = ?", storeID, id).First(&c).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("coupon")
		}
		return nil, fmt.Errorf("coupon get by id: %w", err)
	}
	return &c, nil
}

func (gormRepository) GetByCode(ctx context.Context, db *gorm.DB, storeID uuid.UUID, code string) (*Coupon, error) {
	var c Coupon
	upperCode := strings.ToUpper(strings.TrimSpace(code))
	if err := db.WithContext(ctx).Where("store_id = ? AND UPPER(code) = ?", storeID, upperCode).First(&c).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.CouponNotFound(code)
		}
		return nil, fmt.Errorf("coupon get by code: %w", err)
	}
	return &c, nil
}

func (gormRepository) Create(ctx context.Context, db *gorm.DB, c *Coupon) error {
	if err := db.WithContext(ctx).Create(c).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key") && strings.Contains(err.Error(), "store_id") {
			return apperrors.CouponInvalid(fmt.Sprintf("coupon code %q already exists in this store", c.Code))
		}
		return fmt.Errorf("coupon create: %w", err)
	}
	return nil
}

func (gormRepository) Update(ctx context.Context, db *gorm.DB, c *Coupon) error {
	if err := db.WithContext(ctx).Save(c).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key") && strings.Contains(err.Error(), "store_id") {
			return apperrors.CouponInvalid(fmt.Sprintf("coupon code %q already exists in this store", c.Code))
		}
		return fmt.Errorf("coupon update: %w", err)
	}
	return nil
}

func (gormRepository) SoftDisable(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) error {
	res := db.WithContext(ctx).Model(&Coupon{}).
		Where("store_id = ? AND id = ?", storeID, id).
		Update("status", CouponStatusDisabled)
	if res.Error != nil {
		return fmt.Errorf("coupon soft disable: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("coupon")
	}
	return nil
}

// IncrementUsageInTx atomically increments usage_count. The single UPDATE
// guards against overshooting usage_limit via the WHERE clause. If zero
// rows are affected, the limit was already reached.
func (gormRepository) IncrementUsageInTx(tx *gorm.DB, couponID uuid.UUID) error {
	res := tx.Exec(`
		UPDATE coupons
		SET usage_count = usage_count + 1, updated_at = now()
		WHERE id = ?
		  AND (usage_limit IS NULL OR usage_count < usage_limit)
	`, couponID)
	if res.Error != nil {
		return fmt.Errorf("coupon increment usage: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.CouponUsageLimitReached("", 0)
	}
	return nil
}

func (gormRepository) RecordUsage(tx *gorm.DB, u *CouponUsage) error {
	if err := tx.Create(u).Error; err != nil {
		return fmt.Errorf("coupon record usage: %w", err)
	}
	return nil
}

func (gormRepository) CountCustomerUsage(ctx context.Context, db *gorm.DB, couponID uuid.UUID, email string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Model(&CouponUsage{}).
		Where("coupon_id = ? AND customer_email = ?", couponID, email).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("coupon count customer usage: %w", err)
	}
	return count, nil
}

func (gormRepository) ListUsage(ctx context.Context, db *gorm.DB, couponID uuid.UUID, page, perPage int) ([]CouponUsage, int64, error) {
	var total int64
	q := db.WithContext(ctx).Model(&CouponUsage{}).Where("coupon_id = ?", couponID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("coupon usage count: %w", err)
	}

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	var usages []CouponUsage
	if err := q.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&usages).Error; err != nil {
		return nil, 0, fmt.Errorf("coupon usage list: %w", err)
	}
	return usages, total, nil
}
```

### 4.7 — Repository tests

**File:** `services/marketplace-api/internal/coupon/repository_test.go`

```go
package coupon

import (
	"testing"

	"github.com/google/uuid"
)

// TestNewRepository verifies construction returns a non-nil interface.
func TestNewRepository(t *testing.T) {
	repo := NewRepository()
	if repo == nil {
		t.Fatal("NewRepository() returned nil")
	}
}

// TestListFilter_Defaults verifies sensible defaults for ListFilter.
func TestListFilter_Defaults(t *testing.T) {
	f := ListFilter{
		StoreID:  uuid.New(),
		TenantID: uuid.New(),
	}
	if f.Page != 0 {
		t.Errorf("expected zero-value page, got %d", f.Page)
	}
	// The repository normalises page < 1 to 1 internally.
}
```

### 4.8 — Service

**File:** `services/marketplace-api/internal/coupon/service.go`

```go
package coupon

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/discount"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ServiceConfig groups dependencies for the coupon service.
type ServiceConfig struct {
	DB     *gorm.DB
	Repo   Repository
	Logger *slog.Logger
}

// Service implements coupon CRUD and validation logic.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewService constructs a coupon Service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:     cfg.DB,
		repo:   cfg.Repo,
		logger: cfg.Logger,
	}
}

// ---------- CRUD ----------

// CreateInput holds the fields for creating a coupon.
type CreateInput struct {
	TenantID     uuid.UUID
	StoreID      uuid.UUID
	Code         string
	Title        string
	Description  *string
	Type         string
	Value        decimal.Decimal
	CurrencyCode *string
	MinPurchase  *decimal.Decimal
	MaxDiscount  *decimal.Decimal
	UsageLimit   *int
	PerCustomer  int
	TargetType   string
	TargetIDs    []string
	Stackable    bool
	StartsAt     *time.Time
	EndsAt       *time.Time
}

// Create validates and persists a new coupon.
func (s *Service) Create(ctx context.Context, in CreateInput) (*Coupon, error) {
	if err := s.validateCreateInput(in); err != nil {
		return nil, err
	}

	now := time.Now()
	startsAt := now
	if in.StartsAt != nil {
		startsAt = *in.StartsAt
	}

	perCustomer := in.PerCustomer
	if perCustomer < 1 {
		perCustomer = 1
	}

	targetType := CouponTargetAll
	if in.TargetType != "" {
		targetType = CouponTargetType(in.TargetType)
	}

	c := &Coupon{
		TenantID:     in.TenantID,
		StoreID:      in.StoreID,
		Code:         strings.ToUpper(strings.TrimSpace(in.Code)),
		Title:        strings.TrimSpace(in.Title),
		Description:  in.Description,
		Type:         CouponType(in.Type),
		Value:        in.Value,
		CurrencyCode: in.CurrencyCode,
		MinPurchase:  in.MinPurchase,
		MaxDiscount:  in.MaxDiscount,
		UsageLimit:   in.UsageLimit,
		PerCustomer:  perCustomer,
		TargetType:   targetType,
		TargetIDs:    in.TargetIDs,
		Stackable:    in.Stackable,
		StartsAt:     startsAt,
		EndsAt:       in.EndsAt,
		Status:       CouponStatusActive,
		UsageCount:   0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Create(ctx, s.db, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) validateCreateInput(in CreateInput) error {
	if strings.TrimSpace(in.Code) == "" {
		return apperrors.ValidationFailed("code", "code is required")
	}
	if len(in.Code) > 50 {
		return apperrors.ValidationFailed("code", "code must be 50 characters or fewer")
	}
	if strings.TrimSpace(in.Title) == "" {
		return apperrors.ValidationFailed("title", "title is required")
	}
	if !ValidateType(in.Type) {
		return apperrors.ValidationFailed("type", "type must be percentage, fixed_amount, or free_shipping")
	}
	if in.Value.IsNegative() {
		return apperrors.ValidationFailed("value", "value must be non-negative")
	}
	if CouponType(in.Type) == CouponTypePercentage && in.Value.GreaterThan(decimal.NewFromInt(100)) {
		return apperrors.ValidationFailed("value", "percentage value must be between 0 and 100")
	}
	if CouponType(in.Type) == CouponTypeFixedAmount && (in.CurrencyCode == nil || *in.CurrencyCode == "") {
		return apperrors.ValidationFailed("currency_code", "currency_code is required for fixed_amount coupons")
	}
	if in.EndsAt != nil && in.StartsAt != nil && in.EndsAt.Before(*in.StartsAt) {
		return apperrors.ValidationFailed("ends_at", "ends_at must be after starts_at")
	}
	return nil
}

// PatchInput holds the fields for updating a coupon. Nil fields are not updated.
type PatchInput struct {
	Title       *string
	Description *string
	MinPurchase *decimal.Decimal
	MaxDiscount *decimal.Decimal
	UsageLimit  *int
	PerCustomer *int
	Stackable   *bool
	StartsAt    *time.Time
	EndsAt      *time.Time
	Status      *string
}

// Patch updates mutable fields on an existing coupon.
func (s *Service) Patch(ctx context.Context, storeID, id uuid.UUID, in PatchInput) (*Coupon, error) {
	c, err := s.repo.GetByID(ctx, s.db, storeID, id)
	if err != nil {
		return nil, err
	}

	if in.Title != nil {
		c.Title = strings.TrimSpace(*in.Title)
	}
	if in.Description != nil {
		c.Description = in.Description
	}
	if in.MinPurchase != nil {
		c.MinPurchase = in.MinPurchase
	}
	if in.MaxDiscount != nil {
		c.MaxDiscount = in.MaxDiscount
	}
	if in.UsageLimit != nil {
		c.UsageLimit = in.UsageLimit
	}
	if in.PerCustomer != nil {
		c.PerCustomer = *in.PerCustomer
	}
	if in.Stackable != nil {
		c.Stackable = *in.Stackable
	}
	if in.StartsAt != nil {
		c.StartsAt = *in.StartsAt
	}
	if in.EndsAt != nil {
		c.EndsAt = in.EndsAt
	}
	if in.Status != nil {
		c.Status = CouponStatus(*in.Status)
	}
	c.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, s.db, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Get returns a single coupon by ID with store scope.
func (s *Service) Get(ctx context.Context, storeID, id uuid.UUID) (*Coupon, error) {
	return s.repo.GetByID(ctx, s.db, storeID, id)
}

// List returns a paginated list of coupons for a store.
func (s *Service) List(ctx context.Context, f ListFilter) (ListResult, error) {
	return s.repo.List(ctx, s.db, f)
}

// Delete soft-disables a coupon.
func (s *Service) Delete(ctx context.Context, storeID, id uuid.UUID) error {
	return s.repo.SoftDisable(ctx, s.db, storeID, id)
}

// ---------- Validation (storefront) ----------

// ValidateResult is the discount preview returned by Validate.
type ValidateResult struct {
	CouponID       uuid.UUID       `json:"coupon_id"`
	Code           string          `json:"code"`
	Type           CouponType      `json:"type"`
	Value          decimal.Decimal `json:"value"`
	DiscountAmount decimal.Decimal `json:"discount_amount"`
	FreeShipping   bool            `json:"free_shipping"`
	Title          string          `json:"title"`
}

// ValidateInput holds the parameters for storefront coupon validation.
type ValidateInput struct {
	StoreID       uuid.UUID
	Code          string
	CustomerEmail string
	Subtotal      decimal.Decimal
}

// Validate checks if a coupon code is valid for the given context and
// returns a discount preview. Does NOT apply or increment usage.
func (s *Service) Validate(ctx context.Context, in ValidateInput) (*ValidateResult, error) {
	c, err := s.repo.GetByCode(ctx, s.db, in.StoreID, in.Code)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if !c.IsActive(now) {
		if c.EndsAt != nil && now.After(*c.EndsAt) {
			return nil, apperrors.CouponExpired(c.Code)
		}
		return nil, apperrors.CouponInvalid("coupon is not currently active")
	}

	if !c.HasUsageCapacity() {
		limit := 0
		if c.UsageLimit != nil {
			limit = *c.UsageLimit
		}
		return nil, apperrors.CouponUsageLimitReached(c.Code, limit)
	}

	// Per-customer check.
	if in.CustomerEmail != "" {
		count, err := s.repo.CountCustomerUsage(ctx, s.db, c.ID, in.CustomerEmail)
		if err != nil {
			return nil, fmt.Errorf("coupon validate: %w", err)
		}
		if count >= int64(c.PerCustomer) {
			return nil, apperrors.CouponUsageLimitReached(c.Code, c.PerCustomer)
		}
	}

	// Min purchase check.
	if c.MinPurchase != nil && in.Subtotal.LessThan(*c.MinPurchase) {
		return nil, apperrors.CouponMinPurchaseNotMet(c.Code, c.MinPurchase.StringFixed(2), in.Subtotal.StringFixed(2))
	}

	discountAmount := c.CalculateDiscount(in.Subtotal)

	return &ValidateResult{
		CouponID:       c.ID,
		Code:           c.Code,
		Type:           c.Type,
		Value:          c.Value,
		DiscountAmount: discountAmount,
		FreeShipping:   c.Type == CouponTypeFreeShipping,
		Title:          c.Title,
	}, nil
}

// ---------- Checkout apply (discount.Applier) ----------

// CouponApplier implements discount.Applier for coupon discounts.
// Created by the checkout handler with the validated coupon code.
type CouponApplier struct {
	svc           *Service
	code          string
	customerEmail string
}

// NewCouponApplier creates an Applier that will validate and apply the
// given coupon code during checkout.
func NewCouponApplier(svc *Service, code, customerEmail string) *CouponApplier {
	return &CouponApplier{svc: svc, code: code, customerEmail: customerEmail}
}

// Apply implements discount.Applier. It validates the coupon, atomically
// increments usage_count, records a coupon_usage row, and returns the
// discount amount — all inside the caller's transaction.
func (a *CouponApplier) Apply(ctx context.Context, tx *gorm.DB, in discount.ApplyInput) (discount.ApplyResult, error) {
	zero := discount.ApplyResult{}

	// Look up coupon inside the transaction to get a consistent snapshot.
	c, err := a.svc.repo.GetByCode(ctx, tx, in.StoreID, a.code)
	if err != nil {
		return zero, err
	}

	now := time.Now()
	if !c.IsActive(now) {
		if c.EndsAt != nil && now.After(*c.EndsAt) {
			return zero, apperrors.CouponExpired(c.Code)
		}
		return zero, apperrors.CouponInvalid("coupon is not currently active")
	}

	// Min purchase.
	if c.MinPurchase != nil && in.Subtotal.LessThan(*c.MinPurchase) {
		return zero, apperrors.CouponMinPurchaseNotMet(c.Code, c.MinPurchase.StringFixed(2), in.Subtotal.StringFixed(2))
	}

	// Per-customer check.
	if a.customerEmail != "" {
		count, err := a.svc.repo.CountCustomerUsage(ctx, tx, c.ID, a.customerEmail)
		if err != nil {
			return zero, fmt.Errorf("coupon apply: %w", err)
		}
		if count >= int64(c.PerCustomer) {
			return zero, apperrors.CouponUsageLimitReached(c.Code, c.PerCustomer)
		}
	}

	// Atomic usage increment.
	if err := a.svc.repo.IncrementUsageInTx(tx, c.ID); err != nil {
		return zero, err
	}

	discountAmount := c.CalculateDiscount(in.Subtotal)

	// Record usage.
	usage := &CouponUsage{
		TenantID:       in.TenantID,
		CouponID:       c.ID,
		OrderID:        in.OrderID,
		CustomerEmail:  a.customerEmail,
		DiscountAmount: discountAmount,
		CurrencyCode:   in.CurrencyCode,
	}
	if err := a.svc.repo.RecordUsage(tx, usage); err != nil {
		return zero, err
	}

	desc := fmt.Sprintf("%s — %s", c.Code, c.Title)

	return discount.ApplyResult{
		DiscountAmount: discountAmount,
		Description:    desc,
	}, nil
}

// Unit runs fn inside a GORM transaction. Exposed so handlers can use
// the service's DB connection for transactional work.
func (s *Service) Unit(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// ListUsage returns usage records for a coupon.
func (s *Service) ListUsage(ctx context.Context, couponID uuid.UUID, page, perPage int) ([]CouponUsage, int64, error) {
	return s.repo.ListUsage(ctx, s.db, couponID, page, perPage)
}
```

### 4.9 — Service tests

**File:** `services/marketplace-api/internal/coupon/service_test.go`

```go
package coupon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

func TestService_validateCreateInput(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name    string
		input   CreateInput
		wantErr bool
		errCode apperrors.Code
	}{
		{
			name:    "empty code",
			input:   CreateInput{Code: "", Title: "Test", Type: "percentage", Value: decimal.NewFromInt(10)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "code too long",
			input:   CreateInput{Code: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Title: "T", Type: "percentage", Value: decimal.NewFromInt(10)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "empty title",
			input:   CreateInput{Code: "SAVE10", Title: "", Type: "percentage", Value: decimal.NewFromInt(10)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "invalid type",
			input:   CreateInput{Code: "SAVE10", Title: "T", Type: "bogus", Value: decimal.NewFromInt(10)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "negative value",
			input:   CreateInput{Code: "SAVE10", Title: "T", Type: "percentage", Value: decimal.NewFromInt(-5)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "percentage over 100",
			input:   CreateInput{Code: "SAVE10", Title: "T", Type: "percentage", Value: decimal.NewFromInt(150)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "fixed_amount without currency",
			input:   CreateInput{Code: "FLAT10", Title: "T", Type: "fixed_amount", Value: decimal.NewFromInt(10)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name: "valid percentage",
			input: CreateInput{
				Code:    "SAVE20",
				Title:   "Save 20%",
				Type:    "percentage",
				Value:   decimal.NewFromInt(20),
			},
			wantErr: false,
		},
		{
			name: "valid fixed_amount",
			input: func() CreateInput {
				cur := "USD"
				return CreateInput{
					Code:         "FLAT10",
					Title:        "Flat $10 off",
					Type:         "fixed_amount",
					Value:        decimal.NewFromInt(10),
					CurrencyCode: &cur,
				}
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.validateCreateInput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var ae *apperrors.Error
				if !errors.As(err, &ae) {
					t.Fatalf("expected *apperrors.Error, got %T", err)
				}
				if ae.Code != tt.errCode {
					t.Errorf("expected code %q, got %q", tt.errCode, ae.Code)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateResult_FreeShipping(t *testing.T) {
	r := ValidateResult{
		CouponID:       uuid.New(),
		Code:           "FREESHIP",
		Type:           CouponTypeFreeShipping,
		Value:          decimal.Zero,
		DiscountAmount: decimal.Zero,
		FreeShipping:   true,
	}
	if !r.FreeShipping {
		t.Error("expected FreeShipping to be true")
	}
}

func TestNewCouponApplier(t *testing.T) {
	svc := &Service{}
	a := NewCouponApplier(svc, "SAVE20", "test@example.com")
	if a == nil {
		t.Fatal("expected non-nil applier")
	}
	if a.code != "SAVE20" {
		t.Errorf("expected code SAVE20, got %q", a.code)
	}
	if a.customerEmail != "test@example.com" {
		t.Errorf("expected email test@example.com, got %q", a.customerEmail)
	}
}

func TestNewService(t *testing.T) {
	svc := NewService(ServiceConfig{})
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// Verify CouponApplier satisfies discount.Applier at compile time.
// (Cannot import discount package from within coupon due to circular
// import risk — the real check happens in checkout_ext_test.go or
// main.go compilation.)
```

### 4.10 — Run all coupon tests (GREEN)

```bash
cd services/marketplace-api && go test ./internal/coupon/ -v -count=1
```

### 4.11 — Commit

```
feat(marketplace-api): add coupon models, repository, and service with TDD (M1)
```

---

## Task 5 — Rate-Limit Middleware

### Steps

- [ ] **5.1** Create test at `services/marketplace-api/internal/ratelimit/middleware_test.go`
- [ ] **5.2** Run test — verify RED
- [ ] **5.3** Create `services/marketplace-api/internal/ratelimit/middleware.go`
- [ ] **5.4** Run test — verify GREEN
- [ ] **5.5** Commit

### 5.1 — Middleware test

**File:** `services/marketplace-api/internal/ratelimit/middleware_test.go`

```go
package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPerIP_AllowsUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PerIP(10, 10)) // 10 req/s, burst 10
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
	}
}

func TestPerIP_BlocksOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Very low limit: 1 req/s, burst 1
	r.Use(PerIP(1, 1))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// First request should pass.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w.Code)
	}

	// Second immediate request should be rate-limited.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req2.RemoteAddr = "1.2.3.4:12345"
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", w2.Code)
	}
}

func TestPerIP_DifferentIPsIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PerIP(1, 1))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// IP A
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req1.RemoteAddr = "1.1.1.1:12345"
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("ip-a: expected 200, got %d", w1.Code)
	}

	// IP B — should still be allowed
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req2.RemoteAddr = "2.2.2.2:12345"
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("ip-b: expected 200, got %d", w2.Code)
	}
}
```

### 5.2 — Run test (RED)

```bash
cd services/marketplace-api && go test ./internal/ratelimit/ -v
```

### 5.3 — Middleware implementation

**File:** `services/marketplace-api/internal/ratelimit/middleware.go`

```go
// Package ratelimit provides per-IP rate-limiting Gin middleware using
// an in-memory token bucket. No Redis dependency.
package ratelimit

import (
	"net"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// PerIP returns a Gin middleware that limits requests per client IP.
// rps is the sustained requests-per-second; burst is the maximum burst.
// For example, PerIP(0.167, 10) gives ~10 req/min with a burst of 10.
//
// Implementation: sync.Map of *rate.Limiter keyed by IP string. The map
// grows unboundedly; in production this is acceptable because the
// storefront sits behind Cloudflare which caps unique IPs. For a
// long-running dev server, entries are never evicted — acceptable for
// local use.
func PerIP(rps float64, burst int) gin.HandlerFunc {
	var limiters sync.Map

	return func(c *gin.Context) {
		ip := extractIP(c)

		val, _ := limiters.LoadOrStore(ip, rate.NewLimiter(rate.Limit(rps), burst))
		limiter := val.(*rate.Limiter)

		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limited",
				"message": "too many requests, please try again later",
			})
			return
		}

		c.Next()
	}
}

// extractIP gets the client IP from X-Forwarded-For (Cloudflare/Istio),
// falling back to RemoteAddr.
func extractIP(c *gin.Context) string {
	// Trust first X-Forwarded-For value (set by Cloudflare).
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// Take the first IP in the comma-separated list.
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Fallback to RemoteAddr (strip port).
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return ip
}
```

### 5.4 — Run test (GREEN)

```bash
cd services/marketplace-api && go test ./internal/ratelimit/ -v
```

### 5.5 — Commit

```
feat(marketplace-api): add per-IP rate-limit middleware for storefront endpoints (M1)
```

---

## Task 6 — Admin Coupon Handler

### Steps

- [ ] **6.1** Create DTOs at `services/marketplace-api/internal/handlers/admin/coupons_dto.go`
- [ ] **6.2** Create handler at `services/marketplace-api/internal/handlers/admin/coupons.go`
- [ ] **6.3** Create handler test at `services/marketplace-api/internal/handlers/admin/coupons_test.go`
- [ ] **6.4** Run test — verify GREEN
- [ ] **6.5** Commit

### 6.1 — DTOs

**File:** `services/marketplace-api/internal/handlers/admin/coupons_dto.go`

```go
package admin

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/coupon"
)

// ---------- Request DTOs ----------

// CreateCouponRequest is the JSON body for POST /admin/stores/:storeId/coupons.
type CreateCouponRequest struct {
	Code         string           `json:"code"          binding:"required,max=50"`
	Title        string           `json:"title"         binding:"required,max=200"`
	Description  *string          `json:"description"`
	Type         string           `json:"type"          binding:"required,oneof=percentage fixed_amount free_shipping"`
	Value        decimal.Decimal  `json:"value"         binding:"required"`
	CurrencyCode *string          `json:"currency_code"`
	MinPurchase  *decimal.Decimal `json:"min_purchase"`
	MaxDiscount  *decimal.Decimal `json:"max_discount"`
	UsageLimit   *int             `json:"usage_limit"`
	PerCustomer  int              `json:"per_customer"`
	TargetType   string           `json:"target_type"`
	TargetIDs    []string         `json:"target_ids"`
	Stackable    bool             `json:"stackable"`
	StartsAt     *time.Time       `json:"starts_at"`
	EndsAt       *time.Time       `json:"ends_at"`
}

// PatchCouponRequest is the JSON body for PATCH /admin/stores/:storeId/coupons/:id.
type PatchCouponRequest struct {
	Title       *string          `json:"title"`
	Description *string          `json:"description"`
	MinPurchase *decimal.Decimal `json:"min_purchase"`
	MaxDiscount *decimal.Decimal `json:"max_discount"`
	UsageLimit  *int             `json:"usage_limit"`
	PerCustomer *int             `json:"per_customer"`
	Stackable   *bool            `json:"stackable"`
	StartsAt    *time.Time       `json:"starts_at"`
	EndsAt      *time.Time       `json:"ends_at"`
	Status      *string          `json:"status"`
}

// ---------- Response DTOs ----------

// AdminCouponResponse is the JSON envelope for a coupon returned by admin endpoints.
type AdminCouponResponse struct {
	ID           string           `json:"id"`
	Code         string           `json:"code"`
	Title        string           `json:"title"`
	Description  *string          `json:"description"`
	Type         string           `json:"type"`
	Value        decimal.Decimal  `json:"value"`
	CurrencyCode *string          `json:"currency_code"`
	MinPurchase  *decimal.Decimal `json:"min_purchase"`
	MaxDiscount  *decimal.Decimal `json:"max_discount"`
	UsageLimit   *int             `json:"usage_limit"`
	PerCustomer  int              `json:"per_customer"`
	TargetType   string           `json:"target_type"`
	TargetIDs    []string         `json:"target_ids"`
	Stackable    bool             `json:"stackable"`
	StartsAt     string           `json:"starts_at"`
	EndsAt       *string          `json:"ends_at"`
	Status       string           `json:"status"`
	UsageCount   int              `json:"usage_count"`
	CreatedAt    string           `json:"created_at"`
	UpdatedAt    string           `json:"updated_at"`
}

// AdminCouponUsageResponse is a single coupon_usage row for the detail page.
type AdminCouponUsageResponse struct {
	ID             string          `json:"id"`
	OrderID        string          `json:"order_id"`
	CustomerEmail  string          `json:"customer_email"`
	DiscountAmount decimal.Decimal `json:"discount_amount"`
	CurrencyCode   string          `json:"currency_code"`
	CreatedAt      string          `json:"created_at"`
}

// toAdminCouponResponse maps a domain Coupon to the admin JSON response.
func toAdminCouponResponse(c *coupon.Coupon) AdminCouponResponse {
	r := AdminCouponResponse{
		ID:           c.ID.String(),
		Code:         c.Code,
		Title:        c.Title,
		Description:  c.Description,
		Type:         string(c.Type),
		Value:        c.Value,
		CurrencyCode: c.CurrencyCode,
		MinPurchase:  c.MinPurchase,
		MaxDiscount:  c.MaxDiscount,
		UsageLimit:   c.UsageLimit,
		PerCustomer:  c.PerCustomer,
		TargetType:   string(c.TargetType),
		TargetIDs:    c.TargetIDs,
		Stackable:    c.Stackable,
		StartsAt:     c.StartsAt.Format(time.RFC3339),
		Status:       string(c.Status),
		UsageCount:   c.UsageCount,
		CreatedAt:    c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    c.UpdatedAt.Format(time.RFC3339),
	}
	if c.EndsAt != nil {
		s := c.EndsAt.Format(time.RFC3339)
		r.EndsAt = &s
	}
	if r.TargetIDs == nil {
		r.TargetIDs = []string{}
	}
	return r
}

// toAdminCouponUsageResponse maps a CouponUsage to the admin JSON response.
func toAdminCouponUsageResponse(u *coupon.CouponUsage) AdminCouponUsageResponse {
	return AdminCouponUsageResponse{
		ID:             u.ID.String(),
		OrderID:        u.OrderID.String(),
		CustomerEmail:  u.CustomerEmail,
		DiscountAmount: u.DiscountAmount,
		CurrencyCode:   u.CurrencyCode,
		CreatedAt:      u.CreatedAt.Format(time.RFC3339),
	}
}
```

### 6.2 — Handler

**File:** `services/marketplace-api/internal/handlers/admin/coupons.go`

```go
// Package admin — coupons.go: HTTP handler for the per-store
// coupon CRUD surface mounted at /admin/stores/:storeId/coupons.
package admin

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CouponHandler bundles dependencies for the admin coupon endpoints.
type CouponHandler struct {
	svc    *coupon.Service
	logger *slog.Logger
}

// NewCouponHandler constructs a CouponHandler.
func NewCouponHandler(svc *coupon.Service, logger *slog.Logger) *CouponHandler {
	return &CouponHandler{svc: svc, logger: logger}
}

// List handles GET /admin/stores/:storeId/coupons.
func (h *CouponHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	f := coupon.ListFilter{
		StoreID:  storeID,
		TenantID: tenantID,
		Status:   c.Query("status"),
		Search:   c.Query("search"),
		Page:     page,
		PerPage:  perPage,
	}

	result, err := h.svc.List(c.Request.Context(), f)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]AdminCouponResponse, 0, len(result.Coupons))
	for i := range result.Coupons {
		out = append(out, toAdminCouponResponse(&result.Coupons[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  out,
		"total": result.Total,
		"page":  page,
	})
}

// Create handles POST /admin/stores/:storeId/coupons.
func (h *CouponHandler) Create(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req CreateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	in := coupon.CreateInput{
		TenantID:     tenantID,
		StoreID:      storeID,
		Code:         req.Code,
		Title:        req.Title,
		Description:  req.Description,
		Type:         req.Type,
		Value:        req.Value,
		CurrencyCode: req.CurrencyCode,
		MinPurchase:  req.MinPurchase,
		MaxDiscount:  req.MaxDiscount,
		UsageLimit:   req.UsageLimit,
		PerCustomer:  req.PerCustomer,
		TargetType:   req.TargetType,
		TargetIDs:    req.TargetIDs,
		Stackable:    req.Stackable,
		StartsAt:     req.StartsAt,
		EndsAt:       req.EndsAt,
	}

	created, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toAdminCouponResponse(created)})
}

// Get handles GET /admin/stores/:storeId/coupons/:id.
func (h *CouponHandler) Get(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	couponID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	cpn, err := h.svc.Get(c.Request.Context(), storeID, couponID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	// Fetch usage stats.
	page, _ := strconv.Atoi(c.DefaultQuery("usage_page", "1"))
	usages, usageTotal, err := h.svc.ListUsage(c.Request.Context(), couponID, page, 20)
	if err != nil {
		// Non-fatal: return the coupon without usage data.
		h.logger.Warn("coupon get: usage list failed", "err", err)
		c.JSON(http.StatusOK, gin.H{"data": toAdminCouponResponse(cpn)})
		return
	}

	usageOut := make([]AdminCouponUsageResponse, 0, len(usages))
	for i := range usages {
		usageOut = append(usageOut, toAdminCouponUsageResponse(&usages[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        toAdminCouponResponse(cpn),
		"usage":       usageOut,
		"usage_total": usageTotal,
	})
}

// Patch handles PATCH /admin/stores/:storeId/coupons/:id.
func (h *CouponHandler) Patch(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	couponID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	var req PatchCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	in := coupon.PatchInput{
		Title:       req.Title,
		Description: req.Description,
		MinPurchase: req.MinPurchase,
		MaxDiscount: req.MaxDiscount,
		UsageLimit:  req.UsageLimit,
		PerCustomer: req.PerCustomer,
		Stackable:   req.Stackable,
		StartsAt:    req.StartsAt,
		EndsAt:      req.EndsAt,
		Status:      req.Status,
	}

	updated, err := h.svc.Patch(c.Request.Context(), storeID, couponID, in)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toAdminCouponResponse(updated)})
}

// Delete handles DELETE /admin/stores/:storeId/coupons/:id.
// This is a soft-disable, not a hard delete.
func (h *CouponHandler) Delete(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	couponID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	if err := h.svc.Delete(c.Request.Context(), storeID, couponID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "coupon disabled"})
}
```

### 6.3 — Handler test

**File:** `services/marketplace-api/internal/handlers/admin/coupons_test.go`

```go
package admin

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/coupon"
)

func TestToAdminCouponResponse(t *testing.T) {
	now := time.Now()
	expires := now.Add(24 * time.Hour)
	c := &coupon.Coupon{
		ID:          uuid.New(),
		TenantID:    uuid.New(),
		StoreID:     uuid.New(),
		Code:        "SAVE20",
		Title:       "Save 20%",
		Type:        coupon.CouponTypePercentage,
		Value:       decimal.NewFromInt(20),
		PerCustomer: 1,
		TargetType:  coupon.CouponTargetAll,
		StartsAt:    now,
		EndsAt:      &expires,
		Status:      coupon.CouponStatusActive,
		UsageCount:  5,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	r := toAdminCouponResponse(c)

	if r.ID != c.ID.String() {
		t.Errorf("expected ID %s, got %s", c.ID, r.ID)
	}
	if r.Code != "SAVE20" {
		t.Errorf("expected code SAVE20, got %s", r.Code)
	}
	if r.Type != "percentage" {
		t.Errorf("expected type percentage, got %s", r.Type)
	}
	if r.UsageCount != 5 {
		t.Errorf("expected usage_count 5, got %d", r.UsageCount)
	}
	if r.EndsAt == nil {
		t.Error("expected EndsAt to be set")
	}
	if len(r.TargetIDs) != 0 {
		t.Errorf("expected empty TargetIDs, got %v", r.TargetIDs)
	}
}

func TestToAdminCouponResponse_NilEndsAt(t *testing.T) {
	c := &coupon.Coupon{
		ID:         uuid.New(),
		TenantID:   uuid.New(),
		StoreID:    uuid.New(),
		Code:       "FOREVER",
		Title:      "Forever",
		Type:       coupon.CouponTypePercentage,
		Value:      decimal.NewFromInt(10),
		TargetType: coupon.CouponTargetAll,
		Status:     coupon.CouponStatusActive,
	}

	r := toAdminCouponResponse(c)
	if r.EndsAt != nil {
		t.Error("expected EndsAt to be nil")
	}
}

func TestToAdminCouponUsageResponse(t *testing.T) {
	u := &coupon.CouponUsage{
		ID:             uuid.New(),
		CouponID:       uuid.New(),
		OrderID:        uuid.New(),
		CustomerEmail:  "test@example.com",
		DiscountAmount: decimal.NewFromFloat(15.50),
		CurrencyCode:   "USD",
		CreatedAt:      time.Now(),
	}

	r := toAdminCouponUsageResponse(u)
	if r.CustomerEmail != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", r.CustomerEmail)
	}
	if !r.DiscountAmount.Equal(decimal.NewFromFloat(15.50)) {
		t.Errorf("expected discount 15.50, got %s", r.DiscountAmount)
	}
}

func TestNewCouponHandler(t *testing.T) {
	h := NewCouponHandler(nil, nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}
```

### 6.4 — Run test (GREEN)

```bash
cd services/marketplace-api && go test ./internal/handlers/admin/ -run TestCoupon -v
```

### 6.5 — Commit

```
feat(marketplace-api): add admin coupon CRUD handler with DTOs (M1)
```

---

## Task 7 — Storefront Coupon Validate Handler

### Steps

- [ ] **7.1** Create handler at `services/marketplace-api/internal/handlers/storefront/coupons.go`
- [ ] **7.2** Create test at `services/marketplace-api/internal/handlers/storefront/coupons_test.go`
- [ ] **7.3** Run test — verify GREEN
- [ ] **7.4** Commit

### 7.1 — Handler

**File:** `services/marketplace-api/internal/handlers/storefront/coupons.go`

```go
// Package storefront — coupons.go: storefront coupon validation endpoint.
// POST /storefront/stores/:storeSlug/coupons/validate
package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CouponValidateHandler handles storefront coupon validation.
type CouponValidateHandler struct {
	svc    *coupon.Service
	logger *slog.Logger
}

// NewCouponValidateHandler constructs a CouponValidateHandler.
func NewCouponValidateHandler(svc *coupon.Service, logger *slog.Logger) *CouponValidateHandler {
	return &CouponValidateHandler{svc: svc, logger: logger}
}

// ValidateCouponRequest is the JSON body for the validate endpoint.
type ValidateCouponRequest struct {
	Code          string          `json:"code"           binding:"required"`
	CustomerEmail string          `json:"customer_email" binding:"required,email"`
	Subtotal      decimal.Decimal `json:"subtotal"       binding:"required"`
}

// ValidateCouponResponse is the JSON response for a successful validation.
type ValidateCouponResponse struct {
	CouponID       string          `json:"coupon_id"`
	Code           string          `json:"code"`
	Type           string          `json:"type"`
	Value          decimal.Decimal `json:"value"`
	DiscountAmount decimal.Decimal `json:"discount_amount"`
	FreeShipping   bool            `json:"free_shipping"`
	Title          string          `json:"title"`
}

// Validate handles POST /storefront/stores/:storeSlug/coupons/validate.
func (h *CouponValidateHandler) Validate(c *gin.Context) {
	storeVal, ok := c.Get("store")
	if !ok {
		respondNotFound(c)
		return
	}
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		respondNotFound(c)
		return
	}

	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		h.respondErr(c, apperrors.ValidationFailed("store.id", "invalid uuid"))
		return
	}

	var req ValidateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondErr(c, apperrors.ValidationFailed("body", err.Error()))
		return
	}

	result, err := h.svc.Validate(c.Request.Context(), coupon.ValidateInput{
		StoreID:       storeID,
		Code:          req.Code,
		CustomerEmail: req.CustomerEmail,
		Subtotal:      req.Subtotal,
	})
	if err != nil {
		h.respondErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": ValidateCouponResponse{
			CouponID:       result.CouponID.String(),
			Code:           result.Code,
			Type:           string(result.Type),
			Value:          result.Value,
			DiscountAmount: result.DiscountAmount,
			FreeShipping:   result.FreeShipping,
			Title:          result.Title,
		},
	})
}

// respondErr mirrors the checkout_ext.go error response pattern.
func (h *CouponValidateHandler) respondErr(c *gin.Context, err error) {
	var ae *apperrors.Error
	if asErr, ok := err.(*apperrors.Error); ok {
		ae = asErr
	}
	if ae != nil {
		switch ae.Code {
		case apperrors.CodeValidationFailed:
			c.AbortWithStatusJSON(http.StatusBadRequest, map[string]any{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		case apperrors.CodeCouponNotFound:
			c.AbortWithStatusJSON(http.StatusNotFound, map[string]any{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		case apperrors.CodeCouponExpired,
			apperrors.CodeCouponUsageLimitReached,
			apperrors.CodeCouponInvalid,
			apperrors.CodeCouponMinPurchaseNotMet:
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, map[string]any{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		}
	}
	if h.logger != nil {
		h.logger.Error("coupon validate: unhandled error", "err", err.Error())
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
		"error":   "internal",
		"message": "internal server error",
	})
}
```

### 7.2 — Test

**File:** `services/marketplace-api/internal/handlers/storefront/coupons_test.go`

```go
package storefront

import (
	"testing"
)

func TestNewCouponValidateHandler(t *testing.T) {
	h := NewCouponValidateHandler(nil, nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestValidateCouponResponse_Fields(t *testing.T) {
	r := ValidateCouponResponse{
		CouponID:     "test-id",
		Code:         "SAVE20",
		Type:         "percentage",
		FreeShipping: false,
		Title:        "Save 20%",
	}
	if r.Code != "SAVE20" {
		t.Errorf("expected code SAVE20, got %s", r.Code)
	}
	if r.FreeShipping {
		t.Error("expected FreeShipping to be false")
	}
}
```

### 7.3 — Run test (GREEN)

```bash
cd services/marketplace-api && go test ./internal/handlers/storefront/ -run TestCoupon -v
```

### 7.4 — Commit

```
feat(marketplace-api): add storefront coupon validate handler (M1)
```

---

## Task 8 — Checkout Integration

### Steps

- [ ] **8.1** Add `coupon_code` field to `CheckoutExtRequest` in `checkout_ext.go`
- [ ] **8.2** Add `discount_total` and `coupon_code` to `CheckoutExtResponse`
- [ ] **8.3** Add coupon discount logic in the Checkout method between step 1 (resolve country) and step 2 (calculate shipping)
- [ ] **8.4** Wire free_shipping coupon type to override shipping total
- [ ] **8.5** Commit

### 8.1–8.4 — Modify `checkout_ext.go`

**File:** `services/marketplace-api/internal/handlers/storefront/checkout_ext.go`

Add import for coupon package. Modify `CheckoutExtRequest` to include `CouponCode`:

```go
// In the imports block, add:
	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/internal/discount"
```

Add field to `CheckoutExtRequest`:

```go
type CheckoutExtRequest struct {
	// ... existing fields ...
	CouponCode      *string                `json:"coupon_code"`
	// ... existing fields ...
}
```

Add fields to `CheckoutExtResponse`:

```go
type CheckoutExtResponse struct {
	// ... existing fields ...
	DiscountTotal decimal.Decimal `json:"discount_total"`
	CouponCode    *string         `json:"coupon_code,omitempty"`
}
```

Add `CouponSvc` field to `CheckoutExtHandler`:

```go
type CheckoutExtHandler struct {
	db        *gorm.DB
	orderSvc  *order.Service
	couponSvc *coupon.Service
	logger    *slog.Logger
}

func NewCheckoutExtHandler(db *gorm.DB, orderSvc *order.Service, couponSvc *coupon.Service, logger *slog.Logger) *CheckoutExtHandler {
	return &CheckoutExtHandler{db: db, orderSvc: orderSvc, couponSvc: couponSvc, logger: logger}
}
```

In the `Checkout` method, after resolving the country config (Step 1) and before calculating shipping (Step 2), add:

```go
	// ── Step 1.5: Apply coupon discount ────────────────────────────────
	var couponDiscount decimal.Decimal
	var appliedCouponCode *string
	var freeShippingCoupon bool
	if req.CouponCode != nil && *req.CouponCode != "" {
		applier := coupon.NewCouponApplier(h.couponSvc, *req.CouponCode, req.CustomerEmail)
		// We run coupon validation here but defer the transactional apply
		// (usage increment + record) to inside the order creation tx below.
		validateResult, err := h.couponSvc.Validate(ctx, coupon.ValidateInput{
			StoreID:       storeID,
			Code:          *req.CouponCode,
			CustomerEmail: req.CustomerEmail,
			Subtotal:      req.Subtotal,
		})
		if err != nil {
			h.respondErr(c, err)
			return
		}
		couponDiscount = validateResult.DiscountAmount
		appliedCouponCode = &validateResult.Code
		freeShippingCoupon = validateResult.FreeShipping
		_ = applier // used inside the tx below
	}
```

Update shipping calculation to respect free_shipping coupon:

```go
	// After Step 2 (calculateShipping):
	if freeShippingCoupon {
		shippingTotal = decimal.Zero
	}
```

Update grandTotal calculation:

```go
	grandTotal := req.Subtotal.Add(shippingTotal).Add(taxBreakdown.TaxTotal).Sub(couponDiscount)
```

Inside the order creation transaction, after order is created but before payment, apply the coupon transactionally:

```go
	// After order creation, inside the same transaction scope:
	if appliedCouponCode != nil {
		applier := coupon.NewCouponApplier(h.couponSvc, *appliedCouponCode, req.CustomerEmail)
		if err := h.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
			_, applyErr := applier.Apply(ctx, tx, discount.ApplyInput{
				TenantID:      tenantID,
				StoreID:       storeID,
				OrderID:       result.Order.ID,
				CustomerEmail: req.CustomerEmail,
				Subtotal:      req.Subtotal,
				CurrencyCode:  store.CurrencyCode,
			})
			return applyErr
		}); err != nil {
			h.logWarn("checkout_ext: coupon apply failed (post-order)",
				"order_id", result.Order.ID.String(), "err", err)
			// Non-fatal: order was created. The discount was already
			// reflected in the grand total. Usage tracking failed.
		}
	}
```

Update all `CheckoutExtResponse` returns to include `DiscountTotal` and `CouponCode`:

```go
	c.JSON(http.StatusCreated, CheckoutExtResponse{
		// ... existing fields ...
		DiscountTotal: couponDiscount,
		CouponCode:    appliedCouponCode,
	})
```

### 8.5 — Commit

```
feat(marketplace-api): integrate coupon discount into extended checkout (M1)
```

---

## Task 9 — Wire Routes + main.go

### Steps

- [ ] **9.1** Add `CouponHandler` to admin `Deps` struct in `routes.go`
- [ ] **9.2** Mount coupon admin routes in `RegisterAdmin`
- [ ] **9.3** Add `CouponValidateHandler` to storefront `Deps` struct in `routes.go`
- [ ] **9.4** Mount coupon validate route with rate limiter in `RegisterStorefront`
- [ ] **9.5** Wire coupon repo, service, and handlers in `main.go`
- [ ] **9.6** Update `NewCheckoutExtHandler` call in `main.go` to pass coupon service
- [ ] **9.7** Run `go build ./cmd/marketplace-api/` to verify compilation
- [ ] **9.8** Commit

### 9.1 — Admin Deps

In `services/marketplace-api/internal/handlers/admin/routes.go`, add to `Deps`:

```go
type Deps struct {
	// ... existing fields ...
	CouponHandler            *CouponHandler
	// ... existing fields ...
}
```

### 9.2 — Admin routes

In `RegisterAdmin`, after the abandoned carts block (before the closing `}` of `storeRoute`), add:

```go
		// Coupons — Marketing M1.
		if deps.CouponHandler != nil {
			coupons := storeRoute.Group("/coupons")
			{
				coupons.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.CouponHandler.List)
				coupons.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.CouponHandler.Create)
				coupons.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.CouponHandler.Get)
				coupons.PATCH("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.CouponHandler.Patch)
				coupons.DELETE("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.CouponHandler.Delete)
			}
		}
```

### 9.3 — Storefront Deps

In `services/marketplace-api/internal/handlers/storefront/routes.go`, add to `Deps`:

```go
type Deps struct {
	// ... existing fields ...
	CouponValidateHandler *CouponValidateHandler
	// ... existing fields ...
}
```

### 9.4 — Storefront route

In `RegisterStorefront`, inside the `group` block (after the `/orders/:id` route), add:

```go
		// Coupon validation — Marketing M1.
		// Rate-limited: 10 req/min per IP.
		if deps.CouponValidateHandler != nil {
			group.POST("/coupons/validate",
				ratelimit.PerIP(0.167, 10), // ~10 req/min
				deps.CouponValidateHandler.Validate)
		}
```

Add the ratelimit import to the `routes.go` imports:

```go
	"github.com/mark8ly/marketplace-api/internal/ratelimit"
```

### 9.5 — main.go wiring

In `services/marketplace-api/cmd/marketplace-api/main.go`:

Add import:

```go
	"github.com/mark8ly/marketplace-api/internal/coupon"
```

In the admin wiring block (after the CSV import/export wiring), add:

```go
		// Coupon wiring (Marketing M1).
		couponRepo := coupon.NewRepository()
		couponSvc := coupon.NewService(coupon.ServiceConfig{
			DB:     conn,
			Repo:   couponRepo,
			Logger: log,
		})
		couponHandler := admin.NewCouponHandler(couponSvc, log)
```

Add to `adminDeps`:

```go
		adminDeps = admin.Deps{
			// ... existing fields ...
			CouponHandler:           couponHandler,
			// ... existing fields ...
		}
```

### 9.6 — Storefront wiring

In the storefront wiring block, add:

```go
		// Coupon storefront wiring (Marketing M1).
		couponRepoSF := coupon.NewRepository()
		couponSvcSF := coupon.NewService(coupon.ServiceConfig{
			DB:     conn,
			Repo:   couponRepoSF,
			Logger: log,
		})
		couponValidateHandler := storefront.NewCouponValidateHandler(couponSvcSF, log)
```

Update `NewCheckoutExtHandler` call to pass coupon service:

```go
		checkoutExtHandler := storefront.NewCheckoutExtHandler(conn, orderSvcSF, couponSvcSF, log)
```

Add to `storefrontDeps`:

```go
		storefrontDeps = storefront.Deps{
			// ... existing fields ...
			CouponValidateHandler: couponValidateHandler,
			// ... existing fields ...
		}
```

### 9.7 — Build verification

```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/
```

### 9.8 — Commit

```
feat(marketplace-api): wire coupon routes and dependencies in main.go (M1)
```

---

## Task 10 — Admin UI: API Client, List Page, Create Page, Detail Page

### Steps

- [ ] **10.1** Create API client at `apps/admin/lib/api/coupons-api.ts`
- [ ] **10.2** Create list page components: `CouponsListHeader.tsx`, `CouponsListFilters.tsx`, `CouponsList.tsx`, `CouponsListEmpty.tsx`
- [ ] **10.3** Create list page at `apps/admin/app/marketing/coupons/page.tsx`
- [ ] **10.4** Create form component at `apps/admin/components/marketing/coupons/CouponForm.tsx`
- [ ] **10.5** Create create page at `apps/admin/app/marketing/coupons/new/page.tsx`
- [ ] **10.6** Create detail components: `CouponDetailSummary.tsx`, `CouponUsageTable.tsx`
- [ ] **10.7** Create detail page at `apps/admin/app/marketing/coupons/[id]/page.tsx`
- [ ] **10.8** Run `next build` to verify TypeScript compilation
- [ ] **10.9** Commit

### 10.1 — API client

**File:** `apps/admin/lib/api/coupons-api.ts`

```typescript
// apps/admin/lib/api/coupons-api.ts
//
// Admin coupon API client. Follows the same calling convention as
// marketplace-api.ts: server components pass SessionHeaders, the client
// does the header rename dance.

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface SessionHeaders {
  userId: string;
  tenantId: string;
}

function authHeaders(session: SessionHeaders): Record<string, string> {
  return {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    "Content-Type": "application/json",
    Accept: "application/json",
  };
}

// ---------- Types ----------

export interface AdminCoupon {
  id: string;
  code: string;
  title: string;
  description: string | null;
  type: "percentage" | "fixed_amount" | "free_shipping";
  value: string;
  currency_code: string | null;
  min_purchase: string | null;
  max_discount: string | null;
  usage_limit: number | null;
  per_customer: number;
  target_type: "all" | "products" | "categories";
  target_ids: string[];
  stackable: boolean;
  starts_at: string;
  ends_at: string | null;
  status: "active" | "disabled" | "expired";
  usage_count: number;
  created_at: string;
  updated_at: string;
}

export interface CouponUsageRow {
  id: string;
  order_id: string;
  customer_email: string;
  discount_amount: string;
  currency_code: string;
  created_at: string;
}

export interface ListCouponsQuery {
  status?: string;
  search?: string;
  page?: number;
  per_page?: number;
}

export interface ListCouponsResponse {
  data: AdminCoupon[];
  total: number;
  page: number;
}

export interface GetCouponResponse {
  data: AdminCoupon;
  usage: CouponUsageRow[];
  usage_total: number;
}

export interface CreateCouponBody {
  code: string;
  title: string;
  description?: string;
  type: "percentage" | "fixed_amount" | "free_shipping";
  value: string;
  currency_code?: string;
  min_purchase?: string;
  max_discount?: string;
  usage_limit?: number;
  per_customer?: number;
  target_type?: string;
  target_ids?: string[];
  stackable?: boolean;
  starts_at?: string;
  ends_at?: string;
}

export interface PatchCouponBody {
  title?: string;
  description?: string;
  min_purchase?: string;
  max_discount?: string;
  usage_limit?: number;
  per_customer?: number;
  stackable?: boolean;
  starts_at?: string;
  ends_at?: string;
  status?: string;
}

// ---------- API functions ----------

export async function listCoupons(
  storeId: string,
  query: ListCouponsQuery,
  session: SessionHeaders,
): Promise<ListCouponsResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.search) params.set("search", query.search);
  if (query.page) params.set("page", String(query.page));
  if (query.per_page) params.set("per_page", String(query.per_page));

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/coupons?${params}`;
  try {
    const res = await fetch(url, { headers: authHeaders(session), cache: "no-store" });
    if (!res.ok) return null;
    return (await res.json()) as ListCouponsResponse;
  } catch {
    return null;
  }
}

export async function getCoupon(
  storeId: string,
  couponId: string,
  session: SessionHeaders,
): Promise<GetCouponResponse | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/coupons/${couponId}`;
  try {
    const res = await fetch(url, { headers: authHeaders(session), cache: "no-store" });
    if (!res.ok) return null;
    return (await res.json()) as GetCouponResponse;
  } catch {
    return null;
  }
}

export async function createCoupon(
  storeId: string,
  body: CreateCouponBody,
  session: SessionHeaders,
): Promise<{ data: AdminCoupon } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/coupons`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: authHeaders(session),
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminCoupon };
  } catch {
    return null;
  }
}

export async function patchCoupon(
  storeId: string,
  couponId: string,
  body: PatchCouponBody,
  session: SessionHeaders,
): Promise<{ data: AdminCoupon } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/coupons/${couponId}`;
  try {
    const res = await fetch(url, {
      method: "PATCH",
      headers: authHeaders(session),
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminCoupon };
  } catch {
    return null;
  }
}

export async function deleteCoupon(
  storeId: string,
  couponId: string,
  session: SessionHeaders,
): Promise<boolean> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/coupons/${couponId}`;
  try {
    const res = await fetch(url, {
      method: "DELETE",
      headers: authHeaders(session),
    });
    return res.ok;
  } catch {
    return false;
  }
}
```

### 10.2 — List page components

**File:** `apps/admin/components/marketing/coupons/CouponsListHeader.tsx`

```tsx
import Link from "next/link";

interface CouponsListHeaderProps {
  canCreate: boolean;
}

export function CouponsListHeader({ canCreate }: CouponsListHeaderProps) {
  return (
    <div className="flex items-center justify-between">
      <div>
        <h1 className="font-serif text-2xl font-semibold text-ink-900">
          Coupons
        </h1>
        <p className="mt-1 text-sm text-ink-500">
          Create and manage discount coupons for your store.
        </p>
      </div>
      {canCreate && (
        <Link
          href="/marketing/coupons/new"
          className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800"
        >
          Create coupon
        </Link>
      )}
    </div>
  );
}
```

**File:** `apps/admin/components/marketing/coupons/CouponsListFilters.tsx`

```tsx
"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useState } from "react";

export function CouponsListFilters() {
  const router = useRouter();
  const params = useSearchParams();

  const [search, setSearch] = useState(params.get("search") ?? "");
  const status = params.get("status") ?? "";

  const applyFilters = useCallback(
    (newStatus?: string, newSearch?: string) => {
      const sp = new URLSearchParams();
      const s = newStatus ?? status;
      const q = newSearch ?? search;
      if (s) sp.set("status", s);
      if (q) sp.set("search", q);
      router.push(`/marketing/coupons?${sp.toString()}`);
    },
    [router, status, search],
  );

  return (
    <div className="flex flex-wrap items-center gap-3">
      <input
        type="text"
        placeholder="Search by code or title..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") applyFilters(undefined, search);
        }}
        className="rounded-md border border-ink-200 bg-white px-3 py-1.5 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
      />
      <select
        value={status}
        onChange={(e) => applyFilters(e.target.value)}
        className="rounded-md border border-ink-200 bg-white px-3 py-1.5 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
      >
        <option value="">All statuses</option>
        <option value="active">Active</option>
        <option value="disabled">Disabled</option>
        <option value="expired">Expired</option>
      </select>
    </div>
  );
}
```

**File:** `apps/admin/components/marketing/coupons/CouponsList.tsx`

```tsx
import Link from "next/link";
import type { AdminCoupon } from "@/lib/api/coupons-api";

interface CouponsListProps {
  coupons: AdminCoupon[];
}

function formatType(type: AdminCoupon["type"]): string {
  switch (type) {
    case "percentage":
      return "Percentage";
    case "fixed_amount":
      return "Fixed amount";
    case "free_shipping":
      return "Free shipping";
    default:
      return type;
  }
}

function statusBadge(status: AdminCoupon["status"]) {
  const colors: Record<string, string> = {
    active: "bg-moss-50 text-moss-700",
    disabled: "bg-ink-100 text-ink-500",
    expired: "bg-signal-50 text-signal-700",
  };
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors[status] ?? "bg-ink-100 text-ink-500"}`}
    >
      {status}
    </span>
  );
}

export function CouponsList({ coupons }: CouponsListProps) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-ink-200 text-xs font-medium uppercase tracking-wider text-ink-500">
            <th className="pb-3 pr-4">Code</th>
            <th className="pb-3 pr-4">Title</th>
            <th className="pb-3 pr-4">Type</th>
            <th className="pb-3 pr-4">Value</th>
            <th className="pb-3 pr-4">Used</th>
            <th className="pb-3 pr-4">Status</th>
            <th className="pb-3 pr-4">Expires</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-ink-100">
          {coupons.map((c) => (
            <tr key={c.id} className="group">
              <td className="py-3 pr-4">
                <Link
                  href={`/marketing/coupons/${c.id}`}
                  className="font-mono text-sm font-medium text-moss-700 underline-offset-2 group-hover:underline"
                >
                  {c.code}
                </Link>
              </td>
              <td className="py-3 pr-4 text-ink-700">{c.title}</td>
              <td className="py-3 pr-4 text-ink-500">{formatType(c.type)}</td>
              <td className="py-3 pr-4 font-mono text-ink-700">
                {c.type === "percentage"
                  ? `${c.value}%`
                  : c.type === "free_shipping"
                    ? "--"
                    : `${c.currency_code ?? ""} ${c.value}`}
              </td>
              <td className="py-3 pr-4 text-ink-500">
                {c.usage_count}
                {c.usage_limit != null ? ` / ${c.usage_limit}` : ""}
              </td>
              <td className="py-3 pr-4">{statusBadge(c.status)}</td>
              <td className="py-3 pr-4 text-ink-500">
                {c.ends_at
                  ? new Date(c.ends_at).toLocaleDateString()
                  : "No expiry"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

**File:** `apps/admin/components/marketing/coupons/CouponsListEmpty.tsx`

```tsx
import Link from "next/link";

interface CouponsListEmptyProps {
  variant: "no-coupons" | "no-store";
}

export function CouponsListEmpty({ variant }: CouponsListEmptyProps) {
  if (variant === "no-store") {
    return (
      <div className="flex flex-col items-start gap-2 rounded-md border border-ink-200 bg-white px-6 py-12">
        <p className="text-sm text-ink-500">
          Create a store first to start managing coupons.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-start gap-4 rounded-md border border-ink-200 bg-white px-6 py-12">
      <h2 className="font-serif text-lg font-semibold text-ink-900">
        No coupons yet
      </h2>
      <p className="text-sm text-ink-500">
        Create your first coupon to offer discounts to your customers.
      </p>
      <Link
        href="/marketing/coupons/new"
        className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800"
      >
        Create coupon
      </Link>
    </div>
  );
}
```

### 10.3 — List page

**File:** `apps/admin/app/marketing/coupons/page.tsx`

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listCoupons, type ListCouponsQuery } from "@/lib/api/coupons-api";

import { CouponsListHeader } from "@/components/marketing/coupons/CouponsListHeader";
import { CouponsListFilters } from "@/components/marketing/coupons/CouponsListFilters";
import { CouponsList } from "@/components/marketing/coupons/CouponsList";
import { CouponsListEmpty } from "@/components/marketing/coupons/CouponsListEmpty";

interface CouponsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

function parseSearchParams(
  params: Record<string, string | string[] | undefined>,
): ListCouponsQuery {
  const first = (v: string | string[] | undefined): string | undefined =>
    Array.isArray(v) ? v[0] : v;
  return {
    status: first(params.status),
    search: first(params.search),
    page: first(params.page) ? Number(first(params.page)) : undefined,
    per_page: first(params.per_page) ? Number(first(params.per_page)) : undefined,
  };
}

export default async function CouponsPage({ searchParams }: CouponsPageProps) {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;
  const params = await searchParams;
  const query = parseSearchParams(params);
  const canCreate = role === "owner" || role === "admin";

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="flex flex-col gap-6 px-8 py-6">
          <CouponsListHeader canCreate={false} />
          <CouponsListEmpty variant="no-store" />
        </main>
      </AdminShell>
    );
  }

  const response = await listCoupons(currentStore.id, query, {
    userId,
    tenantId,
  });
  const coupons = response?.data ?? [];

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6">
        <CouponsListHeader canCreate={canCreate} />
        <CouponsListFilters />
        <hr className="border-ink-200" />
        {coupons.length === 0 ? (
          <CouponsListEmpty variant="no-coupons" />
        ) : (
          <CouponsList coupons={coupons} />
        )}
      </main>
    </AdminShell>
  );
}
```

### 10.4 — Coupon form (progressive disclosure)

**File:** `apps/admin/components/marketing/coupons/CouponForm.tsx`

```tsx
"use client";

import { useCallback, useState } from "react";
import { useRouter } from "next/navigation";
import type { CreateCouponBody } from "@/lib/api/coupons-api";

interface CouponFormProps {
  storeId: string;
  storeCurrency: string;
  onSubmit: (body: CreateCouponBody) => Promise<boolean>;
}

export function CouponForm({
  storeId,
  storeCurrency,
  onSubmit,
}: CouponFormProps) {
  const router = useRouter();
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Always-visible fields
  const [code, setCode] = useState("");
  const [type, setType] = useState<CreateCouponBody["type"]>("percentage");
  const [value, setValue] = useState("");
  const [endsAt, setEndsAt] = useState("");

  // Advanced fields
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [minPurchase, setMinPurchase] = useState("");
  const [maxDiscount, setMaxDiscount] = useState("");
  const [usageLimit, setUsageLimit] = useState("");
  const [perCustomer, setPerCustomer] = useState("1");
  const [stackable, setStackable] = useState(false);
  const [startsAt, setStartsAt] = useState("");

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setError(null);
      setSubmitting(true);

      const body: CreateCouponBody = {
        code: code.trim().toUpperCase(),
        title: title.trim() || code.trim().toUpperCase(),
        type,
        value,
      };

      if (description.trim()) body.description = description.trim();
      if (type === "fixed_amount") body.currency_code = storeCurrency;
      if (endsAt) body.ends_at = new Date(endsAt).toISOString();
      if (startsAt) body.starts_at = new Date(startsAt).toISOString();
      if (minPurchase) body.min_purchase = minPurchase;
      if (maxDiscount) body.max_discount = maxDiscount;
      if (usageLimit) body.usage_limit = Number(usageLimit);
      if (perCustomer) body.per_customer = Number(perCustomer);
      body.stackable = stackable;

      try {
        const ok = await onSubmit(body);
        if (ok) {
          router.push("/marketing/coupons");
        } else {
          setError("Failed to create coupon. Please check your input.");
        }
      } catch {
        setError("An unexpected error occurred.");
      } finally {
        setSubmitting(false);
      }
    },
    [
      code, type, value, endsAt, title, description, minPurchase,
      maxDiscount, usageLimit, perCustomer, stackable, startsAt,
      storeCurrency, onSubmit, router,
    ],
  );

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {error && (
        <div className="rounded-md border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
          {error}
        </div>
      )}

      {/* Always visible: code, type, value, expiry */}
      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <label className="mb-1 block text-sm font-medium text-ink-700">
            Coupon code
          </label>
          <input
            type="text"
            required
            maxLength={50}
            value={code}
            onChange={(e) => setCode(e.target.value)}
            placeholder="e.g. SAVE20"
            className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm font-mono uppercase text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-ink-700">
            Discount type
          </label>
          <select
            value={type}
            onChange={(e) =>
              setType(e.target.value as CreateCouponBody["type"])
            }
            className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          >
            <option value="percentage">Percentage</option>
            <option value="fixed_amount">Fixed amount</option>
            <option value="free_shipping">Free shipping</option>
          </select>
        </div>
        {type !== "free_shipping" && (
          <div>
            <label className="mb-1 block text-sm font-medium text-ink-700">
              {type === "percentage" ? "Discount (%)" : `Amount (${storeCurrency})`}
            </label>
            <input
              type="number"
              required
              min="0"
              max={type === "percentage" ? "100" : undefined}
              step="0.01"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
        )}
        <div>
          <label className="mb-1 block text-sm font-medium text-ink-700">
            Expiry date
          </label>
          <input
            type="datetime-local"
            value={endsAt}
            onChange={(e) => setEndsAt(e.target.value)}
            className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
          <p className="mt-1 text-xs text-ink-400">Leave empty for no expiry</p>
        </div>
      </div>

      {/* Advanced options toggle */}
      <button
        type="button"
        onClick={() => setShowAdvanced((v) => !v)}
        className="text-sm font-medium text-moss-700 underline-offset-2 hover:underline"
      >
        {showAdvanced ? "Hide advanced options" : "Advanced options"}
      </button>

      {showAdvanced && (
        <div className="grid gap-4 border-t border-ink-100 pt-4 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Title (internal label)
            </label>
            <input
              type="text"
              maxLength={200}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="e.g. Spring sale 20% off"
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
            <p className="mt-1 text-xs text-ink-400">
              Defaults to the coupon code if left empty
            </p>
          </div>
          <div className="sm:col-span-2">
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Description
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Minimum purchase ({storeCurrency})
            </label>
            <input
              type="number"
              min="0"
              step="0.01"
              value={minPurchase}
              onChange={(e) => setMinPurchase(e.target.value)}
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          {type === "percentage" && (
            <div>
              <label className="mb-1 block text-sm font-medium text-ink-700">
                Maximum discount ({storeCurrency})
              </label>
              <input
                type="number"
                min="0"
                step="0.01"
                value={maxDiscount}
                onChange={(e) => setMaxDiscount(e.target.value)}
                className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
              />
            </div>
          )}
          <div>
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Total usage limit
            </label>
            <input
              type="number"
              min="1"
              value={usageLimit}
              onChange={(e) => setUsageLimit(e.target.value)}
              placeholder="Unlimited"
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Uses per customer
            </label>
            <input
              type="number"
              min="1"
              value={perCustomer}
              onChange={(e) => setPerCustomer(e.target.value)}
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Start date
            </label>
            <input
              type="datetime-local"
              value={startsAt}
              onChange={(e) => setStartsAt(e.target.value)}
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          <div className="flex items-center gap-2 pt-6">
            <input
              type="checkbox"
              id="stackable"
              checked={stackable}
              onChange={(e) => setStackable(e.target.checked)}
              className="h-4 w-4 rounded border-ink-300 text-moss-700 focus:ring-moss-700"
            />
            <label htmlFor="stackable" className="text-sm text-ink-700">
              Allow stacking with other coupons
            </label>
          </div>
        </div>
      )}

      <hr className="border-ink-200" />

      <div className="flex items-center gap-3">
        <button
          type="submit"
          disabled={submitting}
          className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
        >
          {submitting ? "Creating..." : "Create coupon"}
        </button>
        <button
          type="button"
          onClick={() => router.push("/marketing/coupons")}
          className="rounded-md px-4 py-2 text-sm font-medium text-ink-500 transition hover:text-ink-700"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
```

### 10.5 — Create page

**File:** `apps/admin/app/marketing/coupons/new/page.tsx`

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { CouponForm } from "@/components/marketing/coupons/CouponForm";
import { createCoupon, type CreateCouponBody } from "@/lib/api/coupons-api";
import { redirect } from "next/navigation";

export default async function NewCouponPage() {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;

  if (!currentStore) {
    redirect("/marketing/coupons");
  }

  if (role !== "owner" && role !== "admin") {
    redirect("/marketing/coupons");
  }

  async function handleSubmit(body: CreateCouponBody): Promise<boolean> {
    "use server";
    const result = await createCoupon(currentStore!.id, body, {
      userId,
      tenantId,
    });
    return result !== null;
  }

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-2xl px-8 py-6">
        <h1 className="mb-6 font-serif text-2xl font-semibold text-ink-900">
          Create coupon
        </h1>
        <CouponForm
          storeId={currentStore.id}
          storeCurrency={currentStore.currency_code ?? "USD"}
          onSubmit={handleSubmit}
        />
      </main>
    </AdminShell>
  );
}
```

### 10.6 — Detail components

**File:** `apps/admin/components/marketing/coupons/CouponDetailSummary.tsx`

```tsx
import type { AdminCoupon } from "@/lib/api/coupons-api";

interface CouponDetailSummaryProps {
  coupon: AdminCoupon;
}

function formatType(type: AdminCoupon["type"]): string {
  switch (type) {
    case "percentage":
      return "Percentage";
    case "fixed_amount":
      return "Fixed amount";
    case "free_shipping":
      return "Free shipping";
    default:
      return type;
  }
}

function formatValue(coupon: AdminCoupon): string {
  if (coupon.type === "percentage") return `${coupon.value}%`;
  if (coupon.type === "free_shipping") return "Free shipping";
  return `${coupon.currency_code ?? ""} ${coupon.value}`;
}

export function CouponDetailSummary({ coupon }: CouponDetailSummaryProps) {
  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <DetailField label="Code" value={coupon.code} mono />
        <DetailField label="Type" value={formatType(coupon.type)} />
        <DetailField label="Value" value={formatValue(coupon)} mono />
        <DetailField label="Status" value={coupon.status} />
        <DetailField
          label="Used"
          value={
            coupon.usage_limit != null
              ? `${coupon.usage_count} / ${coupon.usage_limit}`
              : `${coupon.usage_count}`
          }
        />
        <DetailField label="Per customer" value={String(coupon.per_customer)} />
        <DetailField
          label="Starts"
          value={new Date(coupon.starts_at).toLocaleString()}
        />
        <DetailField
          label="Expires"
          value={
            coupon.ends_at
              ? new Date(coupon.ends_at).toLocaleString()
              : "No expiry"
          }
        />
        <DetailField
          label="Stackable"
          value={coupon.stackable ? "Yes" : "No"}
        />
        {coupon.min_purchase && (
          <DetailField
            label="Min purchase"
            value={`${coupon.currency_code ?? ""} ${coupon.min_purchase}`}
            mono
          />
        )}
        {coupon.max_discount && (
          <DetailField
            label="Max discount"
            value={`${coupon.currency_code ?? ""} ${coupon.max_discount}`}
            mono
          />
        )}
      </div>
      {coupon.description && (
        <div>
          <span className="text-xs font-medium uppercase tracking-wider text-ink-400">
            Description
          </span>
          <p className="mt-1 text-sm text-ink-700">{coupon.description}</p>
        </div>
      )}
    </div>
  );
}

function DetailField({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div>
      <span className="text-xs font-medium uppercase tracking-wider text-ink-400">
        {label}
      </span>
      <p className={`mt-1 text-sm text-ink-900 ${mono ? "font-mono" : ""}`}>
        {value}
      </p>
    </div>
  );
}
```

**File:** `apps/admin/components/marketing/coupons/CouponUsageTable.tsx`

```tsx
import type { CouponUsageRow } from "@/lib/api/coupons-api";

interface CouponUsageTableProps {
  usages: CouponUsageRow[];
  total: number;
}

export function CouponUsageTable({ usages, total }: CouponUsageTableProps) {
  if (usages.length === 0) {
    return (
      <p className="py-6 text-sm text-ink-400">
        No usage records yet. This coupon has not been redeemed.
      </p>
    );
  }

  return (
    <div>
      <p className="mb-3 text-xs text-ink-400">
        {total} total redemption{total !== 1 ? "s" : ""}
      </p>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-ink-200 text-xs font-medium uppercase tracking-wider text-ink-500">
              <th className="pb-3 pr-4">Customer</th>
              <th className="pb-3 pr-4">Discount</th>
              <th className="pb-3 pr-4">Order</th>
              <th className="pb-3 pr-4">Date</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-ink-100">
            {usages.map((u) => (
              <tr key={u.id}>
                <td className="py-2 pr-4 text-ink-700">{u.customer_email}</td>
                <td className="py-2 pr-4 font-mono text-ink-700">
                  {u.currency_code} {u.discount_amount}
                </td>
                <td className="py-2 pr-4 font-mono text-xs text-ink-500">
                  {u.order_id.slice(0, 8)}...
                </td>
                <td className="py-2 pr-4 text-ink-500">
                  {new Date(u.created_at).toLocaleString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
```

### 10.7 — Detail page

**File:** `apps/admin/app/marketing/coupons/[id]/page.tsx`

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getCoupon } from "@/lib/api/coupons-api";
import { CouponDetailSummary } from "@/components/marketing/coupons/CouponDetailSummary";
import { CouponUsageTable } from "@/components/marketing/coupons/CouponUsageTable";
import Link from "next/link";
import { notFound } from "next/navigation";

interface CouponDetailPageProps {
  params: Promise<{ id: string }>;
}

export default async function CouponDetailPage({
  params,
}: CouponDetailPageProps) {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;
  const { id } = await params;

  if (!currentStore) {
    notFound();
  }

  const response = await getCoupon(currentStore.id, id, { userId, tenantId });
  if (!response) {
    notFound();
  }

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-3xl px-8 py-6">
        <div className="mb-6 flex items-center gap-3">
          <Link
            href="/marketing/coupons"
            className="text-sm text-ink-500 hover:text-ink-700"
          >
            Coupons
          </Link>
          <span className="text-ink-300">/</span>
          <h1 className="font-serif text-2xl font-semibold text-ink-900">
            {response.data.code}
          </h1>
        </div>

        <CouponDetailSummary coupon={response.data} />

        <hr className="my-8 border-ink-200" />

        <h2 className="mb-4 font-serif text-lg font-semibold text-ink-900">
          Usage history
        </h2>
        <CouponUsageTable
          usages={response.usage ?? []}
          total={response.usage_total ?? 0}
        />
      </main>
    </AdminShell>
  );
}
```

### 10.8 — Build verification

```bash
cd apps/admin && npx next build
```

### 10.9 — Commit

```
feat(admin): add coupon list, create, and detail pages with progressive disclosure (M1)
```

---

## Task 11 — Storefront: Coupon Input in Checkout

### Steps

- [ ] **11.1** Create `apps/storefront/components/checkout/CouponInput.tsx`
- [ ] **11.2** Update `apps/storefront/lib/api/checkout-api.ts` to add `coupon_code` to `CheckoutBody` and `discount_total` to `CheckoutResult`
- [ ] **11.3** Integrate `CouponInput` into `apps/storefront/app/checkout/page.tsx`
- [ ] **11.4** Run `next build` to verify TypeScript compilation
- [ ] **11.5** Commit

### 11.1 — CouponInput component

**File:** `apps/storefront/components/checkout/CouponInput.tsx`

```tsx
"use client";

import { useCallback, useState } from "react";

interface CouponInputProps {
  storeSlug: string;
  customerEmail: string;
  subtotal: number;
  currencyCode: string;
  onApplied: (result: CouponValidateResult) => void;
  onRemoved: () => void;
}

interface CouponValidateResult {
  coupon_id: string;
  code: string;
  type: string;
  value: string;
  discount_amount: string;
  free_shipping: boolean;
  title: string;
}

const MARKETPLACE_API_URL =
  process.env.NEXT_PUBLIC_MARKETPLACE_API_URL ??
  process.env.MARKETPLACE_API_URL ??
  "http://localhost:8088";

const STOREFRONT_KEY = process.env.NEXT_PUBLIC_STOREFRONT_KEY ?? "";

export function CouponInput({
  storeSlug,
  customerEmail,
  subtotal,
  currencyCode,
  onApplied,
  onRemoved,
}: CouponInputProps) {
  const [open, setOpen] = useState(false);
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [applied, setApplied] = useState<CouponValidateResult | null>(null);

  const validate = useCallback(async () => {
    if (!code.trim()) return;
    setLoading(true);
    setError(null);

    try {
      const headers: Record<string, string> = {
        "Content-Type": "application/json",
        Accept: "application/json",
      };
      if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;

      const res = await fetch(
        `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/coupons/validate`,
        {
          method: "POST",
          headers,
          body: JSON.stringify({
            code: code.trim(),
            customer_email: customerEmail,
            subtotal: subtotal.toFixed(2),
          }),
        },
      );

      if (!res.ok) {
        const body = await res.json().catch(() => null);
        setError(body?.message ?? "Invalid coupon code");
        return;
      }

      const body = await res.json();
      const result = body.data as CouponValidateResult;
      setApplied(result);
      onApplied(result);
    } catch {
      setError("Failed to validate coupon");
    } finally {
      setLoading(false);
    }
  }, [code, storeSlug, customerEmail, subtotal, onApplied]);

  const remove = useCallback(() => {
    setApplied(null);
    setCode("");
    setError(null);
    onRemoved();
  }, [onRemoved]);

  if (!open && !applied) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="text-sm text-moss-700 underline-offset-2 hover:underline"
      >
        Have a promo code?
      </button>
    );
  }

  if (applied) {
    return (
      <div className="flex items-center justify-between rounded-md border border-moss-200 bg-moss-50 px-3 py-2">
        <div className="text-sm">
          <span className="font-mono font-medium text-moss-700">
            {applied.code}
          </span>
          <span className="ml-2 text-ink-500">
            {applied.free_shipping
              ? "Free shipping"
              : `-${currencyCode} ${applied.discount_amount}`}
          </span>
        </div>
        <button
          type="button"
          onClick={remove}
          className="text-xs text-ink-400 hover:text-ink-600"
        >
          Remove
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <div className="flex gap-2">
        <input
          type="text"
          value={code}
          onChange={(e) => setCode(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              validate();
            }
          }}
          placeholder="Enter promo code"
          className="flex-1 rounded-md border border-ink-200 bg-white px-3 py-2 text-sm font-mono uppercase text-ink-900 placeholder:text-ink-400 placeholder:normal-case focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
        />
        <button
          type="button"
          onClick={validate}
          disabled={loading || !code.trim()}
          className="rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
        >
          {loading ? "..." : "Apply"}
        </button>
      </div>
      {error && <p className="text-xs text-signal-700">{error}</p>}
    </div>
  );
}
```

### 11.2 — Update checkout-api.ts

In `apps/storefront/lib/api/checkout-api.ts`, add `coupon_code` to `CheckoutBody`:

```typescript
export interface CheckoutBody {
  // ... existing fields ...
  coupon_code?: string;
}
```

Add `discount_total` and `coupon_code` to `CheckoutResult`:

```typescript
export interface CheckoutResult {
  // ... existing fields ...
  discount_total?: string;
  coupon_code?: string;
}
```

### 11.3 — Integrate CouponInput into checkout page

In `apps/storefront/app/checkout/page.tsx`, add to the imports:

```typescript
import { CouponInput } from "@/components/checkout/CouponInput";
```

In the component body, add state:

```typescript
  const [couponCode, setCouponCode] = useState<string | null>(null);
  const [couponDiscount, setCouponDiscount] = useState(0);
  const [couponFreeShipping, setCouponFreeShipping] = useState(false);
```

Update the total calculation:

```typescript
  const total = subtotal + (couponFreeShipping ? 0 : shippingTotal) - couponDiscount;
```

In the JSX, add the CouponInput component below the order summary and above the totals section. Place it after the items list:

```tsx
        {/* Coupon input — below order summary */}
        <CouponInput
          storeSlug={storeSlug}
          customerEmail={email}
          subtotal={subtotal}
          currencyCode={currencyCode}
          onApplied={(result) => {
            setCouponCode(result.code);
            setCouponDiscount(Number.parseFloat(result.discount_amount));
            setCouponFreeShipping(result.free_shipping);
          }}
          onRemoved={() => {
            setCouponCode(null);
            setCouponDiscount(0);
            setCouponFreeShipping(false);
          }}
        />
```

In the submitCheckout call, include coupon_code:

```typescript
      coupon_code: couponCode ?? undefined,
```

In the totals section, add a discount line when couponDiscount > 0:

```tsx
        {couponDiscount > 0 && (
          <div className="flex justify-between text-sm">
            <span className="text-ink-500">Discount</span>
            <span className="font-mono text-moss-700">
              -{formatPrice(couponDiscount, currencyCode)}
            </span>
          </div>
        )}
        {couponFreeShipping && (
          <div className="flex justify-between text-sm">
            <span className="text-ink-500">Shipping</span>
            <span className="font-mono text-moss-700">Free</span>
          </div>
        )}
```

### 11.4 — Build verification

```bash
cd apps/storefront && npx next build
```

### 11.5 — Commit

```
feat(storefront): add coupon input to checkout with inline validation (M1)
```

---

## Task 12 — Build Verification + Final Commit

### Steps

- [ ] **12.1** Run Go tests across all changed packages
- [ ] **12.2** Run Go build for the full binary
- [ ] **12.3** Run admin Next.js build
- [ ] **12.4** Run storefront Next.js build
- [ ] **12.5** Final commit (if any remaining changes)

### 12.1 — Go tests

```bash
cd services/marketplace-api && go test ./internal/coupon/ ./internal/ratelimit/ ./internal/handlers/admin/ ./internal/handlers/storefront/ ./pkg/apperrors/ -v -count=1
```

### 12.2 — Go build

```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/
```

### 12.3 — Admin build

```bash
cd apps/admin && npx next build
```

### 12.4 — Storefront build

```bash
cd apps/storefront && npx next build
```

### 12.5 — Final commit

If any build fixes were needed:

```
fix(marketplace-api): resolve build issues from M1 coupon integration
```
