# Payments, Shipping & Tax — Unified Design Spec

**Date:** 2026-04-10
**Status:** Draft — awaiting user review

## 1. Overview

Unified slice adding payments (Stripe, Razorpay, PayPal), shipping (ShipEngine, Delhivery, NinjaVan), tax calculation (flat rate, India GST, US TaxJar), platform fees with ledger, 15-country support, admin settings, onboarding country filtering, and storefront checkout integration — all inside the existing `marketplace-api` binary.

### Scope boundaries

**In scope:** payment intent creation + webhook handling + refund via provider, shipping rate calculation + label creation + tracking, tax calculation (3 strategies), platform fee ledger, admin settings (essential config, country-gated), onboarding country picker filtering, storefront checkout + shipping rate selector + order confirmation, Impeccable chain pass on all new UI surfaces.

**Out of scope (future slices):** promo codes, gift cards, loyalty points, campaigns, advanced admin config (per-region handling fees, insurance, auto-select strategy), payout/settlement to merchant bank accounts.

### Key constraints

- Same binary (`marketplace-api`), same repo (`mark8ly`)
- Provider interfaces so adding provider N+1 is one file
- Country-gated visibility — merchants only see providers their store country supports
- 15 supported countries, single source of truth in `supported_countries` table
- Platform fees from day 1 — Mark8ly takes a cut on every transaction

## 2. Supported Countries

| Code | Name | Region | Currency | Payment providers | Shipping carriers | Tax strategy | Tax rate |
|---|---|---|---|---|---|---|---|
| IN | India | india | INR | razorpay, paypal | delhivery | india_gst | varies by HSN |
| US | United States | americas | USD | stripe, paypal | shipengine | taxjar | varies by jurisdiction |
| CA | Canada | americas | CAD | stripe, paypal | shipengine | flat | 5.0 (GST, province adds HST/PST) |
| GB | United Kingdom | europe | GBP | stripe, paypal | shipengine | flat | 20.0 |
| DE | Germany | europe | EUR | stripe, paypal | shipengine | flat | 19.0 |
| FR | France | europe | EUR | stripe, paypal | shipengine | flat | 20.0 |
| IT | Italy | europe | EUR | stripe, paypal | shipengine | flat | 22.0 |
| ES | Spain | europe | EUR | stripe, paypal | shipengine | flat | 21.0 |
| NL | Netherlands | europe | EUR | stripe, paypal | shipengine | flat | 21.0 |
| AU | Australia | europe | AUD | stripe, paypal | shipengine | flat | 10.0 |
| SG | Singapore | sea | SGD | stripe, paypal | ninjavan | flat | 9.0 |
| MY | Malaysia | sea | MYR | stripe, paypal | ninjavan | flat | 8.0 |
| TH | Thailand | sea | THB | stripe, paypal | ninjavan | flat | 7.0 |
| PH | Philippines | sea | PHP | stripe, paypal | ninjavan | flat | 12.0 |
| ID | Indonesia | sea | IDR | stripe, paypal | ninjavan | flat | 11.0 |

Note: Canada flat rate is the federal GST only (5%). Provincial HST/PST is deferred to a future slice where TaxJar handles CA like it handles US. The 5% is a conservative baseline — merchants in participating provinces may need to manually adjust. Documented in admin settings help text.

## 3. Provider Abstraction

### 3.1 Payment gateway interface

```go
// internal/payment/gateway.go
type Gateway interface {
    CreateIntent(ctx context.Context, in CreateIntentInput) (*Intent, error)
    CapturePayment(ctx context.Context, captureID string) (*Capture, error)
    RefundPayment(ctx context.Context, in RefundInput) (*Refund, error)
    VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error)
    ProviderName() string
    SupportedCountries() []string
}
```

Implementations: `stripe.go`, `razorpay.go`, `paypal.go`.

### 3.2 Shipping carrier interface

