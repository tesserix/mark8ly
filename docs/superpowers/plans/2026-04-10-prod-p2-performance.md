# Production Readiness P2 — Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate storefront images to Next.js Image component, audit and fix N+1 queries, add bundle size monitoring to CI.

**Architecture:** Replace all raw <img> with next/image across storefront app. Add GORM query logging in tests. Add @next/bundle-analyzer.

**Tech Stack:** Next.js 16 Image component, GORM logger, @next/bundle-analyzer, k6.

---

## Task 1 — Tighten `next.config.ts` remotePatterns for storefront

**File:** `apps/storefront/next.config.ts`

The storefront already has `remotePatterns` configured. Tighten the GCS pattern to restrict to `mark8ly-*` bucket prefixes per the spec, matching only production bucket paths.

**Current code (lines 14-19):**
```typescript
remotePatterns: [
  { protocol: "https", hostname: "storage.googleapis.com" },
  { protocol: "https", hostname: "*.storage.googleapis.com" },
  { protocol: "http", hostname: "localhost" },
  { protocol: "http", hostname: "fake-gcs-server" },
],
```

**Replace with:**
```typescript
remotePatterns: [
  { protocol: "https", hostname: "storage.googleapis.com", pathname: "/mark8ly-*/**" },
  { protocol: "https", hostname: "*.storage.googleapis.com", pathname: "/mark8ly-*/**" },
  { protocol: "http", hostname: "localhost" },
  { protocol: "http", hostname: "fake-gcs-server" },
],
```

**Why:** The `pathname` restriction ensures only images from `mark8ly-*` GCS buckets are optimized — prevents the Image component from proxying arbitrary GCS content.

**Verify:** `npm run build` in `apps/storefront` succeeds without warnings about `images` config.

---

## Task 2 — Migrate `<img>` to `next/image` in `apps/storefront/app/products/page.tsx`

**File:** `apps/storefront/app/products/page.tsx`

### Step 1: Add import

Add at the top of the file, after the existing imports:
```typescript
import Image from "next/image";
```

### Step 2: Replace the `<img>` in the `ProductCard` function

**Current code (lines 156-161):**
```tsx
{/* eslint-disable-next-line @next/next/no-img-element */}
<img
  src={cover.url}
  alt={cover.alt ?? product.title}
  className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-[1.02]"
/>
```

**Replace with:**
```tsx
<Image
  src={cover.url}
  alt={cover.alt ?? product.title}
  fill
  sizes="(max-width: 640px) 100vw, (max-width: 1024px) 50vw, 33vw"
  className="object-cover transition-transform duration-500 group-hover:scale-[1.02]"
/>
```

**Notes:**
- `fill` is used because the image container already has `aspect-square` and `overflow-hidden` — the Image component fills its `position: relative` parent.
- The container `<div>` already has `relative` via the `relative aspect-square` classes — no change needed there.
- Remove the `eslint-disable-next-line` comment since `next/image` is being used now.
- `sizes` gives the browser the correct download hint for responsive breakpoints matching the 1-col / 2-col / 3-col grid.

**Verify:** Run `npm run check-types` in `apps/storefront`. Load `/products` in dev and confirm images render with `srcset` and `sizes` in the HTML.

---

## Task 3 — Migrate `<img>` to `next/image` in `apps/storefront/components/FeaturedProducts.tsx`

**File:** `apps/storefront/components/FeaturedProducts.tsx`

### Step 1: Add import

Add after the `import Link from "next/link";` line:
```typescript
import Image from "next/image";
```

### Step 2: Replace the `<img>` in the inner `ProductCard` function

**Current code (lines 61-65):**
```tsx
{/* eslint-disable-next-line @next/next/no-img-element */}
<img
  src={cover.url}
  alt={cover.alt ?? product.title}
  className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-[1.02]"
/>
```

**Replace with:**
```tsx
<Image
  src={cover.url}
  alt={cover.alt ?? product.title}
  fill
  sizes="(max-width: 640px) 50vw, (max-width: 1024px) 33vw, 25vw"
  className="object-cover transition-transform duration-500 group-hover:scale-[1.02]"
/>
```

