# Payments P1 — Foundation, Data Model & Supported Countries

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the schema migration (12 new tables + 15-country seed), the three provider abstraction interfaces (payment gateway, shipping carrier, tax calculator), the `GET /api/v1/public/supported-countries` endpoint, and the onboarding country picker filter — so P2/P3/P4 can build on a stable foundation without touching migrations or interfaces.

**Architecture:** Single migration `000007_payments_shipping_tax.{up,down}.sql` creates all tables and seeds the 15 supported countries. Three Go interface files define the provider contracts. A thin `internal/country/` package wraps the `supported_countries` table with a repository + public HTTP handler. The onboarding app (`apps/onboarding`) replaces its location-service country fetch with a call to the new endpoint.

**Tech Stack:** Go 1.26, GORM, Postgres 15, existing `marketplace-api` patterns. Next.js 16 for the onboarding change. No new external dependencies.

**Spec reference:** `docs/superpowers/specs/2026-04-10-payments-shipping-tax-design.md` — sections §2, §3, §4.1, §10, §11.

**Prerequisite:** Orders M1–M5 on main (provides the `orders`, `stores`, `products` tables that the new tables reference).

---

## File structure produced by P1

```
services/marketplace-api/
├── migrations/
│   ├── 000007_payments_shipping_tax.up.sql     # NEW — 12 tables + seed
│   └── 000007_payments_shipping_tax.down.sql   # NEW
├── internal/
│   ├── payment/
│   │   └── gateway.go                          # NEW — Gateway interface + types
│   ├── shipping/
│   │   └── carrier.go                          # NEW — Carrier interface + types
│   ├── tax/
│   │   └── calculator.go                       # NEW — Calculator interface + types
│   └── country/
│       ├── models.go                           # NEW — SupportedCountry GORM model
│       ├── repository.go                       # NEW — ListActive, GetByCode
│       ├── handler.go                          # NEW — public HTTP handler
│       └── handler_integration_test.go         # NEW
├── internal/handlers/storefront/
│   └── routes.go                               # MODIFY — register /public/supported-countries
└── cmd/marketplace-api/main.go                 # MODIFY — wire country handler

apps/onboarding/
└── lib/api/countries.ts                        # NEW or MODIFY — call marketplace-api endpoint
```

---

## Task 0: Verify prerequisites

**Files:** none (read-only)

- [ ] **Step 1: Verify orders tables exist**

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "SELECT count(*) FROM information_schema.tables WHERE table_name IN ('orders','stores','products');"
```
Expected: `3`. If less, migrations haven't run — run `DATABASE_URL=... go run ./cmd/migrate up`.

- [ ] **Step 2: Verify no migration 000007 exists yet**

```bash
ls services/marketplace-api/migrations/000007_*
```
Expected: no matches. If a file exists, the parallel agent may have shipped something — investigate before proceeding.

- [ ] **Step 3: Verify current migration version**

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "SELECT version FROM marketplace_db_schema_migrations ORDER BY version DESC LIMIT 1;"
```
Expected: `6` (products M3 media migration was 000005, orders watermark was 000006). If higher, a new migration landed — adjust the filename number.

No commit. Task 0 is read-only.

---

## Task 1: Migration — 12 new tables + 15-country seed

**Files:**
- Create: `services/marketplace-api/migrations/000007_payments_shipping_tax.up.sql`
- Create: `services/marketplace-api/migrations/000007_payments_shipping_tax.down.sql`

- [ ] **Step 1: Write the up migration**

Create `services/marketplace-api/migrations/000007_payments_shipping_tax.up.sql` with the full SQL from spec §4.1:
- `supported_countries` with seed INSERT for all 15 countries
- `payment_gateway_configs` (per-store, UNIQUE on store_id+provider)
- `shipping_carrier_configs` (per-store, UNIQUE on store_id+provider, includes warehouse address + handling_fee + free_shipping_min)
- `payment_transactions` + index on order_id
- `refund_transactions`
- `webhook_events` (UNIQUE on provider+provider_event_id for idempotency)
- `shipments` + index on order_id
- `platform_fee_configs` (UNIQUE on store_id)
- `platform_fee_ledger` + indexes on order_id and (store_id, created_at)
- `order_tax_lines` + index on order_id
- `tax_provider_configs` (UNIQUE on store_id+provider)
- ALTER TABLE products ADD COLUMN hsn_code, gst_rate
- ALTER TABLE supported_countries ADD COLUMN tax_rate, tax_strategy (if not in CREATE)

