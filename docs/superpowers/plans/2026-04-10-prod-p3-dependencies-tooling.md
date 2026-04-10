# Production Readiness P3 — Dependencies & Tooling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Update all outdated dependencies, configure Dependabot, create migration runbook, API documentation, load testing scripts, and secrets rotation runbook.

**Architecture:** Dependency updates across Go and Node. New docs in docs/runbooks/. Dependabot config. k6 load test scripts.

**Tech Stack:** Go modules, npm, Dependabot, k6, swaggo/swag (optional for OpenAPI).

---

## Task 1 — Go dependency updates

**File:** `services/marketplace-api/go.mod`

### Step 1: Update `golang.org/x/*` packages

Run from `services/marketplace-api/`:

```bash
cd services/marketplace-api
go get -u golang.org/x/crypto golang.org/x/net golang.org/x/sys golang.org/x/text golang.org/x/sync
go mod tidy
```

**Current versions (from go.mod):**
```
golang.org/x/sync v0.20.0
golang.org/x/text v0.35.0
golang.org/x/crypto v0.49.0   (indirect)
golang.org/x/net v0.52.0       (indirect)
golang.org/x/sys v0.42.0       (indirect)
```

These should update to their latest stable versions. The `go get -u` command fetches the newest minor/patch releases.

### Step 2: Update other direct dependencies

```bash
go get -u github.com/gin-gonic/gin
go get -u gorm.io/gorm gorm.io/driver/postgres gorm.io/datatypes
go get -u github.com/stretchr/testify
go get -u github.com/jackc/pgx/v5
go get -u github.com/openfga/go-sdk
go get -u github.com/shopspring/decimal
go get -u github.com/microcosm-cc/bluemonday
go get -u github.com/golang-migrate/migrate/v4
go mod tidy
```

### Step 3: Verify

```bash
go vet ./...
go build ./...
go test ./...
```

If any tests fail after the update, pin the failing dependency back to its previous version and document the incompatibility for manual investigation.

### Step 4: Check for vulnerabilities

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

Address any findings before committing.

**Verify:** `go mod tidy` produces no diff (all deps are clean). `go vet ./...` and `go test ./...` pass. `govulncheck` reports no actionable vulnerabilities.

---

## Task 2 — npm audit and update (storefront)

**File:** `apps/storefront/package.json`

### Step 1: Audit

```bash
cd apps/storefront
npm audit
```

### Step 2: Fix vulnerabilities

```bash
npm audit fix
```

### Step 3: Update dependencies

```bash
npm update
```

### Step 4: Verify

```bash
npm run check-types
npm run build
```

**Current dependencies (from package.json):**
```json
{
  "next": "^16.1.1",
  "react": "19.2.0",
  "react-dom": "19.2.0",
  "tailwindcss": "^4.2.2"
}
```

**Notes:**
- `react` and `react-dom` are pinned at `19.2.0` — only update if a security advisory exists. Pinned versions are intentional.
- `next` uses `^` so `npm update` will pull the latest `16.x`.

**Verify:** `npm audit` returns 0 vulnerabilities. `npm run build` succeeds.

---

## Task 3 — npm audit and update (admin)

**File:** `apps/admin/package.json`

### Step 1: Audit and fix

```bash
cd apps/admin
npm audit
npm audit fix
npm update
```

### Step 2: Verify

```bash
npm run check-types
npm run build
npm run test
```

**Notes:**
- Admin has more dependencies (dnd-kit, framer-motion, recharts via @repo/ui, react-hook-form, zod, papaparse). Each update should be verified with the type checker and test suite.
- If `npm audit fix` introduces breaking changes (major version bumps), revert and document for manual review.

**Verify:** `npm audit` returns 0 vulnerabilities. `npm run build` and `npm run test` pass.

---

## Task 4 — Dependabot configuration

**File:** `.github/dependabot.yml` (new file)