**Notes:**
- `sizes` differs from Task 2 because the FeaturedProducts grid is 2-col / 3-col / 4-col.
- Container already has `relative aspect-square overflow-hidden rounded-md` — no change needed.
- Remove the `eslint-disable-next-line` comment.

**Verify:** `npm run check-types` passes. Load the home page and inspect the `<img>` tag rendered by Next.js — confirm `srcset` is present.

---

## Task 4 — Migrate `<img>` to `next/image` in `apps/storefront/app/categories/[slug]/page.tsx`

**File:** `apps/storefront/app/categories/[slug]/page.tsx`

### Step 1: Add import

Add after the `import Link from "next/link";` line:
```typescript
import Image from "next/image";
```

### Step 2: Replace the `<img>` in the `ProductCard` function

**Current code (lines 136-140):**
```tsx
{/* eslint-disable-next-line @next/next/no-img-element */}
<img
  src={cover.url}
  alt={cover.alt ?? product.title}
  className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-[1.02]"
/>
```

**Replace with:**
```tsx
<Image
  src={cover.url}
  alt={cover.alt ?? product.title}
  fill
  sizes="(max-width: 640px) 100vw, (max-width: 1024px) 50vw, 33vw"
  className="object-cover transition-transform duration-500 group-hover:scale-[1.02]"
/>
```

**Notes:**
- Same grid as `products/page.tsx` (1/2/3 cols) so same `sizes`.
- Container has `relative aspect-square` — no change needed.
- Remove the `eslint-disable-next-line` comment.

**Verify:** `npm run check-types` passes.

---

## Task 5 — Migrate `<img>` to `next/image` in `apps/storefront/components/MediaGallery.tsx`

**File:** `apps/storefront/components/MediaGallery.tsx`

This is a `"use client"` component, which is fine — `next/image` works in client components.

### Step 1: Add import

Add after the `import { useState } from "react";` line:
```typescript
import Image from "next/image";
```

### Step 2: Replace the main image `<img>` (line 37)

**Current code (lines 36-40):**
```tsx
{/* eslint-disable-next-line @next/next/no-img-element */}
<img
  src={active.url}
  alt={active.alt ?? productTitle}
  className="h-full w-full object-cover"
/>
```

**Replace with:**
```tsx
<Image
  src={active.url}
  alt={active.alt ?? productTitle}
  fill
  sizes="(max-width: 1024px) 100vw, 50vw"
  priority
  className="object-cover"
/>
```

**Notes:**
- `priority` on the main image since it's above the fold on the product detail page (LCP candidate).
- `sizes="(max-width: 1024px) 100vw, 50vw"` matches the 2-column grid on `products/[handle]/page.tsx` where MediaGallery takes the left half on `lg:`.
- Container already has `relative aspect-square overflow-hidden` — no change needed.
- Remove the `eslint-disable-next-line` comment.

### Step 3: Replace the thumbnail `<img>` (line 75)

**Current code (lines 74-78):**
```tsx
{/* eslint-disable-next-line @next/next/no-img-element */}
<img
  src={m.url}
  alt={m.alt ?? `${productTitle} ${i + 1}`}
  className="h-full w-full object-cover"
/>
```

**Replace with:**
```tsx
<Image
  src={m.url}
  alt={m.alt ?? `${productTitle} ${i + 1}`}
  fill
  sizes="64px"
  className="object-cover"
/>
```

**Notes:**
- Thumbnails are fixed 64px (`h-16 w-16`) — `sizes="64px"` tells the browser to fetch the smallest srcset variant.
- The thumbnail `<button>` container already has `relative ... overflow-hidden` — no change needed.
- Remove the `eslint-disable-next-line` comment.

**Verify:** `npm run check-types` passes. Load a product detail page in dev — confirm main image shows `priority` fetch and thumbnails are small downloads.

---

## Task 6 — Migrate `<img>` to `next/image` in `apps/storefront/app/cart/page.tsx`

**File:** `apps/storefront/app/cart/page.tsx`

This is a `"use client"` component.

### Step 1: Add import