Use `BEGIN; ... COMMIT;` transaction wrapper. Use `set_updated_at()` trigger on tables that have `updated_at` (reuse the shared trigger function from 000001).

- [ ] **Step 2: Write the down migration**

Reverse order: DROP TABLEs + ALTER TABLE products DROP COLUMN.

- [ ] **Step 3: Run the migration**

```bash
cd services/marketplace-api
DATABASE_URL=postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable go run ./cmd/migrate up
```
Expected: `migrations applied`.

- [ ] **Step 4: Verify tables + seed data**

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "SELECT count(*) FROM supported_countries;"
```
Expected: `15`.

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "SELECT country_code, payment_providers, shipping_carriers, tax_strategy FROM supported_countries ORDER BY country_code LIMIT 5;"
```
Expected: shows AU, CA, DE, ES, FR with correct provider arrays.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/migrations/000007_payments_shipping_tax.up.sql \
       services/marketplace-api/migrations/000007_payments_shipping_tax.down.sql
git commit -m "feat(marketplace-api): migration 000007 — payments/shipping/tax tables + 15-country seed"
```

---

## Task 2: Provider interfaces — payment, shipping, tax

**Files:**
- Create: `services/marketplace-api/internal/payment/gateway.go`
- Create: `services/marketplace-api/internal/shipping/carrier.go`
- Create: `services/marketplace-api/internal/tax/calculator.go`

- [ ] **Step 1: Write the payment gateway interface**

`internal/payment/gateway.go`:

```go
package payment

import (
    "context"
    "github.com/shopspring/decimal"
)

type Gateway interface {
    CreateIntent(ctx context.Context, in CreateIntentInput) (*Intent, error)
    CapturePayment(ctx context.Context, captureID string) (*Capture, error)
    RefundPayment(ctx context.Context, in RefundInput) (*Refund, error)
    VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error)
    ProviderName() string
    SupportedCountries() []string
}

type CreateIntentInput struct {
    OrderID       string
    Amount        decimal.Decimal
    CurrencyCode  string
    CustomerEmail string
    Description   string
    Metadata      map[string]string
}

type Intent struct {
    ProviderIntentID string
    ClientToken      string   // Stripe client_secret, Razorpay order_id, PayPal approval_url
    Status           string
}

type Capture struct {
    ProviderPaymentID string
    Status            string
    PaymentMethod     string
}

type RefundInput struct {
    ProviderPaymentID string
    Amount            decimal.Decimal
    Reason            string
}

type Refund struct {
    ProviderRefundID string
    Status           string
    Amount           decimal.Decimal
}

type WebhookEvent struct {
    ProviderEventID string
    EventType       string   // "payment.succeeded", "payment.failed", "refund.succeeded"
    OrderID         string
    Amount          decimal.Decimal
    CurrencyCode    string
    PaymentMethod   string
    RawPayload      []byte
}
```

- [ ] **Step 2: Write the shipping carrier interface**

`internal/shipping/carrier.go`:

```go
package shipping

import (
    "context"
    "time"
    "github.com/shopspring/decimal"
)

type Carrier interface {
    GetRates(ctx context.Context, in RateRequest) ([]Rate, error)
    CreateShipment(ctx context.Context, in ShipmentRequest) (*Shipment, error)
    GetTracking(ctx context.Context, trackingNumber string) (*Tracking, error)
    CancelShipment(ctx context.Context, shipmentID string) error
    ProviderName() string
    SupportedCountries() []string
}

type Address struct {
    Name        string
    Line1       string
    Line2       string
    City        string
    Region      string
    PostalCode  string
    CountryCode string
    Phone       string
}

type ParcelItem struct {
    Title      string
    SKU        string
    Quantity   int
    WeightGrams int
}