```yaml
# .github/dependabot.yml
# Auto-creates PRs for outdated dependencies.
# See: https://docs.github.com/en/code-security/dependabot/dependabot-version-updates
version: 2
updates:
  # Go modules — marketplace-api
  - package-ecosystem: gomod
    directory: /services/marketplace-api
    schedule:
      interval: weekly
      day: monday
      time: "06:00"
      timezone: Asia/Kolkata
    open-pull-requests-limit: 5
    reviewers:
      - sam123ben
    labels:
      - dependencies
      - go
    commit-message:
      prefix: "chore(deps)"

  # Go modules — platform-api
  - package-ecosystem: gomod
    directory: /services/platform-api
    schedule:
      interval: weekly
      day: monday
      time: "06:00"
      timezone: Asia/Kolkata
    open-pull-requests-limit: 3
    labels:
      - dependencies
      - go
    commit-message:
      prefix: "chore(deps)"

  # Go modules — auth-bff
  - package-ecosystem: gomod
    directory: /services/auth-bff
    schedule:
      interval: weekly
      day: monday
      time: "06:00"
      timezone: Asia/Kolkata
    open-pull-requests-limit: 3
    labels:
      - dependencies
      - go
    commit-message:
      prefix: "chore(deps)"

  # npm — admin
  - package-ecosystem: npm
    directory: /apps/admin
    schedule:
      interval: weekly
      day: monday
      time: "06:00"
      timezone: Asia/Kolkata
    open-pull-requests-limit: 5
    labels:
      - dependencies
      - npm
    commit-message:
      prefix: "chore(deps)"
    ignore:
      # React is pinned — don't auto-bump
      - dependency-name: "react"
      - dependency-name: "react-dom"

  # npm — storefront
  - package-ecosystem: npm
    directory: /apps/storefront
    schedule:
      interval: weekly
      day: monday
      time: "06:00"
      timezone: Asia/Kolkata
    open-pull-requests-limit: 5
    labels:
      - dependencies
      - npm
    commit-message:
      prefix: "chore(deps)"
    ignore:
      - dependency-name: "react"
      - dependency-name: "react-dom"

  # npm — onboarding
  - package-ecosystem: npm
    directory: /apps/onboarding
    schedule:
      interval: weekly
      day: monday
      time: "06:00"
      timezone: Asia/Kolkata
    open-pull-requests-limit: 5
    labels:
      - dependencies
      - npm
    commit-message:
      prefix: "chore(deps)"
    ignore:
      - dependency-name: "react"
      - dependency-name: "react-dom"

  # GitHub Actions
  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: monthly
    open-pull-requests-limit: 3
    labels:
      - dependencies
      - ci
    commit-message:
      prefix: "chore(ci)"
```

**Notes:**
- Weekly schedule on Mondays at 6 AM IST — gives the week to review and merge.
- React/ReactDOM are excluded from auto-bump since they're pinned.
- Each ecosystem has a reasonable PR limit to avoid noise.
- `chore(deps)` prefix follows the repo's conventional commits format.

**Verify:** After pushing, go to the repo's Settings > Code security and analysis > Dependabot — confirm it's enabled and the config is recognized.

---

## Task 5 — Database migration runbook

**File:** `docs/runbooks/database-migrations.md` (new file)