Add after the `import Link from "next/link";` line:
```typescript
import Image from "next/image";
```

### Step 2: Replace the `<img>` in the `CartRow` function

**Current code (lines 116-120):**
```tsx
{/* eslint-disable-next-line @next/next/no-img-element */}
<img
  src={item.imageUrl}
  alt={item.title}
  className="h-20 w-20 shrink-0 rounded-md bg-[color:var(--paper-200)] object-cover"
/>
```

**Replace with:**
```tsx
<Image
  src={item.imageUrl}
  alt={item.title}
  width={80}
  height={80}
  className="h-20 w-20 shrink-0 rounded-md bg-[color:var(--paper-200)] object-cover"
/>
```

**Notes:**
- Cart thumbnails are fixed 80x80px (`h-20 w-20`). Use explicit `width`/`height` instead of `fill` since there's no `relative` wrapper — the image sits inline in a flex row.
- Remove the `eslint-disable-next-line` comment.

**Verify:** `npm run check-types` passes. Open `/cart` with items and confirm images render correctly.

---

## Task 7 — Migrate `<img>` to `next/image` in `apps/storefront/app/checkout/page.tsx`

**File:** `apps/storefront/app/checkout/page.tsx`

This is a `"use client"` component.

### Step 1: Add import

Add after the `import Link from "next/link";` line:
```typescript
import Image from "next/image";
```

### Step 2: Replace the `<img>` in the order summary items loop

**Current code (lines 294-299):**
```tsx
{/* eslint-disable-next-line @next/next/no-img-element */}
<img
  src={item.imageUrl}
  alt={item.title}
  className="h-16 w-16 shrink-0 rounded-md bg-[color:var(--paper-200)] object-cover"
/>
```

**Replace with:**
```tsx
<Image
  src={item.imageUrl}
  alt={item.title}
  width={64}
  height={64}
  className="h-16 w-16 shrink-0 rounded-md bg-[color:var(--paper-200)] object-cover"
/>
```

**Notes:**
- Checkout item thumbnails are 64x64px (`h-16 w-16`). Same approach as cart — explicit width/height.
- Remove the `eslint-disable-next-line` comment.

**Verify:** `npm run check-types` passes.

---

## Task 8 — Migrate `<img>` to `next/image` in `apps/storefront/app/orders/[id]/page.tsx`

**File:** `apps/storefront/app/orders/[id]/page.tsx`

### Step 1: Add import

Add after the `import Link from "next/link";` line:
```typescript
import Image from "next/image";
```

### Step 2: Replace the `<img>` in the `OrderItemRow` function

**Current code (lines 185-189):**
```tsx
{/* eslint-disable-next-line @next/next/no-img-element */}
<img
  src={item.image_url}
  alt={item.title_snapshot}
  className="h-16 w-16 shrink-0 rounded-md bg-[color:var(--paper-200)] object-cover"
/>
```

**Replace with:**
```tsx
<Image
  src={item.image_url}
  alt={item.title_snapshot}
  width={64}
  height={64}
  className="h-16 w-16 shrink-0 rounded-md bg-[color:var(--paper-200)] object-cover"
/>
```

**Notes:**
- Same pattern as checkout. 64x64 fixed thumbnails.
- Remove the `eslint-disable-next-line` comment.

**Verify:** `npm run check-types` passes.

---

## Task 9 — N+1 query audit with GORM logging

**Files:**
- `services/marketplace-api/internal/product/repository.go` (review only)
- `services/marketplace-api/internal/order/repository.go` (review + potential fix)
- `services/marketplace-api/internal/product/repository_integration_test.go` (add logging)

### Step 1: Review product repository

The product repository already uses proper `Preload` chains for all list/get operations:
- `ListAdmin` (line ~206): `Preload("Options").Preload("Options.Values").Preload("Variants").Preload("Variants.OptionValueLinks").Preload("Media")` — correct, no N+1.
- `ListPublished` (line ~278): same Preload chain — correct.
- `GetPublishedByHandle` (line ~348): same Preload chain — correct.
- `GetByIDForStore` (line ~382): same Preload chain — correct.