type RateRequest struct {
    FromAddress Address
    ToAddress   Address
    Items       []ParcelItem
    CurrencyCode string
}

type Rate struct {
    Service       string          // e.g. "ground", "express", "overnight"
    Carrier       string
    Price         decimal.Decimal
    CurrencyCode  string
    EstimatedDays int
}

type ShipmentRequest struct {
    OrderID      string
    FromAddress  Address
    ToAddress    Address
    Items        []ParcelItem
    Service      string
    CurrencyCode string
}

type Shipment struct {
    ProviderShipmentID string
    TrackingNumber     string
    LabelURL           string
    Carrier            string
    Service            string
    EstimatedDelivery  *time.Time
}

type Tracking struct {
    TrackingNumber string
    Status         string // "in_transit","delivered","exception"
    Events         []TrackingEvent
    EstimatedDelivery *time.Time
}

type TrackingEvent struct {
    Status      string
    Description string
    Location    string
    Timestamp   time.Time
}
```

- [ ] **Step 3: Write the tax calculator interface**

`internal/tax/calculator.go`:

```go
package tax

import (
    "context"
    "github.com/shopspring/decimal"
)

type Calculator interface {
    Calculate(ctx context.Context, in TaxRequest) (*TaxBreakdown, error)
    ProviderName() string
}

type TaxRequest struct {
    StoreCountryCode    string
    SellerAddress       Address
    BuyerAddress        Address
    Items               []TaxableItem
    ShippingAmount      decimal.Decimal
    CurrencyCode        string
}

type Address struct {
    Line1       string
    City        string
    Region      string // state/province
    PostalCode  string
    CountryCode string
}

type TaxableItem struct {
    ProductID    string
    SKU          string
    Amount       decimal.Decimal
    Quantity     int
    HSNCode      string          // India GST — empty for other strategies
    GSTRate      decimal.Decimal // India GST — zero for other strategies
    TaxCode      string          // TaxJar product tax code — empty for non-US
}

type TaxBreakdown struct {
    TaxTotal     decimal.Decimal
    Lines        []TaxLine
}

type TaxLine struct {
    Description  string          // "VAT 20%", "CGST 9%", "CA State Tax"
    Rate         decimal.Decimal
    Amount       decimal.Decimal
    Jurisdiction string          // "Maharashtra", "CA-Los Angeles", "" for flat
}
```

- [ ] **Step 4: Build all three packages**

```bash
cd services/marketplace-api
go build ./internal/payment/... ./internal/shipping/... ./internal/tax/...
```
Expected: exits 0.

- [ ] **Step 5: Commit**

```bash
git add internal/payment/gateway.go internal/shipping/carrier.go internal/tax/calculator.go
git commit -m "feat(marketplace-api): payment/shipping/tax provider abstraction interfaces"
```

---

## Task 3: Supported countries — model + repository + handler

**Files:**
- Create: `services/marketplace-api/internal/country/models.go`
- Create: `services/marketplace-api/internal/country/repository.go`
- Create: `services/marketplace-api/internal/country/handler.go`

- [ ] **Step 1: Write the GORM model**

`internal/country/models.go`:

```go
package country

import "github.com/lib/pq"

type SupportedCountry struct {
    CountryCode      string         `gorm:"column:country_code;primaryKey;type:char(2)"         json:"country_code"`
    Name             string         `gorm:"column:name;type:varchar(100);not null"               json:"name"`
    CurrencyCode     string         `gorm:"column:currency_code;type:char(3);not null"           json:"currency_code"`
    Region           string         `gorm:"column:region;type:varchar(20);not null"               json:"region"`
    PaymentProviders pq.StringArray `gorm:"column:payment_providers;type:text[];not null"         json:"payment_providers"`
    ShippingCarriers pq.StringArray `gorm:"column:shipping_carriers;type:text[];not null"         json:"shipping_carriers"`
    TaxStrategy      string         `gorm:"column:tax_strategy;type:varchar(20);not null"         json:"tax_strategy"`
    TaxRate          *float64       `gorm:"column:tax_rate;type:numeric(5,2)"                     json:"tax_rate,omitempty"`
    IsActive         bool           `gorm:"column:is_active;not null;default:true"                json:"-"`
}