```go
// internal/shipping/carrier.go
type Carrier interface {
    GetRates(ctx context.Context, in RateRequest) ([]Rate, error)
    CreateShipment(ctx context.Context, in ShipmentRequest) (*Shipment, error)
    GetTracking(ctx context.Context, trackingNumber string) (*Tracking, error)
    CancelShipment(ctx context.Context, shipmentID string) error
    ProviderName() string
    SupportedCountries() []string
}
```

Implementations: `shipengine.go`, `delhivery.go`, `ninjavan.go`.

### 3.3 Tax calculator interface

```go
// internal/tax/calculator.go
type Calculator interface {
    Calculate(ctx context.Context, in TaxRequest) (*TaxBreakdown, error)
    ProviderName() string
}
```

Implementations: `flat.go`, `india.go`, `taxjar.go`.

### 3.4 Provider resolution

All three interfaces use the same resolution pattern:

```
Store.country_code
  → SELECT payment_providers, shipping_carriers, tax_strategy FROM supported_countries
  → instantiate the correct implementation(s)
  → pass to service layer
```

Gateway/carrier credentials are loaded from `payment_gateway_configs` / `shipping_carrier_configs` for the specific store. Tax flat rates come from `supported_countries.tax_rate`; India GST rates from product-level `hsn_code` + `gst_rate` fields; TaxJar API key from a per-store config row.

## 4. Data Model

### 4.1 New tables