**Action:** No changes needed for the product repository. Document this finding.

### Step 2: Review order repository

The order repository `GetByID` (line ~80) uses three separate queries:
```go
// Current pattern (pseudocode from repository.go):
// 1. SELECT * FROM orders WHERE id = ?
// 2. SELECT * FROM order_items WHERE order_id = ?
// 3. SELECT * FROM order_addresses WHERE order_id = ?
```

This is **not** N+1 — it's 3 fixed queries regardless of result size. This is acceptable.

**Action:** No changes needed. Document this finding.

### Step 3: Add GORM query logger to integration tests

Create a test helper that enables verbose query logging so future N+1 regressions are caught during development.

**File:** `services/marketplace-api/internal/testutil/query_logger.go` (new file)

```go
package testutil

import (
	"log"
	"os"
	"time"

	"gorm.io/gorm/logger"
)

// VerboseQueryLogger returns a GORM logger that prints every SQL query
// to stdout. Use in integration tests to detect unexpected query counts.
//
//	db.Session(&gorm.Session{Logger: testutil.VerboseQueryLogger()})
func VerboseQueryLogger() logger.Interface {
	return logger.New(
		log.New(os.Stdout, "\n[GORM-TEST] ", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Info,
			IgnoreRecordNotFoundError: false,
			Colorful:                  true,
		},
	)
}
```

### Step 4: Add query-count assertion helper

**File:** `services/marketplace-api/internal/testutil/query_counter.go` (new file)

```go
package testutil

import (
	"context"
	"sync/atomic"

	"gorm.io/gorm"
)

// QueryCounter wraps a GORM DB and counts the number of queries executed.
// Use to assert an operation runs a bounded number of queries.
type QueryCounter struct {
	count atomic.Int64
}

// NewQueryCounter registers a GORM callback that increments the counter
// on every query. Returns the counter and a cleanup function.
func NewQueryCounter(db *gorm.DB) (*QueryCounter, func()) {
	qc := &QueryCounter{}
	callbackName := "test:query_counter"

	db.Callback().Query().Before("gorm:query").Register(callbackName, func(d *gorm.DB) {
		qc.count.Add(1)
	})
	db.Callback().Create().Before("gorm:create").Register(callbackName+"_create", func(d *gorm.DB) {
		qc.count.Add(1)
	})

	cleanup := func() {
		_ = db.Callback().Query().Remove(callbackName)
		_ = db.Callback().Create().Remove(callbackName + "_create")
	}
	return qc, cleanup
}

// Count returns the number of queries executed since creation.
func (qc *QueryCounter) Count() int64 {
	return qc.count.Load()
}

// Reset sets the counter back to zero.
func (qc *QueryCounter) Reset() {
	qc.count.Store(0)
}
```

### Step 5: Add a sample N+1 detection test

Add the following test to `services/marketplace-api/internal/product/repository_integration_test.go` (append to existing file):

```go
func TestListAdmin_QueryCount(t *testing.T) {
	// This test verifies that ListAdmin executes a bounded number of
	// queries regardless of the number of products returned. If this
	// test starts failing with a higher count, an N+1 regression was
	// introduced.
	if testing.Short() {
		t.Skip("integration test")
	}

	db := setupTestDB(t) // existing test helper
	qc, cleanup := testutil.NewQueryCounter(db)
	defer cleanup()

	// Seed 5 products with variants, options, and media
	// (use existing test seed helpers)

	qc.Reset()
	_, _, err := repo.ListAdmin(context.Background(), product.ListAdminQuery{
		StoreID:  testStoreID,
		TenantID: testTenantID,
		Page:     1,
		PageSize: 50,
	})
	require.NoError(t, err)

	// GORM Preload issues: 1 count + 1 products + 1 options + 1 values
	// + 1 variants + 1 option_value_links + 1 media = 7 queries max.
	// Allow headroom for count query.
	assert.LessOrEqual(t, qc.Count(), int64(8),
		"ListAdmin should not issue more than 8 queries (N+1 detected)")
}
```