func (SupportedCountry) TableName() string { return "supported_countries" }
```

- [ ] **Step 2: Write the repository**

`internal/country/repository.go`:

```go
package country

import (
    "context"
    "gorm.io/gorm"
)

type Repository interface {
    ListActive(ctx context.Context) ([]SupportedCountry, error)
    GetByCode(ctx context.Context, code string) (*SupportedCountry, error)
}

type gormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) ListActive(ctx context.Context) ([]SupportedCountry, error) {
    var rows []SupportedCountry
    err := r.db.WithContext(ctx).
        Where("is_active = true").
        Order("name").
        Find(&rows).Error
    return rows, err
}

func (r *gormRepository) GetByCode(ctx context.Context, code string) (*SupportedCountry, error) {
    var c SupportedCountry
    err := r.db.WithContext(ctx).
        Where("country_code = ? AND is_active = true", code).
        First(&c).Error
    if err != nil {
        return nil, err
    }
    return &c, nil
}
```

- [ ] **Step 3: Write the public HTTP handler**

`internal/country/handler.go`:

```go
package country

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

type Handler struct {
    repo Repository
}

func NewHandler(repo Repository) *Handler {
    return &Handler{repo: repo}
}

// ListSupported handles GET /api/v1/public/supported-countries.
// No auth — public reference data for onboarding + storefront.
func (h *Handler) ListSupported(c *gin.Context) {
    rows, err := h.repo.ListActive(c.Request.Context())
    if err != nil {
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
            "error": "internal", "message": "internal server error",
        })
        return
    }
    type countryResponse struct {
        CountryCode  string `json:"country_code"`
        Name         string `json:"name"`
        CurrencyCode string `json:"currency_code"`
        Region       string `json:"region"`
    }
    out := make([]countryResponse, 0, len(rows))
    for _, r := range rows {
        out = append(out, countryResponse{
            CountryCode:  r.CountryCode,
            Name:         r.Name,
            CurrencyCode: r.CurrencyCode,
            Region:       r.Region,
        })
    }
    c.JSON(http.StatusOK, gin.H{"data": out})
}
```

- [ ] **Step 4: Build**

```bash
go build ./internal/country/...
```
Expected: exits 0.

- [ ] **Step 5: Commit**

```bash
git add internal/country/
git commit -m "feat(marketplace-api): supported-countries model, repository, and public handler"
```

---

## Task 4: Wire the public endpoint + main.go

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

The `/api/v1/public/supported-countries` endpoint runs in ALL modes (admin, storefront, both) since it's unauthenticated public reference data.

- [ ] **Step 1: Add the country handler wiring to main.go**

In the section after mode-specific wiring but before engine construction, add:

```go
countryRepo := country.NewRepository(conn)
countryHandler := country.NewHandler(countryRepo)
```

Register on every gin engine:

```go
r.GET("/api/v1/public/supported-countries", countryHandler.ListSupported)
```

Add `"github.com/mark8ly/marketplace-api/internal/country"` to imports.

- [ ] **Step 2: Build the full binary**

```bash
go build ./...
```
Expected: exits 0.

- [ ] **Step 3: Smoke-test the endpoint**

Start the server (if not running): `go run ./cmd/marketplace-api`

```bash
curl -s http://localhost:8088/api/v1/public/supported-countries | jq '.data | length'
```
Expected: `15`.

- [ ] **Step 4: Commit**

```bash
git add cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): wire GET /public/supported-countries in all modes"
```

---

## Task 5: Integration test for supported-countries endpoint

**Files:**
- Create: `services/marketplace-api/internal/country/handler_integration_test.go`

- [ ] **Step 1: Write the integration test**

```go
//go:build integration