```sql
-- Single source of truth for which countries Mark8ly supports.
-- Seeds 15 rows at migration time. Adding country #16 is one INSERT.
CREATE TABLE supported_countries (
    country_code  CHAR(2)      PRIMARY KEY,
    name          VARCHAR(100) NOT NULL,
    currency_code CHAR(3)      NOT NULL,
    region        VARCHAR(20)  NOT NULL,  -- 'india','americas','europe','sea'
    payment_providers TEXT[]   NOT NULL,   -- e.g. '{stripe,paypal}'
    shipping_carriers TEXT[]   NOT NULL,   -- e.g. '{shipengine}'
    tax_strategy  VARCHAR(20)  NOT NULL DEFAULT 'flat', -- 'flat','india_gst','taxjar'
    tax_rate      NUMERIC(5,2),           -- flat rate %; NULL for india_gst/taxjar
    is_active     BOOLEAN      NOT NULL DEFAULT true
);

-- Per-store payment gateway credentials + activation state.
CREATE TABLE payment_gateway_configs (
    id                 UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID         NOT NULL,
    store_id           UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    provider           VARCHAR(20)  NOT NULL,  -- 'stripe','razorpay','paypal'
    api_key_encrypted  TEXT         NOT NULL,
    secret_key_encrypted TEXT,
    mode               VARCHAR(10)  NOT NULL DEFAULT 'test', -- 'test','live'
    is_active          BOOLEAN      NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (store_id, provider)
);

-- Per-store shipping carrier credentials + activation state.
CREATE TABLE shipping_carrier_configs (
    id                 UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID         NOT NULL,
    store_id           UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    provider           VARCHAR(20)  NOT NULL,  -- 'shipengine','delhivery','ninjavan'
    api_key_encrypted  TEXT         NOT NULL,
    secret_key_encrypted TEXT,
    mode               VARCHAR(10)  NOT NULL DEFAULT 'test',
    is_active          BOOLEAN      NOT NULL DEFAULT false,
    -- Warehouse / ship-from address (required before creating shipments)
    warehouse_name     VARCHAR(200),
    warehouse_line1    VARCHAR(300),
    warehouse_line2    VARCHAR(300),
    warehouse_city     VARCHAR(200),
    warehouse_region   VARCHAR(200),
    warehouse_postal   VARCHAR(40),
    warehouse_country  CHAR(2),
    warehouse_phone    VARCHAR(40),
    -- Rate adjustments
    handling_fee       NUMERIC(12,2) NOT NULL DEFAULT 0,
    free_shipping_min  NUMERIC(12,2),          -- NULL = no free shipping threshold
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, provider)
);

-- Payment transaction records.
CREATE TABLE payment_transactions (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL,
    order_id            UUID          NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    provider            VARCHAR(20)   NOT NULL,
    provider_intent_id  VARCHAR(200),
    provider_payment_id VARCHAR(200),
    amount              NUMERIC(12,2) NOT NULL,
    currency_code       CHAR(3)       NOT NULL,
    status              VARCHAR(20)   NOT NULL DEFAULT 'pending',
    payment_method      VARCHAR(40),  -- 'card','upi','netbanking','wallet','paypal'
    metadata            JSONB         NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX payment_tx_order_idx ON payment_transactions (order_id);

-- Refund transaction records (linked to original payment).
CREATE TABLE refund_transactions (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    payment_transaction_id UUID       NOT NULL REFERENCES payment_transactions(id),
    order_id            UUID          NOT NULL,
    provider_refund_id  VARCHAR(200),
    amount              NUMERIC(12,2) NOT NULL,
    currency_code       CHAR(3)       NOT NULL,
    status              VARCHAR(20)   NOT NULL DEFAULT 'pending',
    reason              VARCHAR(200),
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- Webhook event log (audit trail, idempotency guard).
CREATE TABLE webhook_events (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    provider          VARCHAR(20)  NOT NULL,
    provider_event_id VARCHAR(200) NOT NULL,
    event_type        VARCHAR(60)  NOT NULL,
    payload           JSONB        NOT NULL,
    status            VARCHAR(20)  NOT NULL DEFAULT 'received', -- 'received','processed','failed'
    processed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_event_id)
);

-- Shipment records.
CREATE TABLE shipments (
    id                   UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID          NOT NULL,
    store_id             UUID          NOT NULL,
    order_id             UUID          NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    carrier              VARCHAR(20)   NOT NULL,
    tracking_number      VARCHAR(100),
    label_url            TEXT,
    status               VARCHAR(20)   NOT NULL DEFAULT 'pending',
    ship_from            JSONB         NOT NULL,  -- address snapshot
    ship_to              JSONB         NOT NULL,  -- address snapshot
    base_rate            NUMERIC(12,2),
    handling_fee         NUMERIC(12,2) NOT NULL DEFAULT 0,
    total_cost           NUMERIC(12,2),
    currency_code        CHAR(3)       NOT NULL,
    estimated_delivery   TIMESTAMPTZ,
    shipped_at           TIMESTAMPTZ,
    delivered_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX shipments_order_idx ON shipments (order_id);

-- Platform fee configuration (per-store, set by Mark8ly platform).
CREATE TABLE platform_fee_configs (
    id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID          NOT NULL,
    store_id      UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    fee_percent   NUMERIC(5,2)  NOT NULL DEFAULT 2.5,
    fee_fixed     NUMERIC(12,2) NOT NULL DEFAULT 0.30,
    fee_currency  CHAR(3)       NOT NULL DEFAULT 'USD',
    payer         VARCHAR(20)   NOT NULL DEFAULT 'merchant', -- 'merchant','customer','split'
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

-- Append-only platform fee ledger.
CREATE TABLE platform_fee_ledger (
    id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID          NOT NULL,
    store_id          UUID          NOT NULL,
    order_id          UUID          NOT NULL REFERENCES orders(id),
    transaction_type  VARCHAR(20)   NOT NULL, -- 'collection','refund'
    gross_amount      NUMERIC(12,2) NOT NULL,
    fee_amount        NUMERIC(12,2) NOT NULL,
    net_amount        NUMERIC(12,2) NOT NULL,
    currency_code     CHAR(3)       NOT NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX fee_ledger_order_idx ON platform_fee_ledger (order_id);
CREATE INDEX fee_ledger_store_idx ON platform_fee_ledger (store_id, created_at);

-- Per-order tax breakdown lines (for invoicing).
CREATE TABLE order_tax_lines (
    id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id      UUID          NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    description   VARCHAR(100)  NOT NULL, -- 'CGST 9%','CA State Tax','VAT 20%'
    rate          NUMERIC(5,2)  NOT NULL,
    amount        NUMERIC(12,2) NOT NULL,
    jurisdiction  VARCHAR(100)            -- 'Maharashtra','CA-Los Angeles', NULL for flat
);
CREATE INDEX tax_lines_order_idx ON order_tax_lines (order_id);
```