**Verify:** `cd services/marketplace-api && go vet ./internal/testutil/...` passes. Integration tests pass with `go test ./internal/product/ -run TestListAdmin_QueryCount -count=1`.

---

## Task 10 — Add `@next/bundle-analyzer` to storefront

### Step 1: Install the package

```bash
cd apps/storefront && npm install --save-dev @next/bundle-analyzer
```

### Step 2: Update `apps/storefront/next.config.ts`

**Current full file:**
```typescript
import path from "node:path";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  outputFileTracingRoot: path.join(__dirname, "../.."),
  reactStrictMode: true,
  images: {
    remotePatterns: [
      { protocol: "https", hostname: "storage.googleapis.com", pathname: "/mark8ly-*/**" },
      { protocol: "https", hostname: "*.storage.googleapis.com", pathname: "/mark8ly-*/**" },
      { protocol: "http", hostname: "localhost" },
      { protocol: "http", hostname: "fake-gcs-server" },
    ],
  },
};

export default nextConfig;
```

**Replace the entire file with:**
```typescript
import path from "node:path";
import type { NextConfig } from "next";

const withBundleAnalyzer = process.env.ANALYZE === "true"
  ? (await import("@next/bundle-analyzer")).default({ enabled: true })
  : (config: NextConfig) => config;

const nextConfig: NextConfig = {
  output: "standalone",
  outputFileTracingRoot: path.join(__dirname, "../.."),
  reactStrictMode: true,
  images: {
    remotePatterns: [
      { protocol: "https", hostname: "storage.googleapis.com", pathname: "/mark8ly-*/**" },
      { protocol: "https", hostname: "*.storage.googleapis.com", pathname: "/mark8ly-*/**" },
      { protocol: "http", hostname: "localhost" },
      { protocol: "http", hostname: "fake-gcs-server" },
    ],
  },
};

export default withBundleAnalyzer(nextConfig);
```

### Step 3: Add analyze script to `apps/storefront/package.json`

Add to the `"scripts"` object:
```json
"analyze": "ANALYZE=true next build"
```

### Step 4: Add `.next/analyze` to `.gitignore`

Append to the repo root `.gitignore`:
```
# Bundle analyzer output
.next/analyze
```

**Verify:** `cd apps/storefront && ANALYZE=true npm run build` opens the bundle visualization in the browser. Confirm the total JS bundle size and identify the largest chunks.

---

## Task 11 — Add `@next/bundle-analyzer` to admin

### Step 1: Install the package

```bash
cd apps/admin && npm install --save-dev @next/bundle-analyzer
```

### Step 2: Update `apps/admin/next.config.ts`

**Current full file:**
```typescript
import path from "node:path";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  outputFileTracingRoot: path.join(__dirname, "../.."),
  reactStrictMode: true,
};

export default nextConfig;
```

**Replace the entire file with:**
```typescript
import path from "node:path";
import type { NextConfig } from "next";

const withBundleAnalyzer = process.env.ANALYZE === "true"
  ? (await import("@next/bundle-analyzer")).default({ enabled: true })
  : (config: NextConfig) => config;

const nextConfig: NextConfig = {
  output: "standalone",
  outputFileTracingRoot: path.join(__dirname, "../.."),
  reactStrictMode: true,
};

export default withBundleAnalyzer(nextConfig);
```

### Step 3: Add analyze script to `apps/admin/package.json`

Add to the `"scripts"` object:
```json
"analyze": "ANALYZE=true next build"
```

**Verify:** `cd apps/admin && ANALYZE=true npm run build` opens the bundle visualization.

---

## Task 12 — CI bundle size recording

**File:** `.github/workflows/ci.yml`

Add a new step to the existing `node` job, after the turbo build step:

```yaml
      - name: Record storefront bundle size
        if: github.event_name == 'pull_request'
        run: |
          set -euo pipefail
          # Parse the .next build output manifest for total JS size
          STOREFRONT_DIR="apps/storefront/.next"
          if [ -d "$STOREFRONT_DIR" ]; then
            TOTAL_JS=$(find "$STOREFRONT_DIR/static" -name '*.js' -exec wc -c {} + | tail -1 | awk '{print $1}')
            TOTAL_JS_KB=$((TOTAL_JS / 1024))
            echo "### Storefront bundle: ${TOTAL_JS_KB}KB total JS" >> "$GITHUB_STEP_SUMMARY"

            # Warn if total JS exceeds 500KB (adjust threshold as needed)
            if [ "$TOTAL_JS_KB" -gt 500 ]; then
              echo "::warning::Storefront JS bundle is ${TOTAL_JS_KB}KB (threshold: 500KB)"
            fi
          fi

      - name: Record admin bundle size
        if: github.event_name == 'pull_request'
        run: |
          set -euo pipefail
          ADMIN_DIR="apps/admin/.next"
          if [ -d "$ADMIN_DIR" ]; then
            TOTAL_JS=$(find "$ADMIN_DIR/static" -name '*.js' -exec wc -c {} + | tail -1 | awk '{print $1}')
            TOTAL_JS_KB=$((TOTAL_JS / 1024))
            echo "### Admin bundle: ${TOTAL_JS_KB}KB total JS" >> "$GITHUB_STEP_SUMMARY"

            if [ "$TOTAL_JS_KB" -gt 800 ]; then
              echo "::warning::Admin JS bundle is ${TOTAL_JS_KB}KB (threshold: 800KB)"
            fi
          fi
```

**Notes:**
- Only runs on PRs (not pushes to main) to surface the size in PR checks.
- Admin threshold is higher (800KB) because it includes Recharts, dnd-kit, framer-motion, react-hook-form, and papaparse.
- Uses `$GITHUB_STEP_SUMMARY` for visibility in the PR.

**Verify:** Create a test PR and confirm the bundle size step runs and reports sizes in the step summary.

---

## Verification checklist

After all tasks are complete:

- [ ] `npm run check-types` passes in `apps/storefront`
- [ ] `npm run check-types` passes in `apps/admin`
- [ ] `npm run build` passes in `apps/storefront` (no `@next/next/no-img-element` warnings remaining)
- [ ] `npm run build` passes in `apps/admin`
- [ ] `go vet ./...` passes in `services/marketplace-api`
- [ ] `go test ./...` passes in `services/marketplace-api`
- [ ] No `eslint-disable-next-line @next/next/no-img-element` comments remain in `apps/storefront/`
- [ ] Product pages render images with `srcset` attribute in dev
- [ ] `ANALYZE=true npm run build` in storefront opens bundle analyzer
- [ ] Product detail page main image has `fetchpriority="high"` in rendered HTML (from `priority` prop)

---

## Files modified (summary)

| File | Change |
|------|--------|
| `apps/storefront/next.config.ts` | Tighten `pathname` on remotePatterns, add bundle analyzer |
| `apps/storefront/app/products/page.tsx` | `<img>` to `<Image>` |
| `apps/storefront/components/FeaturedProducts.tsx` | `<img>` to `<Image>` |
| `apps/storefront/app/categories/[slug]/page.tsx` | `<img>` to `<Image>` |
| `apps/storefront/components/MediaGallery.tsx` | 2x `<img>` to `<Image>` |
| `apps/storefront/app/cart/page.tsx` | `<img>` to `<Image>` |
| `apps/storefront/app/checkout/page.tsx` | `<img>` to `<Image>` |
| `apps/storefront/app/orders/[id]/page.tsx` | `<img>` to `<Image>` |
| `apps/storefront/package.json` | Add `@next/bundle-analyzer`, `analyze` script |
| `apps/admin/next.config.ts` | Add bundle analyzer |
| `apps/admin/package.json` | Add `@next/bundle-analyzer`, `analyze` script |
| `services/marketplace-api/internal/testutil/query_logger.go` | New file — verbose GORM logger for tests |
| `services/marketplace-api/internal/testutil/query_counter.go` | New file — query count assertion helper |
| `services/marketplace-api/internal/product/repository_integration_test.go` | Add N+1 detection test |
| `.github/workflows/ci.yml` | Add bundle size recording steps |
| `.gitignore` | Add `.next/analyze` |