package country_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/require"

    "github.com/mark8ly/marketplace-api/internal/country"
    "github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestListSupportedCountries(t *testing.T) {
    gin.SetMode(gin.TestMode)
    db := testdb.NewDB(t) // no truncate needed — seed data is read-only

    repo := country.NewRepository(db)
    handler := country.NewHandler(repo)

    r := gin.New()
    r.GET("/api/v1/public/supported-countries", handler.ListSupported)

    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/v1/public/supported-countries", nil)
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)

    var resp struct {
        Data []struct {
            CountryCode  string `json:"country_code"`
            Name         string `json:"name"`
            CurrencyCode string `json:"currency_code"`
            Region       string `json:"region"`
        } `json:"data"`
    }
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    require.Len(t, resp.Data, 15)

    // Spot-check a few entries.
    byCode := map[string]string{}
    for _, c := range resp.Data {
        byCode[c.CountryCode] = c.CurrencyCode
    }
    require.Equal(t, "INR", byCode["IN"])
    require.Equal(t, "USD", byCode["US"])
    require.Equal(t, "EUR", byCode["DE"])
    require.Equal(t, "SGD", byCode["SG"])
}
```

- [ ] **Step 2: Run the test**

```bash
TEST_DATABASE_URL=postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable \
  go test -tags=integration -count=1 ./internal/country/...
```
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/country/handler_integration_test.go
git commit -m "test(marketplace-api): integration test for GET /public/supported-countries"
```

---

## Task 6: Onboarding app — filter country picker to supported countries

**Files:**
- Create or modify: `apps/onboarding/lib/api/countries.ts`
- Modify: the onboarding form component that renders the country picker

- [ ] **Step 1: Find the current country picker implementation**

```bash
grep -rn "country" apps/onboarding/app/onboarding/ --include="*.tsx" --include="*.ts" | head -20
grep -rn "country" apps/onboarding/components/ --include="*.tsx" --include="*.ts" | head -20
```

Identify which component renders the country select and where it gets its options from.

- [ ] **Step 2: Create the marketplace-api countries client**

`apps/onboarding/lib/api/countries.ts`:

```typescript
const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface SupportedCountry {
  country_code: string;
  name: string;
  currency_code: string;
  region: string;
}

export async function fetchSupportedCountries(): Promise<SupportedCountry[]> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/public/supported-countries`,
    { cache: "force-cache" }, // static data, cache aggressively
  );
  if (!res.ok) {
    throw new Error(`supported-countries: ${res.status}`);
  }
  const body = (await res.json()) as { data: SupportedCountry[] };
  return body.data;
}
```

- [ ] **Step 3: Update the country picker to use the new endpoint**

Replace the current country options source (location-service or hardcoded list) with a call to `fetchSupportedCountries()`. The select renders only the 15 supported countries.

This step depends on what the exploration in step 1 reveals — the exact file and component name will vary. The pattern is:

```typescript
// Before: countries from location-service (200+ countries)
// After:
import { fetchSupportedCountries } from "@/lib/api/countries";
const countries = await fetchSupportedCountries();
// Render <select> options from countries
```

- [ ] **Step 4: Verify the onboarding app builds**

```bash
cd apps/onboarding && npx tsc --noEmit -p .
```
Expected: exits 0.

- [ ] **Step 5: Commit**

```bash
git add apps/onboarding/lib/api/countries.ts apps/onboarding/...
git commit -m "feat(onboarding): filter country picker to 15 supported countries via marketplace-api"
```

---

## Task 7: Push to main

- [ ] **Step 1: Run full integration tests**

```bash
cd services/marketplace-api
TEST_DATABASE_URL=postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable \
  go test -tags=integration -count=1 -p 1 ./internal/country/... ./internal/order/... ./internal/outbox/...
```
Expected: all PASS.

- [ ] **Step 2: Full Go build**

```bash
go build ./...
```
Expected: exits 0.

- [ ] **Step 3: Push**

```bash
git push origin <branch>:main
```

---

## What P1 delivers

After P1 ships:
- The database has all 12 tables P2/P3/P4 need
- 15 supported countries are seeded and queryable
- Provider interfaces are defined — P2 implements `Gateway`, P3 implements `Carrier`, P4 implements `Calculator`
- The onboarding country picker shows only the 15 supported countries
- A public endpoint serves the country list for any consumer

**Next:** P2 (payment providers) can start immediately. P3 (shipping) and P4 (tax) can start in parallel with P2 since they share no code — only the migration and interfaces from P1.