### 4.2 Product table additions (India GST)

```sql
ALTER TABLE products ADD COLUMN hsn_code VARCHAR(10);
ALTER TABLE products ADD COLUMN gst_rate NUMERIC(5,2);
```

Only populated for stores in India. Ignored by the flat-rate and TaxJar strategies.

### 4.3 TaxJar config (US stores)

Stored in a generic `tax_provider_configs` table or reuse the `payment_gateway_configs` pattern:

```sql
CREATE TABLE tax_provider_configs (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID         NOT NULL,
    store_id          UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    provider          VARCHAR(20)  NOT NULL DEFAULT 'taxjar',
    api_key_encrypted TEXT         NOT NULL,
    mode              VARCHAR(10)  NOT NULL DEFAULT 'test',
    is_active         BOOLEAN      NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (store_id, provider)
);
```

## 5. Payment Flow

### 5.1 Checkout (storefront)

```
Customer loads checkout page
  → storefront calls GET /api/v1/storefront/stores/:slug/payment-methods
      → returns available methods based on store country (e.g. ["card","upi"] for IN)
  → customer fills in details, clicks "Pay"
  → storefront calls POST /api/v1/storefront/stores/:slug/checkout (extended)
      → order.Service.Create (existing)
      → tax.Service.Calculate → writes order_tax_lines
      → payment.Service.CreateIntent(order, gateway)
          → Stripe: PaymentIntent → returns client_secret
          → Razorpay: creates razorpay order → returns razorpay_order_id + key_id
          → PayPal: creates order → returns approval_url
      → inserts payment_transactions row (status=pending)
      → response: { order_id, payment_token, provider, ... }
  → storefront completes payment client-side (Stripe.js / Razorpay.js / PayPal redirect)
```

### 5.2 Webhook (provider callback)

```
Provider → POST /api/v1/webhooks/:provider
  → payment.WebhookHandler.Handle(provider, payload, signature)
      → gateway.VerifyWebhook (signature verification)
      → idempotency: INSERT INTO webhook_events ... ON CONFLICT DO NOTHING
      → if duplicate: return 200 (already processed)
      → payment.Service.RecordPayment → update payment_transactions.status
      → order.Service.Confirm(ctx, nil, orderID, &PaymentStatusPaid, "webhook")
      → platform.Service.RecordFee → append to platform_fee_ledger
      → outbox event: order.payment_succeeded
  → respond 200 to provider
```

### 5.3 Refund (admin, extended)

```
Admin → POST /orders/:id/refund (existing endpoint, enhanced)
  → payment.Service.RefundViaProvider(orderID, amount)
      → load payment_transactions for this order
      → gateway.RefundPayment (Stripe/Razorpay/PayPal refund API)
      → insert refund_transactions row
      → on success: order.Service.RecordRefund (existing atomic guard)
      → platform.Service.RecordRefundFee → ledger credit row
  → response: updated order
```

## 6. Shipping Flow

### 6.1 Rate calculation (storefront checkout)

```
Customer enters shipping address during checkout
  → storefront calls POST /api/v1/storefront/stores/:slug/shipping-rates
      → body: { items, ship_to_address }
      → shipping.Service.GetRates(storeID, warehouseAddress, toAddress, items)
          → resolve carrier from store country
          → carrier.GetRates → provider API call
          → apply handling_fee from shipping_carrier_configs
          → apply free_shipping_min threshold if subtotal qualifies
      → response: [{ service, carrier, price, estimated_days }, ...]
  → customer selects a rate
  → selected rate included as shipping_total in checkout request
```

### 6.2 Shipment creation (admin)