```markdown
# Database Migration Runbook

## Overview

marketplace-api uses [golang-migrate](https://github.com/golang-migrate/migrate)
for schema migrations. Migration files live in
`services/marketplace-api/migrations/` as numbered pairs:
`NNNNNN_description.up.sql` and `NNNNNN_description.down.sql`.

## Adding a new migration

1. Determine the next sequence number:
   ```bash
   ls services/marketplace-api/migrations/*.up.sql | tail -1
   # e.g., 000018_add_gift_cards.up.sql → next is 000019
   ```

2. Create both files:
   ```bash
   touch services/marketplace-api/migrations/000019_description.up.sql
   touch services/marketplace-api/migrations/000019_description.down.sql
   ```

3. Write the UP migration (the forward schema change).

4. Write the DOWN migration (the exact reverse). Every UP must have a
   working DOWN so rollbacks are possible.

5. Test locally:
   ```bash
   cd services/marketplace-api
   go run ./cmd/... migrate up
   go run ./cmd/... migrate down 1
   go run ./cmd/... migrate up
   ```

## Running migrations locally

```bash
cd services/marketplace-api

# Apply all pending migrations
go run ./cmd/... migrate up

# Rollback the last migration
go run ./cmd/... migrate down 1

# Check current version
go run ./cmd/... migrate version
```

Requires `DATABASE_URL` or individual `DB_*` env vars in `.env`.

## Running migrations in production

Migrations run automatically via a Kubernetes init container before the
main marketplace-api container starts.

**Flow:**
1. New image is deployed (ArgoCD syncs the tag bump from tesserix-k8s)
2. Init container runs: `marketplace-api migrate up`
3. If migration succeeds → main container starts
4. If migration fails → pod stays in Init:Error, no traffic is served

**To run manually (emergency):**
```bash
# Port-forward the Cloud SQL Auth Proxy pod
kubectl port-forward -n marketplace deploy/marketplace-api 5432:5432

# Run from local machine
DATABASE_URL="postgres://marketplace_user:PASSWORD@localhost:5432/marketplace_db?sslmode=disable" \
  go run ./cmd/... migrate up
```

## Rollback procedure

1. Identify the target version:
   ```bash
   # Check current version in production
   kubectl exec -n marketplace deploy/marketplace-api -c marketplace-api -- \
     marketplace-api migrate version
   ```

2. Roll back one step:
   ```bash
   kubectl exec -n marketplace deploy/marketplace-api -c marketplace-api -- \
     marketplace-api migrate down 1
   ```

3. If the rollback migration fails, connect directly and run the DOWN SQL
   manually via `psql`.

## Schema version assertion

On startup, marketplace-api checks the migration version matches the
expected version compiled into the binary. If the schema is behind, the
service refuses to start and logs:
```
FATAL: schema version mismatch: expected 19, got 18. Run migrations.
```

This prevents running application code against an incompatible schema.

## Best practices

- Always write both UP and DOWN.
- Never modify a migration that has been applied in production.
- Use `IF NOT EXISTS` / `IF EXISTS` guards for idempotency.
- Keep migrations small — one logical change per file.
- Test the full up/down/up cycle locally before pushing.
- For data migrations (backfills), create a separate migration file rather
  than mixing DDL and DML.
```

**Verify:** The file is well-formed Markdown and matches the actual migration tooling in `services/marketplace-api/cmd/`.

---

## Task 6 — API documentation (manual OpenAPI YAML)

**File:** `docs/api/storefront-api.yaml` (new file)

Create a minimal OpenAPI 3.1 spec covering the public storefront and webhook endpoints. This is a manual spec — not auto-generated from code.

```yaml
# docs/api/storefront-api.yaml
openapi: "3.1.0"
info:
  title: Mark8ly Storefront API
  version: "1.0.0"
  description: |
    Public-facing API for Mark8ly storefronts. All endpoints are scoped
    to a store via the `X-Store-Slug` header or subdomain resolution.

servers:
  - url: https://{store}.mark8ly.com/api/v1
    description: Production (per-store subdomain)
    variables:
      store:
        default: demo

paths:
  /storefront/products:
    get:
      operationId: listProducts
      summary: List published products
      tags: [Products]
      parameters:
        - name: page
          in: query
          schema: { type: integer, default: 1 }
        - name: page_size
          in: query
          schema: { type: integer, default: 24, maximum: 100 }
        - name: search
          in: query
          schema: { type: string }
        - name: category_slug
          in: query
          schema: { type: string }
      responses:
        "200":
          description: Paginated product list
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items: { $ref: "#/components/schemas/StorefrontProduct" }
                  meta:
                    $ref: "#/components/schemas/PaginationMeta"

  /storefront/products/{handle}:
    get:
      operationId: getProductByHandle
      summary: Get a single product by its URL handle
      tags: [Products]
      parameters:
        - name: handle
          in: path
          required: true
          schema: { type: string }
      responses:
        "200":
          description: Product detail with variants, options, and media
          content:
            application/json:
              schema: { $ref: "#/components/schemas/StorefrontProduct" }
        "404":
          description: Product not found

  /storefront/categories:
    get:
      operationId: listCategories
      summary: List published categories for a store
      tags: [Categories]
      responses:
        "200":
          description: Category list
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items: { $ref: "#/components/schemas/Category" }

  /storefront/checkout:
    post:
      operationId: submitCheckout
      summary: Submit a checkout order
      tags: [Checkout]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/CheckoutBody" }
      responses:
        "201":
          description: Order created
          content:
            application/json:
              schema:
                type: object
                properties:
                  order_id: { type: string, format: uuid }
                  order_number: { type: string }
        "400":
          description: Validation error
        "409":
          description: Idempotency conflict (order already exists)

  /storefront/shipping/rates:
    post:
      operationId: fetchShippingRates
      summary: Get available shipping rates for a destination
      tags: [Shipping]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/ShippingRateRequest" }
      responses:
        "200":
          description: Available shipping rates
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items: { $ref: "#/components/schemas/ShippingRate" }

  /storefront/payment-methods:
    get:
      operationId: getPaymentMethods
      summary: List enabled payment methods for a store
      tags: [Payment]
      responses:
        "200":
          description: Payment methods
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items: { $ref: "#/components/schemas/PaymentMethod" }

  /storefront/coupons/validate:
    post:
      operationId: validateCoupon
      summary: Validate and preview a coupon code
      tags: [Coupons]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [code, subtotal, currency_code]
              properties:
                code: { type: string }
                subtotal: { type: string }
                currency_code: { type: string }
                customer_email: { type: string }
      responses:
        "200":
          description: Coupon validation result
        "404":
          description: Coupon not found or expired

  /api/v1/webhooks/{storeSlug}/{provider}:
    post:
      operationId: handleWebhook
      summary: Payment provider webhook endpoint
      tags: [Webhooks]
      parameters:
        - name: storeSlug
          in: path
          required: true
          schema: { type: string }
        - name: provider
          in: path
          required: true
          schema: { type: string, enum: [stripe, razorpay, paypal] }
      requestBody:
        required: true
        content:
          application/json:
            schema: { type: object }
      responses:
        "200":
          description: Webhook processed
        "400":
          description: Invalid signature or payload

components:
  schemas:
    StorefrontProduct:
      type: object
      properties:
        id: { type: string, format: uuid }
        handle: { type: string }
        title: { type: string }
        description: { type: string, nullable: true }
        seo_title: { type: string, nullable: true }
        seo_description: { type: string, nullable: true }
        price_range:
          type: object
          properties:
            min: { type: string }
            max: { type: string }
            currency_code: { type: string }
        media:
          type: array
          items: { $ref: "#/components/schemas/Media" }
        variants:
          type: array
          items: { $ref: "#/components/schemas/Variant" }
        options:
          type: array
          items: { $ref: "#/components/schemas/Option" }
        categories:
          type: array
          items: { $ref: "#/components/schemas/Category" }

    Media:
      type: object
      properties:
        url: { type: string, format: uri }
        alt: { type: string, nullable: true }
        position: { type: integer }

    Variant:
      type: object
      properties:
        id: { type: string, format: uuid }
        sku: { type: string }
        price: { type: string }
        compare_at_price: { type: string, nullable: true }
        currency_code: { type: string }
        in_stock: { type: boolean }
        low_stock: { type: boolean }

    Option:
      type: object
      properties:
        name: { type: string }
        values:
          type: array
          items:
            type: object
            properties:
              label: { type: string }
              value: { type: string }

    Category:
      type: object
      properties:
        slug: { type: string }
        name: { type: string }

    PaginationMeta:
      type: object
      properties:
        page: { type: integer }
        page_size: { type: integer }
        total: { type: integer }

    CheckoutBody:
      type: object
      required: [idempotency_key, customer_email, items, shipping_address, shipping_service, payment_provider, subtotal]
      properties:
        idempotency_key: { type: string, format: uuid }
        customer_email: { type: string, format: email }
        customer_name: { type: string }
        items:
          type: array
          items: { $ref: "#/components/schemas/CheckoutItem" }
        shipping_address: { $ref: "#/components/schemas/Address" }
        shipping_service: { type: string }
        payment_provider: { type: string }
        subtotal: { type: string }
        coupon_code: { type: string }

    CheckoutItem:
      type: object
      properties:
        product_id: { type: string, format: uuid }
        variant_id: { type: string, format: uuid }
        title_snapshot: { type: string }
        sku_snapshot: { type: string }
        unit_price: { type: string }
        quantity: { type: integer, minimum: 1 }
        line_total: { type: string }
        currency_code: { type: string }
        image_url: { type: string, format: uri }

    Address:
      type: object
      required: [name, line1, city, country_code]
      properties:
        name: { type: string }
        line1: { type: string }
        line2: { type: string }
        city: { type: string }
        region: { type: string }
        postal_code: { type: string }
        country_code: { type: string, minLength: 2, maxLength: 2 }

    ShippingRateRequest:
      type: object
      properties:
        items:
          type: array
          items:
            type: object
            properties:
              product_id: { type: string }
              variant_id: { type: string }
              quantity: { type: integer }
              weight_grams: { type: integer }
        ship_to: { $ref: "#/components/schemas/Address" }

    ShippingRate:
      type: object
      properties:
        service: { type: string }
        price: { type: string }
        currency_code: { type: string }
        estimated_days: { type: integer }

    PaymentMethod:
      type: object
      properties:
        provider: { type: string, enum: [stripe, razorpay, paypal] }
```

**Verify:** Paste into https://editor.swagger.io/ — should render without errors. Alternatively:
```bash
npx @redocly/cli lint docs/api/storefront-api.yaml
```

---

## Task 7 — k6 load test scripts

### Step 1: Create directory

```bash
mkdir -p scripts/loadtest
```

### Step 2: Storefront product list (read-heavy)

**File:** `scripts/loadtest/storefront-products.js` (new file)

```javascript
// k6 load test: Storefront product listing
//
// Usage:
//   k6 run --env BASE_URL=http://localhost:8080 \
//          --env STORE_SLUG=demo \
//          scripts/loadtest/storefront-products.js
//
// Simulates customer browsing: list products, paginate, search.

import http from "k6/http";
import { check, sleep } from "k6";
import { Rate, Trend } from "k6/metrics";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const STORE_SLUG = __ENV.STORE_SLUG || "demo";

const errorRate = new Rate("errors");
const listLatency = new Trend("list_latency", true);

export const options = {
  stages: [
    { duration: "30s", target: 10 },   // ramp up
    { duration: "2m", target: 50 },    // sustained load
    { duration: "30s", target: 0 },    // ramp down
  ],
  thresholds: {
    http_req_duration: ["p(95)<500", "p(99)<1000"],
    errors: ["rate<0.01"],
  },
};

export default function () {
  const headers = {
    "Content-Type": "application/json",
    "X-Store-Slug": STORE_SLUG,
  };

  // List products — page 1
  const listRes = http.get(
    `${BASE_URL}/api/v1/storefront/products?page=1&page_size=24`,
    { headers, tags: { name: "list_products" } }
  );
  listLatency.add(listRes.timings.duration);
  check(listRes, {
    "list products: status 200": (r) => r.status === 200,
    "list products: has data": (r) => {
      const body = r.json();
      return body && Array.isArray(body.data);
    },
  }) || errorRate.add(1);

  sleep(1);

  // List products — page 2
  const page2Res = http.get(
    `${BASE_URL}/api/v1/storefront/products?page=2&page_size=24`,
    { headers, tags: { name: "list_products_page2" } }
  );
  check(page2Res, {
    "page 2: status 200": (r) => r.status === 200,
  }) || errorRate.add(1);

  sleep(0.5);

  // Search products
  const searchRes = http.get(
    `${BASE_URL}/api/v1/storefront/products?search=test&page_size=24`,
    { headers, tags: { name: "search_products" } }
  );
  check(searchRes, {
    "search: status 200": (r) => r.status === 200,
  }) || errorRate.add(1);

  sleep(1);
}
```

### Step 3: Storefront checkout flow (write-heavy)

**File:** `scripts/loadtest/storefront-checkout.js` (new file)

```javascript
// k6 load test: Checkout flow
//
// Usage:
//   k6 run --env BASE_URL=http://localhost:8080 \
//          --env STORE_SLUG=demo \
//          --env PRODUCT_ID=<uuid> \
//          --env VARIANT_ID=<uuid> \
//          scripts/loadtest/storefront-checkout.js
//
// Simulates the full purchase flow: list products -> fetch shipping ->
// fetch payment methods -> submit checkout.

import http from "k6/http";
import { check, sleep } from "k6";
import { Rate, Trend } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const STORE_SLUG = __ENV.STORE_SLUG || "demo";
const PRODUCT_ID = __ENV.PRODUCT_ID || "00000000-0000-0000-0000-000000000001";
const VARIANT_ID = __ENV.VARIANT_ID || "00000000-0000-0000-0000-000000000001";

const checkoutErrors = new Rate("checkout_errors");
const checkoutLatency = new Trend("checkout_latency", true);

export const options = {
  stages: [
    { duration: "15s", target: 5 },
    { duration: "1m", target: 20 },
    { duration: "15s", target: 0 },
  ],
  thresholds: {
    http_req_duration: ["p(95)<2000"],
    checkout_errors: ["rate<0.05"],
  },
};

export default function () {
  const headers = {
    "Content-Type": "application/json",
    "X-Store-Slug": STORE_SLUG,
  };

  // 1. Fetch payment methods
  const pmRes = http.get(
    `${BASE_URL}/api/v1/storefront/payment-methods`,
    { headers, tags: { name: "payment_methods" } }
  );
  check(pmRes, { "payment methods: 200": (r) => r.status === 200 });

  sleep(0.5);

  // 2. Fetch shipping rates
  const shippingBody = JSON.stringify({
    items: [{ product_id: PRODUCT_ID, variant_id: VARIANT_ID, quantity: 1, weight_grams: 500 }],
    ship_to: {
      line1: "123 Load Test St",
      city: "Mumbai",
      country_code: "IN",
      postal_code: "400001",
    },
  });
  const shipRes = http.post(
    `${BASE_URL}/api/v1/storefront/shipping/rates`,
    shippingBody,
    { headers, tags: { name: "shipping_rates" } }
  );
  check(shipRes, { "shipping rates: 200": (r) => r.status === 200 });

  sleep(0.5);

  // 3. Submit checkout
  const checkoutBody = JSON.stringify({
    idempotency_key: uuidv4(),
    customer_email: `loadtest-${uuidv4().slice(0, 8)}@example.com`,
    customer_name: "k6 Load Test",
    items: [
      {
        product_id: PRODUCT_ID,
        variant_id: VARIANT_ID,
        title_snapshot: "Load Test Product",
        sku_snapshot: "LT-001",
        unit_price: "10.00",
        quantity: 1,
        line_total: "10.00",
        currency_code: "INR",
      },
    ],
    shipping_address: {
      name: "k6 Load Test",
      line1: "123 Load Test St",
      city: "Mumbai",
      country_code: "IN",
      postal_code: "400001",
    },
    shipping_service: "standard",
    payment_provider: "stripe",
    subtotal: "10.00",
  });

  const start = Date.now();
  const checkoutRes = http.post(
    `${BASE_URL}/api/v1/storefront/checkout`,
    checkoutBody,
    { headers, tags: { name: "submit_checkout" } }
  );
  checkoutLatency.add(Date.now() - start);

  const ok = check(checkoutRes, {
    "checkout: status 201 or 409": (r) => r.status === 201 || r.status === 409,
  });
  if (!ok) checkoutErrors.add(1);

  sleep(2);
}
```

### Step 4: Admin product CRUD (mixed)

**File:** `scripts/loadtest/admin-products.js` (new file)

```javascript
// k6 load test: Admin product CRUD
//
// Usage:
//   k6 run --env BASE_URL=http://localhost:8080 \
//          --env STORE_ID=<uuid> \
//          --env TENANT_ID=<uuid> \
//          --env AUTH_TOKEN=<bearer-token> \
//          scripts/loadtest/admin-products.js
//
// Simulates admin listing, creating, and updating products.

import http from "k6/http";
import { check, sleep } from "k6";
import { Rate } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const STORE_ID = __ENV.STORE_ID || "00000000-0000-0000-0000-000000000001";
const TENANT_ID = __ENV.TENANT_ID || "00000000-0000-0000-0000-000000000001";
const AUTH_TOKEN = __ENV.AUTH_TOKEN || "test-token";

const errors = new Rate("errors");

export const options = {
  stages: [
    { duration: "15s", target: 5 },
    { duration: "1m", target: 15 },
    { duration: "15s", target: 0 },
  ],
  thresholds: {
    http_req_duration: ["p(95)<1000"],
    errors: ["rate<0.05"],
  },
};

export default function () {
  const headers = {
    "Content-Type": "application/json",
    Authorization: `Bearer ${AUTH_TOKEN}`,
    "X-Tenant-ID": TENANT_ID,
  };

  // 1. List products
  const listRes = http.get(
    `${BASE_URL}/api/v1/stores/${STORE_ID}/products?page=1&page_size=20`,
    { headers, tags: { name: "admin_list_products" } }
  );
  check(listRes, { "admin list: 200": (r) => r.status === 200 }) || errors.add(1);

  sleep(1);

  // 2. Create a draft product
  const handle = `loadtest-${uuidv4().slice(0, 8)}`;
  const createBody = JSON.stringify({
    title: `Load Test Product ${handle}`,
    handle: handle,
    status: "draft",
    description: "Created by k6 load test",
    variants: [
      {
        sku: `LT-${handle}`,
        price: "25.00",
        currency_code: "INR",
        track_inventory: false,
      },
    ],
  });

  const createRes = http.post(
    `${BASE_URL}/api/v1/stores/${STORE_ID}/products`,
    createBody,
    { headers, tags: { name: "admin_create_product" } }
  );
  const created = check(createRes, {
    "create: 201": (r) => r.status === 201,
  });
  if (!created) {
    errors.add(1);
    return;
  }

  const productId = createRes.json("data.id");

  sleep(0.5);

  // 3. Update the product
  const updateBody = JSON.stringify({
    title: `Updated ${handle}`,
    description: "Updated by k6",
  });
  const updateRes = http.patch(
    `${BASE_URL}/api/v1/stores/${STORE_ID}/products/${productId}`,
    updateBody,
    { headers, tags: { name: "admin_update_product" } }
  );
  check(updateRes, { "update: 200": (r) => r.status === 200 }) || errors.add(1);

  sleep(1);
}
```

### Step 5: Webhook burst test

**File:** `scripts/loadtest/webhook-burst.js` (new file)

```javascript
// k6 load test: Webhook ingestion burst
//
// Usage:
//   k6 run --env BASE_URL=http://localhost:8080 \
//          --env STORE_SLUG=demo \
//          scripts/loadtest/webhook-burst.js
//
// Simulates a burst of Stripe webhooks (e.g., after a provider outage
// delivers a backlog). Tests that the webhook endpoint handles high
// throughput without dropping events.

import http from "k6/http";
import { check } from "k6";
import { Rate } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const STORE_SLUG = __ENV.STORE_SLUG || "demo";

const webhookErrors = new Rate("webhook_errors");

export const options = {
  scenarios: {
    burst: {
      executor: "constant-arrival-rate",
      rate: 100,           // 100 requests per second
      timeUnit: "1s",
      duration: "30s",
      preAllocatedVUs: 50,
      maxVUs: 100,
    },
  },
  thresholds: {
    http_req_duration: ["p(95)<500"],
    webhook_errors: ["rate<0.01"],
  },
};

export default function () {
  // Simulate a Stripe payment_intent.succeeded webhook
  // Note: In real testing, the signature header must be valid.
  // This script tests throughput, not signature verification.
  const payload = JSON.stringify({
    id: `evt_${uuidv4().replace(/-/g, "").slice(0, 24)}`,
    type: "payment_intent.succeeded",
    data: {
      object: {
        id: `pi_${uuidv4().replace(/-/g, "").slice(0, 24)}`,
        amount: 2500,
        currency: "inr",
        status: "succeeded",
        metadata: {
          order_id: uuidv4(),
        },
      },
    },
  });

  const res = http.post(
    `${BASE_URL}/api/v1/webhooks/${STORE_SLUG}/stripe`,
    payload,
    {
      headers: {
        "Content-Type": "application/json",
        // Stripe-Signature would go here in a real test
        // "Stripe-Signature": "t=...,v1=..."
      },
      tags: { name: "stripe_webhook" },
    }
  );

  // Accept 200 (processed) or 400 (invalid signature — expected without
  // real Stripe-Signature). 5xx means the handler is broken.
  check(res, {
    "webhook: not 5xx": (r) => r.status < 500,
  }) || webhookErrors.add(1);
}
```

### Step 6: Create README for load tests

**File:** `scripts/loadtest/README.md` (new file)

```markdown
# Load Tests

k6 scripts for baseline performance measurement.

## Prerequisites

Install k6: https://k6.io/docs/get-started/installation/

## Running locally

Start marketplace-api locally, then:

```bash
# Product listing (read-heavy)
k6 run --env BASE_URL=http://localhost:8080 --env STORE_SLUG=demo \
  scripts/loadtest/storefront-products.js

# Checkout flow (write-heavy)
k6 run --env BASE_URL=http://localhost:8080 --env STORE_SLUG=demo \
  --env PRODUCT_ID=<uuid> --env VARIANT_ID=<uuid> \
  scripts/loadtest/storefront-checkout.js

# Admin CRUD (mixed)
k6 run --env BASE_URL=http://localhost:8080 \
  --env STORE_ID=<uuid> --env TENANT_ID=<uuid> --env AUTH_TOKEN=<token> \
  scripts/loadtest/admin-products.js

# Webhook burst
k6 run --env BASE_URL=http://localhost:8080 --env STORE_SLUG=demo \
  scripts/loadtest/webhook-burst.js
```

## Baseline targets (db-f1-micro)

| Scenario | Target RPS | p95 latency | Error rate |
|----------|-----------|-------------|------------|
| Product list | 50 | <500ms | <1% |
| Checkout | 20 | <2000ms | <5% |
| Admin CRUD | 15 | <1000ms | <5% |
| Webhook burst | 100 | <500ms | <1% |

Record results in `docs/runbooks/performance-baseline.md` after each run.
```

**Verify:** `k6 run --dry-run scripts/loadtest/storefront-products.js` parses without errors.

---

## Task 8 — Secrets rotation runbook

**File:** `docs/runbooks/secrets-rotation.md` (new file)

```markdown
# Secrets Rotation Runbook

## Overview

All production secrets are stored in GCP Secret Manager and synced to
Kubernetes via External Secrets Operator (ESO). Local development uses
`.env` files (never committed).

## GIP API Keys

**What:** Google Identity Platform Web API key used by frontends.

**Rotation steps:**
1. Go to GCP Console > APIs & Services > Credentials
2. Create a new API key with the same restrictions as the current one
3. Update the secret in GCP Secret Manager:
   ```bash
   echo -n "NEW_API_KEY" | gcloud secrets versions add gip-web-api-key --data-file=-
   ```
4. ESO will sync the new value to Kubernetes within its refresh interval
   (default: 1h). To force immediate sync:
   ```bash
   kubectl annotate externalsecret gip-web-api-key -n platform \
     force-sync=$(date +%s) --overwrite
   ```
5. Restart affected pods to pick up the new env var:
   ```bash
   kubectl rollout restart deploy/auth-bff -n platform
   ```
6. Verify the old key still works during the transition period
7. Delete the old API key from GCP Console after 24h

**Affected services:** auth-bff, all Next.js apps (via build-time
NEXT_PUBLIC_GIP_API_KEY)

**Note:** Next.js apps bake the API key into the client bundle at build
time. A full rebuild + redeploy is needed for frontend key rotation.

## OAuth Client Secret

**What:** GIP OAuth client secret for OIDC flows.

**Rotation steps:**
1. Go to GCP Console > APIs & Services > Credentials > OAuth 2.0 Client
2. Click "Reset Secret" — this generates a new secret immediately
3. Update in Secret Manager:
   ```bash
   echo -n "NEW_CLIENT_SECRET" | gcloud secrets versions add gip-oauth-client-secret --data-file=-
   ```
4. Force ESO sync + restart auth-bff (see GIP API Keys above)
5. Verify login flow works with the new secret

**Warning:** The old secret is immediately invalidated when you click
"Reset Secret". Plan for a brief auth outage (~2 min during pod restart).

## Stripe / Razorpay / PayPal Keys

**What:** Per-merchant payment provider API keys and webhook secrets.

**Rotation steps:**
1. Merchant rotates keys in their Stripe/Razorpay/PayPal dashboard
2. Merchant updates keys in Mark8ly Admin > Settings > Payments
3. The new keys are encrypted via the Encryptor (P0 Task 2) and stored
   in `payment_gateway_configs`
4. No pod restart needed — keys are read from DB per request

**Note:** Webhook secrets are separate from API keys (P0 Task 6). Both
must be rotated independently.

## Session Encryption Key

**What:** AES key used by auth-bff to encrypt session cookies.

**Rolling rotation strategy:**
1. Generate a new key:
   ```bash
   openssl rand -base64 32
   ```
2. Update Secret Manager with BOTH old and new keys (comma-separated):
   ```bash
   echo -n "NEW_KEY,OLD_KEY" | gcloud secrets versions add session-encrypt-key --data-file=-
   ```
3. auth-bff reads both keys: encrypts with the first, decrypts trying
   each in order. This allows existing sessions to remain valid.
4. After 24h (max session lifetime), remove the old key:
   ```bash
   echo -n "NEW_KEY" | gcloud secrets versions add session-encrypt-key --data-file=-
   ```
5. Force ESO sync + restart auth-bff

## KMS Key (Envelope Encryption)

**What:** GCP KMS key used for encrypting sensitive DB columns.

**Rotation:** Automatic via GCP KMS key rotation policy.
- Configure in Terraform or GCP Console:
  ```bash
  gcloud kms keys update marketplace-dek \
    --keyring=mark8ly --location=asia-south1 \
    --rotation-period=90d --next-rotation-time=<timestamp>
  ```
- KMS handles version management — old ciphertexts can still be
  decrypted with older key versions. No application changes needed.
- Re-encryption of existing data is optional but recommended annually.

## Database Credentials

**What:** Cloud SQL user passwords for per-service databases.

**Rotation steps:**
1. Generate new password:
   ```bash
   openssl rand -base64 24
   ```
2. Update Cloud SQL user:
   ```bash
   gcloud sql users set-password marketplace_user \
     --instance=tesserix-db --password="NEW_PASSWORD"
   ```
3. Update Secret Manager:
   ```bash
   echo -n "NEW_PASSWORD" | gcloud secrets versions add marketplace-db-password --data-file=-
   ```
4. Force ESO sync:
   ```bash
   kubectl annotate externalsecret marketplace-db-password -n marketplace \
     force-sync=$(date +%s) --overwrite
   ```
5. Restart the service:
   ```bash
   kubectl rollout restart deploy/marketplace-api -n marketplace
   ```

**Warning:** There is a brief window (~30s) where the old password is
invalid and the service hasn't restarted yet. Schedule during low
traffic.

## GHCR_PAT (CI)

**What:** GitHub Personal Access Token for pushing container images.

**Rotation steps:**
1. Generate a new classic PAT with `write:packages` scope at
   github.com/settings/tokens
2. Update the repo secret:
   ```bash
   gh secret set GHCR_PAT --body "NEW_TOKEN" --repo tesserix/mark8ly
   ```
3. Trigger a CI run to verify the new token works
4. Revoke the old PAT

## TESSERIX_K8S_BOT (CI)

**What:** PAT for cross-repo PR creation on tesserix-k8s.

**Rotation steps:**
1. Generate a new classic PAT with `repo` scope
2. Update:
   ```bash
   gh secret set TESSERIX_K8S_BOT --body "NEW_TOKEN" --repo tesserix/mark8ly
   ```
3. Push to main to trigger the bump-k8s job and verify it can open a PR
4. Revoke the old PAT

## Schedule

| Secret | Rotation frequency | Owner |
|--------|-------------------|-------|
| GIP API Key | Annually or on compromise | Platform team |
| OAuth Client Secret | Annually or on compromise | Platform team |
| Payment keys | Per merchant, as needed | Merchant |
| Session key | Quarterly | Platform team |
| KMS key | Auto (90d) | GCP KMS |
| DB credentials | Quarterly | Platform team |
| GHCR_PAT | Annually | CI owner |
| TESSERIX_K8S_BOT | Annually | CI owner |
```

**Verify:** The file is well-formed Markdown and all referenced commands are correct for the project's GCP setup.

---

## Verification checklist

After all tasks are complete:

- [ ] `go vet ./...` passes in `services/marketplace-api`
- [ ] `go build ./...` passes in `services/marketplace-api`
- [ ] `go test ./...` passes in `services/marketplace-api`
- [ ] `govulncheck ./...` reports no actionable issues
- [ ] `npm audit` returns 0 vulnerabilities in `apps/storefront`
- [ ] `npm audit` returns 0 vulnerabilities in `apps/admin`
- [ ] `npm run build` passes in `apps/storefront`
- [ ] `npm run build` passes in `apps/admin`
- [ ] `.github/dependabot.yml` is valid YAML and recognized by GitHub
- [ ] `docs/runbooks/database-migrations.md` exists and is accurate
- [ ] `docs/api/storefront-api.yaml` validates as OpenAPI 3.1
- [ ] `scripts/loadtest/*.js` parse without errors (`k6 run --dry-run`)
- [ ] `docs/runbooks/secrets-rotation.md` exists and covers all secret types

---

## Files modified / created (summary)

| File | Change |
|------|--------|
| `services/marketplace-api/go.mod` | Updated dependency versions |
| `services/marketplace-api/go.sum` | Updated checksums |
| `apps/storefront/package.json` | `npm update` |
| `apps/storefront/package-lock.json` | Updated lockfile |
| `apps/admin/package.json` | `npm update` |
| `apps/admin/package-lock.json` | Updated lockfile |
| `.github/dependabot.yml` | New file — Dependabot configuration |
| `docs/runbooks/database-migrations.md` | New file — migration runbook |
| `docs/api/storefront-api.yaml` | New file — OpenAPI spec |
| `scripts/loadtest/storefront-products.js` | New file — k6 product list test |
| `scripts/loadtest/storefront-checkout.js` | New file — k6 checkout flow test |
| `scripts/loadtest/admin-products.js` | New file — k6 admin CRUD test |
| `scripts/loadtest/webhook-burst.js` | New file — k6 webhook burst test |
| `scripts/loadtest/README.md` | New file — load test instructions |
| `docs/runbooks/secrets-rotation.md` | New file — secrets rotation runbook |