```
Admin → POST /orders/:id/ship (new endpoint)
  → body: { carrier, service, ship_from_address? }
      → defaults to warehouse from shipping_carrier_configs if omitted
  → shipping.Service.CreateShipment
      → carrier.CreateShipment → tracking number + label URL
      → insert shipments row
      → order.Service.MarkFulfilled (existing)
      → outbox event: order.shipped with tracking_number
  → response: shipment with tracking_number + label_url
```

### 6.3 Tracking

```
GET /api/v1/admin/stores/:storeId/orders/:id/shipment
GET /api/v1/storefront/stores/:slug/orders/:id/tracking (customer-facing)
  → shipping.Service.GetTracking(shipmentID)
      → carrier.GetTracking → real-time status from carrier API
  → response: { status, tracking_number, events[], estimated_delivery }
```

## 7. Tax Calculation

### 7.1 Flat rate (GB, EU, AU, CA, SEA)

```
tax_total = subtotal × (supported_countries.tax_rate / 100)
```

Single `order_tax_lines` row: description="VAT 20%", rate=20.0, amount=calculated.

### 7.2 India GST

Per-item calculation:
- Look up product's `gst_rate` (0%, 5%, 12%, 18%, 28%)
- Compare seller state (warehouse address) vs buyer state (shipping address)
  - **Same state (intra-state):** CGST = rate/2, SGST = rate/2
  - **Different state (inter-state):** IGST = rate
- Write 2 `order_tax_lines` rows per item for intra-state (CGST + SGST) or 1 for inter-state (IGST)

### 7.3 US TaxJar

- Call TaxJar `/v2/taxes` with line items (product tax codes), from address, to address
- TaxJar returns jurisdiction-level breakdown
- Write one `order_tax_lines` row per jurisdiction returned

## 8. Platform Fees

### 8.1 Configuration

```
platform_fee_configs:
  fee_percent: 2.5      # percentage of gross_amount
  fee_fixed: 0.30       # fixed per transaction
  payer: merchant        # merchant | customer | split
```

Set by Mark8ly platform, not merchant-editable. Read-only in admin order detail.

### 8.2 Calculation

```
fee = (gross × fee_percent / 100) + fee_fixed
net = gross - fee
```

### 8.3 Ledger entries

| Event | type | gross | fee | net |
|---|---|---|---|---|
| Payment succeeded | collection | 100.00 | 2.80 | 97.20 |
| Partial refund 30.00 | refund | -30.00 | -0.75 | -29.25 |

Fee credited back proportionally on refund: `refund_fee = (refund_amount / original_gross) × original_fee`.

## 9. Admin Settings

### 9.1 Payments settings (`/settings/payments`)

- Country-gated: only shows providers from `supported_countries.payment_providers` for the store's country
- Per provider card: active toggle, API key + secret inputs, test/live mode switch
- "Test connection" button: zero-value auth to verify credentials
- Read-only: supported payment methods for this provider+country

### 9.2 Shipping settings (`/settings/shipping`)

- Country-gated: only shows carriers from `supported_countries.shipping_carriers`
- Per carrier card: active toggle, API key input, test/live mode
- Default warehouse address form (required before creating any shipment)
- Handling fee input (flat amount added to carrier rates)
- Free shipping threshold input (optional)

### 9.3 Tax settings (`/settings/tax`)

- India stores: products form gets HSN code + GST rate fields; settings page shows "India GST (automatic)" — no config needed
- US stores: TaxJar API key input, test/live mode; settings page shows "TaxJar (automatic per jurisdiction)"
- All others: settings page shows "VAT/GST at {rate}% (automatic)" — read-only, rate from `supported_countries`

## 10. Onboarding Integration

The onboarding app (`apps/onboarding`) country picker currently shows all active countries from the location-service.

**Change:** replace with a call to `GET /api/v1/public/supported-countries` which returns the 15 rows from the `supported_countries` table. No auth — public reference data.

Response shape:
```json
{
  "data": [
    { "country_code": "IN", "name": "India", "currency_code": "INR", "region": "india" },
    { "country_code": "US", "name": "United States", "currency_code": "USD", "region": "americas" },
    ...
  ]
}
```

The country picker renders only these 15 countries. When Mark8ly adds country #16, it's one DB row insert — no frontend deploy.

## 11. Storefront Integration

### 11.1 New/extended endpoints

```
POST /api/v1/storefront/stores/:slug/checkout          # extended — now returns payment token
GET  /api/v1/storefront/stores/:slug/payment-methods    # available methods for store country
POST /api/v1/storefront/stores/:slug/shipping-rates     # rate calculation
GET  /api/v1/storefront/stores/:slug/orders/:id         # order status (customer-facing)
GET  /api/v1/storefront/stores/:slug/orders/:id/tracking # shipment tracking
POST /api/v1/webhooks/stripe                            # Stripe webhook
POST /api/v1/webhooks/razorpay                          # Razorpay webhook
POST /api/v1/webhooks/paypal                            # PayPal webhook
GET  /api/v1/public/supported-countries                 # onboarding + storefront
```

### 11.2 Storefront UI (`apps/storefront`)

- **Checkout page** — payment form (Stripe Elements / Razorpay.js / PayPal buttons), auto-selected by store country. Shipping rate selector. Tax line display. Order summary with subtotal/shipping/tax/total.
- **Order confirmation page** — `app/orders/[id]/page.tsx` with order number, payment status, items, shipping address, tracking link (once shipped).
- **Order history** (optional, can defer) — list of customer's past orders.

### 11.3 Admin order detail (extended)

The existing order detail page gets three new sections:
- **Payment info** — provider, method, transaction ID, status
- **Shipping info** — carrier, tracking number, label download link, status timeline
- **Platform fee summary** — gross, fee, net (read-only)

## 12. Impeccable Chain Pass (final task)

After all UI surfaces ship, run the full chain on:
- **Admin:** `/settings/payments`, `/settings/shipping`, `/settings/tax`, order detail (extended with payment/shipping/fee)
- **Storefront:** checkout page (payment + shipping + tax), order confirmation page
- **Onboarding:** country picker (15 supported countries)

Chain: `/critique` → fix P0/P1s → `/harden` (payment forms are high-stakes) → `/arrange` → `/clarify` → `/delight` → `/polish`

## 13. Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Stripe/Razorpay webhook delivery failure | Payment collected but order stays pending | Webhook retry (providers retry for 72h). Manual reconciliation endpoint in admin. |
| TaxJar API downtime | US checkout cannot calculate tax | Fallback to store-configured flat rate with admin warning. |
| Carrier rate API timeout | Checkout shows no shipping options | Cache last-known rates per (origin, destination, weight tier) with 1h TTL. Show cached + "(estimated)" label. |
| Provider credential rotation | Live payments break | Test-connection button in admin. Mode toggle (test→live) is a deliberate action, not automatic. |
| Platform fee dispute | Merchant contests the fee | Ledger is append-only and auditable. Every row traces to an order_id. |
| India GST rate changes | Tax calculated at old rate | GST rates are per-product fields, merchant-editable. Platform can also push rate updates. |
| Canada province-level tax | Federal GST only (5%) is under-collecting in some provinces | Documented in admin settings. TaxJar integration for CA is a future slice. |

## 14. Out of Scope (future slices)

1. Promo codes, gift cards, loyalty points, campaigns — depend on this slice but don't affect its architecture
2. Payout/settlement to merchant bank accounts — needs banking integration (Stripe Connect, Razorpay Route)
3. Advanced admin config — per-region handling fees, insurance, auto-select strategy, carrier priority
4. Canada province-level tax via TaxJar
5. Additional countries beyond the initial 15
6. Additional payment providers (Afterpay, Cashfree, PhonePe, etc.)
7. Additional shipping carriers (Shiprocket excluded deliberately, FedEx/UPS direct)
8. Split payments (gift card + payment provider in one checkout)
9. Subscription/recurring payments
10. Invoice PDF generation with tax breakdown (uses the `order_tax_lines` data but the PDF renderer is a separate effort)
