# Products M7c — Admin UI: Variants + Rich Media Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `ProductForm` (from M7b) with three new tabs — **Media**, **Options**, **Variants** — supporting the full variant matrix editor (per-row price/sku/weight/stock + column bulk actions) and the rich media editor (GCS signed-URL upload via M5b, drag-reorder, delete, set primary, alt text, per-variant image assignment, client-side crop/rotate with re-editable originals, and bulk upload progress). No new page routes; everything hangs off the existing `/products/new` and `/products/:id` pages.

**Architecture:** `ProductForm` remains the single RHF form root; the three new tabs are siblings of the existing General tab. Variants are **derived from options** through a pure `generateVariants()` helper that preserves matched variant data by sorted-key equality. Media uploads use a two-step flow (`POST signed-url` → direct `PUT` to GCS → `POST /media` finalize) and crop happens client-side before PUT via `react-easy-crop`. Re-cropping reopens the crop dialog with the *original* blob fetched via `gcs_path_original`. Save path is a single `PATCH /products/:id` with the full aggregate (options, variants, media, categories, removed-variant ids).

**Tech Stack:** Next.js 16 (App Router + server components), React 19, Tailwind v4, `@tesserix/web` v1.7.1 primitives, `@repo/ui` promoted components, React Hook Form, Zod, `react-easy-crop`, `@dnd-kit/sortable`, Vitest + RTL, Playwright 1.59+, Paper · Ink · Moss design tokens. Backend: Go 1.26 / Gin / GORM / Postgres under `services/marketplace-api`.

**Design Authority:** `docs/superpowers/specs/2026-04-10-products-admin-ui-slice-2-design.md` §2, §5.1, §6 M7c, §7 landmines 1–5 and 16.

---

## Status

> **Pending.** All tasks open. Current branch: none (start from `main`).

---

## Scope check

Extends `apps/admin/components/products/form/*`, adds new components under `apps/admin/components/products/{media,options,variants}/`, adds `apps/admin/lib/products/generateVariants.ts`, extends `apps/admin/lib/api/marketplace-api.ts` with media + aggregate-PATCH clients, extends `apps/admin/app/products/actions.ts` server actions. **May add backend work** to `services/marketplace-api/internal/product/*` and a new migration (`000004_product_media_original.up.sql`) if Task 1 verification finds gaps — this is scoped into M7c, not deferred.

Spec sections authoritative for this milestone:
- Design spec §2 (all subsections — architecture, state model, data flow, variant rules, media flow, backend gate, libraries)
- Design spec §5.1 (testing strategy)
- Design spec §6 M7c (task sizing)
- Design spec §7 landmines 1, 2, 3, 4, 5, 16
- `mark8ly/.impeccable.md` — Paper · Ink · Moss design context
- Marketplace spec §7.2, §7.3 (product detail page), §7.8 (role-based UX), §7.9 (a11y), §7.10 (component reuse), §13.1.1 (permission map), §13.5 (no dialogs except hard delete)

**Out of scope (deferred to M7d/M7e or later):**
- Bulk actions on the list page (M7d)
- Copy-to-store dialog (M7d)
- CSV import/export (M7e)
- Inventory multi-location UI
- Product tags / metafields editor
- Digital product / download UI

---

## Decisions locked (from the spec)

1. **Server-side aggregate save.** A single `PATCH /api/v1/admin/stores/:storeId/products/:id` carries the full aggregate (options, variants, media, categories, `removed_variant_ids`). No per-entity endpoints for variants or options; media uses its own endpoints for upload/recrop but still round-trips through the aggregate PATCH on save for position + variant_id wiring.

2. **Variants are derived, not independently edited.** Users edit *options* → `generateVariants()` rebuilds the matrix → users then edit per-variant price/sku/stock/weight inline. Users never add a variant row without a corresponding option combination.

3. **Variant key = sorted `name=value` pairs joined by `|`.** Order-insensitive, stable across option reordering. Key equality preserves existing variant server `id`, price, sku, stock, weight.

4. **Orphans go to a `removed_variant_ids` bucket on form state.** They are deleted server-side on save, not silently forgotten.

5. **500-variant hard cap, enforced both client and server** — M7c Task 1 deliverable.

6. **Media upload is two-step.** Signed URL → PUT → finalize. 60-min expiry. Client retries once on 403 with a fresh URL.

7. **Cropping is client-side.** `react-easy-crop` → canvas → blob → PUT. Originals are stored under a dedicated `gcs_path_original` column on `product_media` so re-cropping is possible. M7c Task 1 deliverable.

8. **Bulk upload concurrency cap = 3.** No configuration, hard-coded.

9. **Design system:** Paper · Ink · Moss tokens, Source Serif 4 display, Source Sans 3 body, `@tesserix/web` primitives first, `@repo/ui` promoted flat-file components second. No new hex values. No new dialogs except hard-delete confirm.

10. **Impeccable chain is a gate, not a nice-to-have.** Task 0 verifies `mark8ly/.impeccable.md` exists. Task 14 runs the full chain (`frontend-design` → `critique` → `polish` → `arrange` → `typeset` → `audit` → `adapt`) with a `critique` score ≥ 7.5 threshold.

---

## File structure produced by M7c

### New frontend files

```
apps/admin/
  lib/products/
    generateVariants.ts
    generateVariants.test.ts
    variantKey.ts
    variantKey.test.ts
  lib/api/
    marketplace-api.ts              (modify — add media + PATCH aggregate clients)
  lib/validation/
    product-form.ts                 (modify — add variants/options/media schemas)
  components/products/
    form/
      ProductForm.tsx               (modify — tab shell + save path)
      ProductFormTabs.tsx           (new — controlled tab bar)
      GeneralTab.tsx                (new — extract current form body)
      MediaTab.tsx                  (new)
      OptionsTab.tsx                (new)
      VariantsTab.tsx               (new)
    media/
      MediaGrid.tsx                 (new — @dnd-kit sortable grid)
      MediaCard.tsx                 (new)
      MediaUploader.tsx             (new — drag-drop + progress)
      MediaCropDialog.tsx           (new — react-easy-crop)
      MediaAltTextInput.tsx         (new)
      mediaUploadClient.ts          (new — signed URL + retry + PUT)
      mediaUploadClient.test.ts     (new)
    options/
      OptionsEditor.tsx             (new)
      OptionRow.tsx                 (new)
    variants/
      VariantMatrixTable.tsx        (new)
      VariantRow.tsx                (new)
      VariantBulkBar.tsx            (new)
      VariantImagePicker.tsx        (new)
  app/products/
    actions.ts                      (modify — extend updateProductAction for aggregate)
  tests/e2e/
    products-variants-flow.spec.ts  (new — E2E 1)
    products-media-flow.spec.ts     (new — E2E 2)
```

### New backend files (only if Task 1 verification finds gaps)

```
services/marketplace-api/
  migrations/
    000004_product_media_original.up.sql      (only if gcs_path_original missing)
    000004_product_media_original.down.sql
  internal/product/
    repository.go                   (modify — aggregate PATCH coverage)
    service.go                      (modify — 500-variant cap + aggregate orchestration)
    service_aggregate_test.go       (new — integration tests for aggregate PATCH)
    models.go                       (modify — add GcsPathOriginal)
  internal/media/
    handlers.go                     (modify or new — signed_url, create, delete, recrop)
    service.go                      (modify — recrop orchestration)
    service_recrop_test.go          (new)
```

---

## New npm dependencies (`apps/admin/package.json`)

- `react-easy-crop@^5` (crop dialog)
- `@dnd-kit/core@^6` + `@dnd-kit/sortable@^8` + `@dnd-kit/utilities@^3` (drag-reorder — verify not already present first)

---

## Landmines

1. `PATCH /products/:id` may not currently accept the full aggregate. Do not start frontend work until Task 1 closes every gap.
2. `gcs_path_original` likely does not exist yet — treat as a required backend deliverable in Task 1, not an optional verification.
3. 500-variant cap must be enforced **both** client- and server-side. Client-only is insufficient (API consumers bypass).
4. Option reorder must not orphan variants. Variant key builder **sorts keys** before joining; write a test case for it explicitly.
5. Signed URL 403 retry: client must fetch a fresh URL and retry the PUT **once**, not loop. Second failure surfaces to the user.
6. Crop re-edit requires `gcs_path_original` to be a **separate GCS object**, not an overwrite. The recrop endpoint reads original → writes a new `gcs_path` → leaves original alone.
7. `react-easy-crop` returns a crop box in relative coordinates. The canvas draw step converts to absolute pixels using `createImage().naturalWidth/Height`. Off-by-one rounding can crop a pixel short — use `Math.round()`, not `Math.floor()`.
8. `@dnd-kit/sortable` requires a stable `id` per sortable item. Use the media row `id` for persisted rows and a `temp-<uuid>` for in-flight uploads; never use array index.
9. RHF `useFieldArray` with nested arrays (options → values; variants → option values) has known re-render footguns. Keep variants at the top level of the form state and derive via `generateVariants()` — do NOT nest variants inside options in RHF.
10. The `removed_variant_ids` bucket must survive form re-renders. Store it in a separate `useRef` or RHF custom field; don't let RHF prune it.
11. `@tesserix/web` Dialog primitive is the only dialog primitive allowed for the hard-delete confirm (per §13.5). Do not introduce a new modal system for the crop dialog — crop is a full-panel inline surface, not a dialog. (Revisit if that contradicts the design critique.)
12. Paper · Ink · Moss tokens only. No new hex values. No terracotta/sage/cream legacy aliases in new code.

---

## Task decomposition

**15 tasks**, dependency-ordered. Task 0 and Task 1 are gates. Tasks 2 and the `variantKey.ts`/`mediaUploadClient.ts` pure units can run in parallel after Task 1. Tasks 3–10 are frontend-serial because they all touch `ProductForm`. Tasks 12–15 are verification.

Legend: **R** = repository, **S** = service, **U** = unit/pure, **I** = integration (needs Postgres or GCS emulator), **C** = component (RTL), **E** = E2E (Playwright).

---

### Task 0: Impeccable design context check

**Files:** none (verification only)

**Scope:** Ensure `mark8ly/.impeccable.md` exists and is current before any UI code is written. This pins Paper · Ink · Moss design context for the `frontend-design` / `critique` / `polish` chain used in Task 14.

- [ ] **Step 1: Check for the file**

```bash
test -f mark8ly/.impeccable.md && echo "OK" || echo "MISSING"
```

Expected: `OK`. If `MISSING`, stop and run the `teach-impeccable` skill to generate it, then commit the result before continuing.

- [ ] **Step 2: Verify it mentions Paper · Ink · Moss**

```bash
grep -q "Paper" mark8ly/.impeccable.md && grep -q "Ink" mark8ly/.impeccable.md && grep -q "Moss" mark8ly/.impeccable.md && echo "OK" || echo "STALE"
```

Expected: `OK`. If `STALE`, re-run `teach-impeccable` and re-verify.

- [ ] **Step 3: Commit (only if regenerated)**

```bash
git add mark8ly/.impeccable.md
git commit -m "chore(impeccable): refresh design context for M7c"
```

---

### Task 1: Backend verification gate + fix sub-tasks

**Files (investigation):**
- Read: `services/marketplace-api/internal/product/repository.go`
- Read: `services/marketplace-api/internal/product/service.go`
- Read: `services/marketplace-api/internal/product/models.go`
- Read: `services/marketplace-api/internal/media/` (if present)
- Read: `services/marketplace-api/cmd/marketplace-api/main.go` (route registration)

**Scope:** Close every backend gap required by M7c before frontend work begins. This task can balloon — it's the single biggest risk in the milestone. Work through the 7 verification items from spec §2.7 in order. Each missing item becomes its own sub-task with its own tests and commits.

- [ ] **Step 1: Catalog current aggregate PATCH behavior**

Run the existing aggregate integration tests:

```bash
cd services/marketplace-api
go test ./internal/product/... -run Aggregate -v
```

Read `internal/product/service.go` Update method end-to-end. Write a markdown scratch file `.planning/m7c-backend-gaps.md` listing which of these are already supported and which are gaps:

1. Add/remove options in `PATCH`
2. Add/remove option values in `PATCH`
3. Variant matrix with mixed existing + new rows (some have `id`, some don't)
4. `removed_variant_ids` array: soft-delete vs hard-delete behavior
5. Dedicated media endpoints: `POST /media`, `DELETE /media/:id`, `POST /media/:id/recrop`
6. `variant_id` on `product_media` rows
7. `gcs_path_original` column + populated on every upload
8. 500-variant backend cap returning HTTP 422 with typed error

- [ ] **Step 2: Write a failing integration test for every gap**

For each missing item, add one integration test under `internal/product/service_aggregate_test.go` that drives the intended behavior through the service + repository layer against real Postgres. Example skeleton:

```go
func TestAggregatePatch_AddAndRemoveOptionValues(t *testing.T) {
    ctx, db, cleanup := testdb.Setup(t)
    defer cleanup()

    svc := product.NewService(product.NewRepository(db), nil, nil)
    storeID, _ := testdb.SeedStore(t, db)

    created, err := svc.Create(ctx, product.CreateInput{
        StoreID: storeID,
        Title:   "T-shirt",
        Options: []product.OptionInput{
            {Name: "Size", Values: []string{"S", "M"}},
        },
    })
    require.NoError(t, err)

    // Add value "L", remove value "S"
    updated, err := svc.Update(ctx, product.UpdateInput{
        ID:      created.ID,
        StoreID: storeID,
        Options: []product.OptionInput{
            {Name: "Size", Values: []string{"M", "L"}},
        },
    })
    require.NoError(t, err)
    require.Len(t, updated.Variants, 2)
    assertVariantKeys(t, updated.Variants, []string{"Size=M", "Size=L"})
}
```

Run it:

```bash
go test ./internal/product/... -run TestAggregatePatch_AddAndRemoveOptionValues -v
```

Expected: FAIL (behavior not implemented).

- [ ] **Step 3: Implement minimal fix for that gap**

Extend `product.Service.Update` (and `Repository.UpdateAggregate` if needed) to handle option value add/remove. Keep each fix commit small — one gap per commit. Prefer reusing the existing variant-diff logic; if none exists, add it in a dedicated helper file `internal/product/aggregate_diff.go`.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/product/... -run TestAggregatePatch_AddAndRemoveOptionValues -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/product/
git commit -m "feat(marketplace-api): support option value add/remove in aggregate PATCH (M7c gap)"
```

- [ ] **Step 6: Repeat Steps 2–5 for every remaining gap from Step 1**

For gap #7 (`gcs_path_original`), the fix involves a migration:

```sql
-- migrations/000004_product_media_original.up.sql
ALTER TABLE product_media ADD COLUMN gcs_path_original text NOT NULL DEFAULT '';
-- Backfill: treat existing images as "already cropped to themselves"
UPDATE product_media SET gcs_path_original = gcs_path WHERE gcs_path_original = '';
-- Drop the default once backfilled
ALTER TABLE product_media ALTER COLUMN gcs_path_original DROP DEFAULT;
```

```sql
-- migrations/000004_product_media_original.down.sql
ALTER TABLE product_media DROP COLUMN gcs_path_original;
```

Commit the migration and the model change together:

```bash
git add services/marketplace-api/migrations/000004_product_media_original.*.sql services/marketplace-api/internal/product/models.go
git commit -m "feat(marketplace-api): add product_media.gcs_path_original column (M7c gap)"
```

For gap #8 (500-variant cap):

```go
// internal/product/service.go
const MaxVariantsPerProduct = 500

func (s *Service) validateVariantCap(variants []VariantInput) error {
    if len(variants) > MaxVariantsPerProduct {
        return apperrors.Unprocessable("variant_cap_exceeded", fmt.Sprintf("product has %d variants, max is %d", len(variants), MaxVariantsPerProduct))
    }
    return nil
}
```

Add a test that creates 501 variants and asserts HTTP 422 with error code `variant_cap_exceeded`, then implement, then commit.

- [ ] **Step 7: Rerun full marketplace-api suite**

```bash
cd services/marketplace-api
go test ./... -race
```

Expected: all green.

- [ ] **Step 8: Update `.planning/m7c-backend-gaps.md`**

Mark every gap as `closed` with the commit hash. This doc gets deleted at the end of M7c but is useful while the task is in flight.

- [ ] **Step 9: Commit the gap log deletion at end of M7c (deferred)**

Noted here for Task 15.

---

### Task 2: `variantKey.ts` + `generateVariants.ts` pure logic (U)

**Files:**
- Create: `apps/admin/lib/products/variantKey.ts`
- Create: `apps/admin/lib/products/variantKey.test.ts`
- Create: `apps/admin/lib/products/generateVariants.ts`
- Create: `apps/admin/lib/products/generateVariants.test.ts`

**Scope:** Pure TypeScript helpers. No React. No DOM. Unit tests run in ms via Vitest. This is the foundation for the entire variants tab — get it rock solid before anything visual.

- [ ] **Step 1: Write the failing test for `variantKey.ts`**

```typescript
// apps/admin/lib/products/variantKey.test.ts
import { describe, it, expect } from "vitest";
import { buildVariantKey, parseVariantKey } from "./variantKey";

describe("buildVariantKey", () => {
  it("joins sorted option name=value pairs with |", () => {
    expect(
      buildVariantKey([
        { name: "Color", value: "Red" },
        { name: "Size", value: "M" },
      ])
    ).toBe("Color=Red|Size=M");
  });

  it("is order-insensitive (sorts by option name)", () => {
    const a = buildVariantKey([
      { name: "Size", value: "M" },
      { name: "Color", value: "Red" },
    ]);
    const b = buildVariantKey([
      { name: "Color", value: "Red" },
      { name: "Size", value: "M" },
    ]);
    expect(a).toBe(b);
    expect(a).toBe("Color=Red|Size=M");
  });

  it("handles single option", () => {
    expect(buildVariantKey([{ name: "Size", value: "M" }])).toBe("Size=M");
  });

  it("handles empty input as empty string", () => {
    expect(buildVariantKey([])).toBe("");
  });

  it("parseVariantKey is the inverse of buildVariantKey", () => {
    const pairs = [
      { name: "Color", value: "Red" },
      { name: "Size", value: "M" },
    ];
    const key = buildVariantKey(pairs);
    expect(parseVariantKey(key)).toEqual([
      { name: "Color", value: "Red" },
      { name: "Size", value: "M" },
    ]);
  });
});
```

- [ ] **Step 2: Run test (expect FAIL)**

```bash
cd apps/admin
npm run test -- variantKey
```

Expected: FAIL with module not found.

- [ ] **Step 3: Implement `variantKey.ts`**

```typescript
// apps/admin/lib/products/variantKey.ts
export interface OptionValuePair {
  name: string;
  value: string;
}

export function buildVariantKey(pairs: OptionValuePair[]): string {
  if (pairs.length === 0) return "";
  const sorted = [...pairs].sort((a, b) => a.name.localeCompare(b.name));
  return sorted.map((p) => `${p.name}=${p.value}`).join("|");
}

export function parseVariantKey(key: string): OptionValuePair[] {
  if (key === "") return [];
  return key.split("|").map((segment) => {
    const [name, value] = segment.split("=");
    return { name, value };
  });
}
```

- [ ] **Step 4: Run test (expect PASS)**

```bash
npm run test -- variantKey
```

- [ ] **Step 5: Commit**

```bash
git add apps/admin/lib/products/variantKey.ts apps/admin/lib/products/variantKey.test.ts
git commit -m "feat(admin): variantKey pure helpers with sorted-key stability (M7c)"
```

- [ ] **Step 6: Write the failing test for `generateVariants.ts`**

```typescript
// apps/admin/lib/products/generateVariants.test.ts
import { describe, it, expect } from "vitest";
import { generateVariants, type OptionDraft, type VariantDraft } from "./generateVariants";

const baseDefaults = { price: "19.99", sku: "", stock: 0, weight: 0 };

describe("generateVariants", () => {
  it("returns empty array when options are empty", () => {
    expect(generateVariants([], [], baseDefaults)).toEqual({
      variants: [],
      removedIds: [],
    });
  });

  it("generates cartesian product for two options", () => {
    const options: OptionDraft[] = [
      { name: "Color", values: ["Red", "Blue"] },
      { name: "Size", values: ["S", "M"] },
    ];
    const result = generateVariants(options, [], baseDefaults);
    expect(result.variants).toHaveLength(4);
    const keys = result.variants.map((v) => v.key).sort();
    expect(keys).toEqual([
      "Color=Blue|Size=M",
      "Color=Blue|Size=S",
      "Color=Red|Size=M",
      "Color=Red|Size=S",
    ]);
  });

  it("preserves existing variant data by matching key", () => {
    const options: OptionDraft[] = [
      { name: "Size", values: ["S", "M"] },
    ];
    const existing: VariantDraft[] = [
      {
        key: "Size=S",
        id: "var-1",
        price: "29.99",
        sku: "SHIRT-S",
        stock: 42,
        weight: 0.2,
      },
    ];
    const result = generateVariants(options, existing, baseDefaults);
    const small = result.variants.find((v) => v.key === "Size=S");
    expect(small?.id).toBe("var-1");
    expect(small?.price).toBe("29.99");
    expect(small?.sku).toBe("SHIRT-S");
    expect(small?.stock).toBe(42);
  });

  it("drops unmatched existing variants into removedIds", () => {
    const options: OptionDraft[] = [
      { name: "Size", values: ["M"] },
    ];
    const existing: VariantDraft[] = [
      { key: "Size=S", id: "var-1", ...baseDefaults },
      { key: "Size=M", id: "var-2", ...baseDefaults },
    ];
    const result = generateVariants(options, existing, baseDefaults);
    expect(result.variants.map((v) => v.key)).toEqual(["Size=M"]);
    expect(result.removedIds).toEqual(["var-1"]);
  });

  it("new variants get defaults from baseDefaults", () => {
    const options: OptionDraft[] = [{ name: "Size", values: ["L"] }];
    const result = generateVariants(options, [], baseDefaults);
    expect(result.variants[0]).toMatchObject({
      key: "Size=L",
      price: "19.99",
      stock: 0,
      sku: "",
    });
    expect(result.variants[0].id).toBeUndefined();
  });

  it("renaming an option value does NOT preserve data (rename is a new value to the key builder)", () => {
    // Documented behavior: rename = new value. UI layer handles rename-in-place via a separate path.
    const options: OptionDraft[] = [{ name: "Size", values: ["Medium"] }];
    const existing: VariantDraft[] = [
      { key: "Size=M", id: "var-1", price: "29.99", sku: "X", stock: 5, weight: 0 },
    ];
    const result = generateVariants(options, existing, baseDefaults);
    expect(result.variants[0].key).toBe("Size=Medium");
    expect(result.variants[0].id).toBeUndefined();
    expect(result.removedIds).toEqual(["var-1"]);
  });

  it("reordering options does NOT orphan variants (sorted key is stable)", () => {
    const existing: VariantDraft[] = [
      { key: "Color=Red|Size=M", id: "var-1", price: "19.99", sku: "R-M", stock: 3, weight: 0 },
    ];
    // User reorders: Size first, Color second — same product.
    const options: OptionDraft[] = [
      { name: "Size", values: ["M"] },
      { name: "Color", values: ["Red"] },
    ];
    const result = generateVariants(options, existing, baseDefaults);
    expect(result.variants[0].id).toBe("var-1");
    expect(result.variants[0].sku).toBe("R-M");
    expect(result.removedIds).toEqual([]);
  });

  it("enforces 500-variant cap by throwing", () => {
    // 11 × 11 × 5 = 605 > 500
    const options: OptionDraft[] = [
      { name: "A", values: Array.from({ length: 11 }, (_, i) => `a${i}`) },
      { name: "B", values: Array.from({ length: 11 }, (_, i) => `b${i}`) },
      { name: "C", values: Array.from({ length: 5 }, (_, i) => `c${i}`) },
    ];
    expect(() => generateVariants(options, [], baseDefaults)).toThrow(
      /too many variants/i
    );
  });

  it("503 combinations is allowed only if exactly at the cap", () => {
    // 500 = 10 × 50
    const options: OptionDraft[] = [
      { name: "A", values: Array.from({ length: 10 }, (_, i) => `a${i}`) },
      { name: "B", values: Array.from({ length: 50 }, (_, i) => `b${i}`) },
    ];
    const result = generateVariants(options, [], baseDefaults);
    expect(result.variants).toHaveLength(500);
  });
});
```

- [ ] **Step 7: Run test (expect FAIL)**

```bash
npm run test -- generateVariants
```

Expected: FAIL.

- [ ] **Step 8: Implement `generateVariants.ts`**

```typescript
// apps/admin/lib/products/generateVariants.ts
import { buildVariantKey } from "./variantKey";

export const MAX_VARIANTS = 500;

export interface OptionDraft {
  name: string;
  values: string[];
}

export interface VariantDefaults {
  price: string;
  sku: string;
  stock: number;
  weight: number;
}

export interface VariantDraft extends VariantDefaults {
  key: string;
  id?: string;
  variantImageId?: string | null;
}

export interface GenerateVariantsResult {
  variants: VariantDraft[];
  removedIds: string[];
}

export function generateVariants(
  options: OptionDraft[],
  existing: VariantDraft[],
  defaults: VariantDefaults
): GenerateVariantsResult {
  if (options.length === 0) {
    return {
      variants: [],
      removedIds: existing.filter((v) => v.id).map((v) => v.id!),
    };
  }

  // Cartesian product of option values, in declared-option order.
  const combinations: Array<{ name: string; value: string }[]> = [[]];
  for (const option of options) {
    const next: Array<{ name: string; value: string }[]> = [];
    for (const combo of combinations) {
      for (const value of option.values) {
        next.push([...combo, { name: option.name, value }]);
      }
    }
    combinations.splice(0, combinations.length, ...next);
  }

  if (combinations.length > MAX_VARIANTS) {
    throw new Error(
      `Too many variants: ${combinations.length}. Maximum is ${MAX_VARIANTS}.`
    );
  }

  const existingByKey = new Map(existing.map((v) => [v.key, v]));
  const nextVariants: VariantDraft[] = combinations.map((pairs) => {
    const key = buildVariantKey(pairs);
    const prior = existingByKey.get(key);
    if (prior) {
      existingByKey.delete(key); // mark as consumed
      return { ...prior, key };
    }
    return { key, ...defaults };
  });

  const removedIds = Array.from(existingByKey.values())
    .filter((v) => v.id)
    .map((v) => v.id!);

  return { variants: nextVariants, removedIds };
}
```

- [ ] **Step 9: Run test (expect PASS)**

```bash
npm run test -- generateVariants
```

All 9 cases green.

- [ ] **Step 10: Commit**

```bash
git add apps/admin/lib/products/generateVariants.ts apps/admin/lib/products/generateVariants.test.ts
git commit -m "feat(admin): generateVariants pure helper with cap + orphan tracking (M7c)"
```

---

### Task 3: `mediaUploadClient.ts` + signed-URL flow with 403 retry (U)

**Files:**
- Create: `apps/admin/components/products/media/mediaUploadClient.ts`
- Create: `apps/admin/components/products/media/mediaUploadClient.test.ts`
- Modify: `apps/admin/lib/api/marketplace-api.ts` — add `requestMediaSignedUrl`, `finalizeMedia`, `deleteMedia`, `recropMedia` typed clients

**Scope:** The client-side upload pipeline. Abstract over `fetch` so tests can mock it. Handle the 60-minute signed URL, the 403 retry-with-fresh-URL path, progress reporting via `XMLHttpRequest` (fetch can't emit upload progress events in browsers).

- [ ] **Step 1: Write failing test for the signed URL + PUT flow**

```typescript
// apps/admin/components/products/media/mediaUploadClient.test.ts
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { uploadMediaFile } from "./mediaUploadClient";

describe("uploadMediaFile", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;
  let putSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    putSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("happy path: request signed url → PUT → finalize", async () => {
    fetchSpy
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          upload_url: "https://gcs.example/upload/abc",
          gcs_path: "stores/s1/products/p1/abc.jpg",
          expires_at: new Date(Date.now() + 3600 * 1000).toISOString(),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          id: "media-1",
          url: "https://cdn.example/abc.jpg",
          alt: "",
          position: 0,
          variant_id: null,
        }),
      });
    putSpy.mockResolvedValueOnce({ ok: true });

    const file = new File(["hello"], "hello.jpg", { type: "image/jpeg" });
    const result = await uploadMediaFile({
      storeId: "s1",
      productId: "p1",
      file,
      onProgress: vi.fn(),
      putFn: putSpy,
    });

    expect(result.id).toBe("media-1");
    expect(fetchSpy).toHaveBeenCalledTimes(2);
    expect(putSpy).toHaveBeenCalledTimes(1);
  });

  it("retries PUT once on 403 with a fresh signed URL", async () => {
    fetchSpy
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          upload_url: "https://gcs.example/upload/stale",
          gcs_path: "stores/s1/products/p1/stale.jpg",
          expires_at: new Date(Date.now() - 1000).toISOString(),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          upload_url: "https://gcs.example/upload/fresh",
          gcs_path: "stores/s1/products/p1/fresh.jpg",
          expires_at: new Date(Date.now() + 3600 * 1000).toISOString(),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          id: "media-2",
          url: "https://cdn.example/fresh.jpg",
          alt: "",
          position: 0,
          variant_id: null,
        }),
      });
    putSpy
      .mockResolvedValueOnce({ ok: false, status: 403 })
      .mockResolvedValueOnce({ ok: true });

    const file = new File(["x"], "x.jpg", { type: "image/jpeg" });
    const result = await uploadMediaFile({
      storeId: "s1",
      productId: "p1",
      file,
      onProgress: vi.fn(),
      putFn: putSpy,
    });

    expect(result.id).toBe("media-2");
    expect(putSpy).toHaveBeenCalledTimes(2);
    expect(fetchSpy).toHaveBeenCalledTimes(3); // signed_url, signed_url, finalize
  });

  it("surfaces error after second PUT also fails", async () => {
    fetchSpy
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          upload_url: "https://gcs.example/upload/a",
          gcs_path: "a",
          expires_at: new Date(Date.now() + 1000).toISOString(),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          upload_url: "https://gcs.example/upload/b",
          gcs_path: "b",
          expires_at: new Date(Date.now() + 1000).toISOString(),
        }),
      });
    putSpy
      .mockResolvedValueOnce({ ok: false, status: 403 })
      .mockResolvedValueOnce({ ok: false, status: 403 });

    const file = new File(["x"], "x.jpg", { type: "image/jpeg" });
    await expect(
      uploadMediaFile({
        storeId: "s1",
        productId: "p1",
        file,
        onProgress: vi.fn(),
        putFn: putSpy,
      })
    ).rejects.toThrow(/upload failed/i);
  });
});
```

- [ ] **Step 2: Run test (expect FAIL)**

```bash
npm run test -- mediaUploadClient
```

- [ ] **Step 3: Implement the typed marketplace-api clients**

Add to `apps/admin/lib/api/marketplace-api.ts`:

```typescript
// --- Media ---
export interface SignedUrlResponse {
  upload_url: string;
  gcs_path: string;
  expires_at: string;
}

export async function requestMediaSignedUrl(args: {
  storeId: string;
  productId: string;
  filename: string;
  contentType: string;
}): Promise<SignedUrlResponse> {
  const res = await fetchAdminApi(
    `/stores/${args.storeId}/products/${args.productId}/media/signed-url`,
    { method: "POST", body: JSON.stringify({ filename: args.filename, content_type: args.contentType }) }
  );
  if (!res.ok) throw new Error(`signed url request failed: ${res.status}`);
  return res.json();
}

export interface AdminMedia {
  id: string;
  url: string;
  alt: string;
  position: number;
  variant_id: string | null;
  gcs_path: string;
  gcs_path_original: string;
}

export async function finalizeMedia(args: {
  storeId: string;
  productId: string;
  gcsPath: string;
  alt?: string;
  position: number;
  variantId?: string | null;
}): Promise<AdminMedia> {
  const res = await fetchAdminApi(
    `/stores/${args.storeId}/products/${args.productId}/media`,
    {
      method: "POST",
      body: JSON.stringify({
        gcs_path: args.gcsPath,
        alt: args.alt ?? "",
        position: args.position,
        variant_id: args.variantId ?? null,
      }),
    }
  );
  if (!res.ok) throw new Error(`finalize media failed: ${res.status}`);
  return res.json();
}

export async function deleteMedia(args: {
  storeId: string;
  productId: string;
  mediaId: string;
}): Promise<void> {
  const res = await fetchAdminApi(
    `/stores/${args.storeId}/products/${args.productId}/media/${args.mediaId}`,
    { method: "DELETE" }
  );
  if (!res.ok) throw new Error(`delete media failed: ${res.status}`);
}

export async function recropMedia(args: {
  storeId: string;
  productId: string;
  mediaId: string;
  cropBox: { x: number; y: number; width: number; height: number };
  rotation: number;
}): Promise<AdminMedia> {
  const res = await fetchAdminApi(
    `/stores/${args.storeId}/products/${args.productId}/media/${args.mediaId}/recrop`,
    { method: "POST", body: JSON.stringify({ crop_box: args.cropBox, rotation: args.rotation }) }
  );
  if (!res.ok) throw new Error(`recrop failed: ${res.status}`);
  return res.json();
}
```

(Adjust imports/`fetchAdminApi` helper name to match what M7a/M7b established.)

- [ ] **Step 4: Implement `mediaUploadClient.ts`**

```typescript
// apps/admin/components/products/media/mediaUploadClient.ts
import {
  requestMediaSignedUrl,
  finalizeMedia,
  type AdminMedia,
} from "@/lib/api/marketplace-api";

export interface UploadArgs {
  storeId: string;
  productId: string;
  file: Blob;
  filename?: string;
  position?: number;
  variantId?: string | null;
  alt?: string;
  onProgress?: (pct: number) => void;
  // Injectable for tests — defaults to window.fetch-backed PUT
  putFn?: (url: string, body: Blob) => Promise<{ ok: boolean; status?: number }>;
}

const defaultPut = async (url: string, body: Blob) => {
  const res = await fetch(url, {
    method: "PUT",
    headers: { "Content-Type": body.type || "application/octet-stream" },
    body,
  });
  return { ok: res.ok, status: res.status };
};

export async function uploadMediaFile(args: UploadArgs): Promise<AdminMedia> {
  const put = args.putFn ?? defaultPut;
  const filename = args.filename ?? (args.file as File).name ?? "upload.bin";
  const contentType = args.file.type || "application/octet-stream";

  // 1) Request signed URL
  let signed = await requestMediaSignedUrl({
    storeId: args.storeId,
    productId: args.productId,
    filename,
    contentType,
  });

  // 2) PUT with one retry on 403 (stale URL)
  args.onProgress?.(0);
  let putResult = await put(signed.upload_url, args.file);
  if (!putResult.ok && putResult.status === 403) {
    signed = await requestMediaSignedUrl({
      storeId: args.storeId,
      productId: args.productId,
      filename,
      contentType,
    });
    putResult = await put(signed.upload_url, args.file);
  }
  if (!putResult.ok) {
    throw new Error(`upload failed: PUT returned ${putResult.status ?? "unknown"}`);
  }
  args.onProgress?.(100);

  // 3) Finalize
  return finalizeMedia({
    storeId: args.storeId,
    productId: args.productId,
    gcsPath: signed.gcs_path,
    alt: args.alt,
    position: args.position ?? 0,
    variantId: args.variantId,
  });
}
```

- [ ] **Step 5: Run test (expect PASS)**

```bash
npm run test -- mediaUploadClient
```

- [ ] **Step 6: Commit**

```bash
git add apps/admin/components/products/media/mediaUploadClient.ts apps/admin/components/products/media/mediaUploadClient.test.ts apps/admin/lib/api/marketplace-api.ts
git commit -m "feat(admin): media upload client with signed URL + 403 retry (M7c)"
```

---

### Task 4: `MediaCropDialog` with `react-easy-crop` and re-crop round-trip (C)

**Files:**
- Create: `apps/admin/components/products/media/MediaCropDialog.tsx`
- Create: `apps/admin/components/products/media/cropImage.ts` (pure canvas helper)
- Create: `apps/admin/components/products/media/cropImage.test.ts`
- Modify: `apps/admin/package.json` — add `react-easy-crop`

**Scope:** The crop dialog is a full-panel inline surface (not a true modal — this is an editing workbench). Takes a source blob or URL, shows `react-easy-crop`, returns a cropped blob on apply. Re-crop reads `gcs_path_original` via a fresh signed GET URL.

- [ ] **Step 1: Install `react-easy-crop`**

```bash
cd apps/admin
npm install react-easy-crop@^5
```

Verify it landed in package.json and commit the lockfile change separately at the end.

- [ ] **Step 2: Write failing test for the pure canvas helper**

```typescript
// apps/admin/components/products/media/cropImage.test.ts
import { describe, it, expect, vi } from "vitest";
import { cropToBlob } from "./cropImage";

describe("cropToBlob", () => {
  it("calls canvas drawImage with rounded pixel coordinates", async () => {
    const drawImage = vi.fn();
    const toBlob = vi.fn((cb: BlobCallback) =>
      cb(new Blob(["x"], { type: "image/jpeg" }))
    );

    // Mock canvas
    const fakeCtx = { drawImage } as unknown as CanvasRenderingContext2D;
    const fakeCanvas = {
      width: 0,
      height: 0,
      getContext: () => fakeCtx,
      toBlob,
    } as unknown as HTMLCanvasElement;

    vi.spyOn(document, "createElement").mockImplementation(
      ((tag: string) => {
        if (tag === "canvas") return fakeCanvas;
        return document.createElement(tag);
      }) as typeof document.createElement
    );

    const fakeImage = {
      naturalWidth: 800,
      naturalHeight: 600,
    } as HTMLImageElement;

    const blob = await cropToBlob(fakeImage, {
      x: 100.4,
      y: 200.6,
      width: 300.5,
      height: 400.5,
    }, 0);

    expect(drawImage).toHaveBeenCalled();
    const args = drawImage.mock.calls[0];
    // Rounded, not floored
    expect(args[1]).toBe(100);
    expect(args[2]).toBe(201);
    expect(args[3]).toBe(301);
    expect(args[4]).toBe(401);
    expect(blob).toBeInstanceOf(Blob);
  });
});
```

- [ ] **Step 3: Run test (expect FAIL)**

```bash
npm run test -- cropImage
```

- [ ] **Step 4: Implement `cropImage.ts`**

```typescript
// apps/admin/components/products/media/cropImage.ts
export interface CropBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export async function cropToBlob(
  image: HTMLImageElement,
  box: CropBox,
  rotationDeg: number
): Promise<Blob> {
  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("2d context unavailable");

  const sx = Math.round(box.x);
  const sy = Math.round(box.y);
  const sw = Math.round(box.width);
  const sh = Math.round(box.height);

  canvas.width = sw;
  canvas.height = sh;

  if (rotationDeg !== 0) {
    ctx.translate(sw / 2, sh / 2);
    ctx.rotate((rotationDeg * Math.PI) / 180);
    ctx.translate(-sw / 2, -sh / 2);
  }

  ctx.drawImage(image, sx, sy, sw, sh, 0, 0, sw, sh);

  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (b) => (b ? resolve(b) : reject(new Error("canvas.toBlob returned null"))),
      "image/jpeg",
      0.92
    );
  });
}

export function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => resolve(img);
    img.onerror = reject;
    img.src = src;
  });
}
```

- [ ] **Step 5: Run test (expect PASS)**

```bash
npm run test -- cropImage
```

- [ ] **Step 6: Implement `MediaCropDialog.tsx`**

Use `frontend-design` skill to generate the editorial-grade UI for this dialog. It must:
- Render `react-easy-crop` in the main panel
- Show zoom slider and rotation buttons (90° increments)
- "Cancel" and "Apply crop" actions in a hairline footer
- Use Paper surface, Ink text, Moss accent for the primary action
- Keyboard-accessible: Escape cancels, Enter applies
- `prefers-reduced-motion` honored

Skeleton (fill in with frontend-design output):

```typescript
// apps/admin/components/products/media/MediaCropDialog.tsx
"use client";
import { useCallback, useState } from "react";
import Cropper, { type Area } from "react-easy-crop";
import { cropToBlob, loadImage, type CropBox } from "./cropImage";

export interface MediaCropDialogProps {
  sourceUrl: string;              // gcs_path_original signed GET URL when re-cropping, or object URL for fresh upload
  aspect?: number;                // defaults to free
  onApply: (blob: Blob, box: CropBox, rotation: number) => void;
  onCancel: () => void;
}

export function MediaCropDialog({ sourceUrl, aspect, onApply, onCancel }: MediaCropDialogProps) {
  const [crop, setCrop] = useState({ x: 0, y: 0 });
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);
  const [pixelCrop, setPixelCrop] = useState<Area | null>(null);

  const onCropComplete = useCallback((_area: Area, pixels: Area) => {
    setPixelCrop(pixels);
  }, []);

  const handleApply = async () => {
    if (!pixelCrop) return;
    const img = await loadImage(sourceUrl);
    const box: CropBox = {
      x: pixelCrop.x,
      y: pixelCrop.y,
      width: pixelCrop.width,
      height: pixelCrop.height,
    };
    const blob = await cropToBlob(img, box, rotation);
    onApply(blob, box, rotation);
  };

  return (
    <section
      role="dialog"
      aria-label="Crop image"
      className="flex flex-col gap-4 p-6 bg-[var(--paper-200)] text-[var(--ink-900)]"
    >
      <header>
        <h2 className="font-[var(--font-serif)] text-2xl">Crop image</h2>
      </header>

      <div className="relative h-96 bg-black">
        <Cropper
          image={sourceUrl}
          crop={crop}
          zoom={zoom}
          rotation={rotation}
          aspect={aspect}
          onCropChange={setCrop}
          onZoomChange={setZoom}
          onCropComplete={onCropComplete}
        />
      </div>

      {/* Zoom slider + rotation buttons — fill in per frontend-design */}

      <footer className="flex justify-end gap-2 pt-2 border-t border-[var(--ink-100)]">
        <button type="button" onClick={onCancel} className="text-[var(--ink-700)]">
          Cancel
        </button>
        <button
          type="button"
          onClick={handleApply}
          className="bg-[var(--ink-900)] text-[var(--paper-200)] px-4 py-2 rounded-md"
        >
          Apply crop
        </button>
      </footer>
    </section>
  );
}
```

- [ ] **Step 7: Write a component test for the dialog (apply path)**

```typescript
// apps/admin/components/products/media/MediaCropDialog.test.tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { MediaCropDialog } from "./MediaCropDialog";

vi.mock("react-easy-crop", () => ({
  default: (props: any) => {
    // Simulate the first onCropComplete once mounted
    setTimeout(() => props.onCropComplete?.({ x: 0, y: 0, width: 100, height: 100 }, { x: 10, y: 20, width: 200, height: 300 }), 0);
    return <div data-testid="mock-cropper" />;
  },
}));

vi.mock("./cropImage", () => ({
  cropToBlob: vi.fn(async () => new Blob(["x"], { type: "image/jpeg" })),
  loadImage: vi.fn(async () => ({ naturalWidth: 800, naturalHeight: 600 })),
}));

describe("MediaCropDialog", () => {
  it("calls onApply with the cropped blob", async () => {
    const onApply = vi.fn();
    render(<MediaCropDialog sourceUrl="blob:fake" onApply={onApply} onCancel={vi.fn()} />);

    // wait a tick for the mock cropper to emit
    await new Promise((r) => setTimeout(r, 10));

    fireEvent.click(screen.getByText(/apply crop/i));
    await new Promise((r) => setTimeout(r, 10));

    expect(onApply).toHaveBeenCalled();
    const [blob, box, rotation] = onApply.mock.calls[0];
    expect(blob).toBeInstanceOf(Blob);
    expect(box).toEqual({ x: 10, y: 20, width: 200, height: 300 });
    expect(rotation).toBe(0);
  });
});
```

- [ ] **Step 8: Run the component test**

```bash
npm run test -- MediaCropDialog
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/admin/components/products/media/cropImage.ts apps/admin/components/products/media/cropImage.test.ts apps/admin/components/products/media/MediaCropDialog.tsx apps/admin/components/products/media/MediaCropDialog.test.tsx apps/admin/package.json apps/admin/package-lock.json
git commit -m "feat(admin): MediaCropDialog with react-easy-crop + rounded pixel helper (M7c)"
```

---

### Task 5: `MediaCard` + `MediaGrid` with `@dnd-kit/sortable` (C)

**Files:**
- Create: `apps/admin/components/products/media/MediaCard.tsx`
- Create: `apps/admin/components/products/media/MediaGrid.tsx`
- Create: `apps/admin/components/products/media/MediaGrid.test.tsx`
- Modify: `apps/admin/package.json` — add `@dnd-kit/*` packages (verify not already present)

**Scope:** Sortable grid of media cards. Each card has thumbnail, overflow menu (Set primary / Crop / Replace / Delete), primary badge, and alt text display. Drag-reorder updates positions; dropping commits to form state.

- [ ] **Step 1: Check if `@dnd-kit` is already pinned**

```bash
cd apps/admin
npm ls @dnd-kit/core @dnd-kit/sortable 2>/dev/null || true
```

If already pinned at a recent version, skip install. If absent:

```bash
npm install @dnd-kit/core@^6 @dnd-kit/sortable@^8 @dnd-kit/utilities@^3
```

- [ ] **Step 2: Write failing test for `MediaGrid` reorder**

```typescript
// apps/admin/components/products/media/MediaGrid.test.tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MediaGrid } from "./MediaGrid";

const items = [
  { id: "m1", url: "https://x/a.jpg", alt: "a", position: 0, variant_id: null, gcs_path: "", gcs_path_original: "" },
  { id: "m2", url: "https://x/b.jpg", alt: "b", position: 1, variant_id: null, gcs_path: "", gcs_path_original: "" },
  { id: "m3", url: "https://x/c.jpg", alt: "c", position: 2, variant_id: null, gcs_path: "", gcs_path_original: "" },
];

describe("MediaGrid", () => {
  it("renders a card per item with alt text as accessible name", () => {
    render(<MediaGrid items={items} onReorder={vi.fn()} onAction={vi.fn()} />);
    expect(screen.getByRole("img", { name: "a" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "b" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "c" })).toBeInTheDocument();
  });

  it("marks the first item as primary", () => {
    render(<MediaGrid items={items} onReorder={vi.fn()} onAction={vi.fn()} />);
    const cards = screen.getAllByRole("listitem");
    expect(cards[0]).toHaveAttribute("data-primary", "true");
    expect(cards[1]).not.toHaveAttribute("data-primary", "true");
  });
});
```

(DnD reorder testing is hard in jsdom; keep component tests to rendering + a11y. Reorder is covered in the Playwright E2E.)

- [ ] **Step 3: Run test (expect FAIL)**

```bash
npm run test -- MediaGrid
```

- [ ] **Step 4: Implement `MediaCard` + `MediaGrid`**

Use `frontend-design` skill for the visual layer. Requirements:
- `MediaCard` is ~160px square thumbnail, hairline border (`--ink-100`), crisp 6px radius
- Primary badge is an editorial serif "1" in a Moss circle (not a checkmark icon)
- Overflow menu button top-right, reveals on hover/focus only
- Focus ring uses `--moss-700` per tokens
- `MediaGrid` uses `<ol role="list">` for semantics; items are `<li role="listitem">`
- Empty state is inline hairline-bordered zone with "Drop images here" prompt (handled by `MediaUploader`, see Task 6)

```typescript
// apps/admin/components/products/media/MediaCard.tsx
"use client";
import { MoreHorizontal } from "lucide-react";
import type { AdminMedia } from "@/lib/api/marketplace-api";

export type MediaAction = "set-primary" | "crop" | "replace" | "delete" | "edit-alt";

export interface MediaCardProps {
  media: AdminMedia;
  isPrimary: boolean;
  onAction: (action: MediaAction, media: AdminMedia) => void;
}

export function MediaCard({ media, isPrimary, onAction }: MediaCardProps) {
  return (
    <li
      role="listitem"
      data-primary={isPrimary}
      className="relative h-40 w-40 border border-[var(--ink-100)] rounded-md overflow-hidden bg-[var(--background-elevated)]"
    >
      <img src={media.url} alt={media.alt} className="w-full h-full object-cover" />
      {isPrimary && (
        <span
          aria-label="Primary image"
          className="absolute top-2 left-2 w-6 h-6 rounded-full bg-[var(--moss-700)] text-[var(--paper-200)] font-[var(--font-serif)] text-sm flex items-center justify-center"
        >
          1
        </span>
      )}
      <button
        type="button"
        aria-label="Media actions"
        onClick={() => onAction("set-primary", media)} // placeholder — wire to popover
        className="absolute top-2 right-2 p-1 rounded-md bg-[var(--paper-200)]/80 opacity-0 hover:opacity-100 focus-visible:opacity-100"
      >
        <MoreHorizontal size={16} />
      </button>
    </li>
  );
}
```

```typescript
// apps/admin/components/products/media/MediaGrid.tsx
"use client";
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { SortableContext, arrayMove, horizontalListSortingStrategy, useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { AdminMedia } from "@/lib/api/marketplace-api";
import { MediaCard, type MediaAction } from "./MediaCard";

export interface MediaGridProps {
  items: AdminMedia[];
  onReorder: (nextOrder: AdminMedia[]) => void;
  onAction: (action: MediaAction, media: AdminMedia) => void;
}

function SortableMedia({ media, isPrimary, onAction }: { media: AdminMedia; isPrimary: boolean; onAction: (a: MediaAction, m: AdminMedia) => void }) {
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({ id: media.id });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };
  return (
    <div ref={setNodeRef} style={style} {...attributes} {...listeners}>
      <MediaCard media={media} isPrimary={isPrimary} onAction={onAction} />
    </div>
  );
}

export function MediaGrid({ items, onReorder, onAction }: MediaGridProps) {
  const sensors = useSensors(useSensor(PointerSensor));

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = items.findIndex((i) => i.id === active.id);
    const newIndex = items.findIndex((i) => i.id === over.id);
    onReorder(arrayMove(items, oldIndex, newIndex));
  };

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={items.map((i) => i.id)} strategy={horizontalListSortingStrategy}>
        <ol role="list" className="flex flex-wrap gap-3">
          {items.map((m, idx) => (
            <SortableMedia key={m.id} media={m} isPrimary={idx === 0} onAction={onAction} />
          ))}
        </ol>
      </SortableContext>
    </DndContext>
  );
}
```

- [ ] **Step 5: Run test (expect PASS)**

```bash
npm run test -- MediaGrid
```

- [ ] **Step 6: Commit**

```bash
git add apps/admin/components/products/media/MediaCard.tsx apps/admin/components/products/media/MediaGrid.tsx apps/admin/components/products/media/MediaGrid.test.tsx apps/admin/package.json apps/admin/package-lock.json
git commit -m "feat(admin): MediaGrid + MediaCard with dnd-kit drag-reorder (M7c)"
```

---

### Task 6: `MediaTab` — composition, upload dropzone, alt text, per-variant assignment

**Files:**
- Create: `apps/admin/components/products/form/MediaTab.tsx`
- Create: `apps/admin/components/products/media/MediaUploader.tsx`
- Create: `apps/admin/components/products/media/MediaAltTextInput.tsx`
- Create: `apps/admin/components/products/variants/VariantImagePicker.tsx`

**Scope:** Composes `MediaGrid`, `MediaUploader` (dropzone + progress strip), `MediaCropDialog`, `MediaAltTextInput`, and `VariantImagePicker` into a tab that reads/writes the `media` field array on the RHF form. Also owns the crop dialog open/close state.

Pseudocode outline:

```typescript
// apps/admin/components/products/form/MediaTab.tsx
"use client";
import { useState } from "react";
import { useFieldArray, useFormContext } from "react-hook-form";
import { MediaGrid } from "../media/MediaGrid";
import { MediaUploader } from "../media/MediaUploader";
import { MediaCropDialog } from "../media/MediaCropDialog";
import type { AdminMedia } from "@/lib/api/marketplace-api";
import { uploadMediaFile } from "../media/mediaUploadClient";

export function MediaTab({ storeId, productId }: { storeId: string; productId: string }) {
  const { control } = useFormContext();
  const { fields, append, remove, replace, update } = useFieldArray({ control, name: "media" });
  const [cropTarget, setCropTarget] = useState<AdminMedia | null>(null);

  const handleFiles = async (files: File[]) => {
    for (const file of files) {
      const result = await uploadMediaFile({ storeId, productId, file, position: fields.length });
      append(result);
    }
  };

  const handleAction = (action: string, media: AdminMedia) => {
    switch (action) {
      case "delete":
        remove(fields.findIndex((f) => (f as unknown as AdminMedia).id === media.id));
        break;
      case "set-primary":
        // Move to index 0
        const idx = fields.findIndex((f) => (f as unknown as AdminMedia).id === media.id);
        if (idx > 0) {
          const next = [...(fields as unknown as AdminMedia[])];
          const [primary] = next.splice(idx, 1);
          next.unshift(primary);
          replace(next);
        }
        break;
      case "crop":
        setCropTarget(media);
        break;
    }
  };

  return (
    <div className="space-y-6">
      <MediaUploader onFiles={handleFiles} />
      <MediaGrid
        items={fields as unknown as AdminMedia[]}
        onReorder={(next) => replace(next)}
        onAction={handleAction}
      />
      {cropTarget && (
        <MediaCropDialog
          sourceUrl={cropTarget.gcs_path_original /* resolve to signed GET */}
          onApply={(blob, box, rotation) => {
            // recropMedia API → returns updated AdminMedia → update field
            setCropTarget(null);
          }}
          onCancel={() => setCropTarget(null)}
        />
      )}
    </div>
  );
}
```

**`MediaUploader`** is a drop zone + file picker + progress strip with concurrency cap 3. Implement via `frontend-design` skill for the editorial feel.

**`VariantImagePicker`** is a small popover launched from a variant row: shows the media grid filtered to the current product, user picks one, writes `variant_id` onto that media row.

- [ ] **Step 1: Scaffold each file with a todo implementation**
- [ ] **Step 2: Write component tests for handleFiles / handleAction (mock `uploadMediaFile`)**
- [ ] **Step 3: Run tests (FAIL)**
- [ ] **Step 4: Implement**
- [ ] **Step 5: Run tests (PASS)**
- [ ] **Step 6: Commit**

```bash
git add apps/admin/components/products/form/MediaTab.tsx apps/admin/components/products/media/MediaUploader.tsx apps/admin/components/products/media/MediaAltTextInput.tsx apps/admin/components/products/variants/VariantImagePicker.tsx
git commit -m "feat(admin): MediaTab composition with dropzone + crop dialog wiring (M7c)"
```

---

### Task 7: `OptionsEditor` + `OptionRow`

**Files:**
- Create: `apps/admin/components/products/options/OptionsEditor.tsx`
- Create: `apps/admin/components/products/options/OptionRow.tsx`
- Create: `apps/admin/components/products/options/OptionsEditor.test.tsx`

**Scope:** List of options. Each row has a name input ("Size") and a chip-based value editor ("S", "M", "L"). Add/remove option rows. Rename option in place (rebuilds variants via `generateVariants`). Add/remove values.

- [ ] **Step 1: Write failing test**

```typescript
// apps/admin/components/products/options/OptionsEditor.test.tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { OptionsEditor } from "./OptionsEditor";

describe("OptionsEditor", () => {
  it("adds a new option row", () => {
    const onChange = vi.fn();
    render(<OptionsEditor value={[]} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: /add option/i }));
    expect(onChange).toHaveBeenCalledWith([{ name: "", values: [] }]);
  });

  it("adds a value chip", () => {
    const onChange = vi.fn();
    render(<OptionsEditor value={[{ name: "Size", values: [] }]} onChange={onChange} />);
    fireEvent.change(screen.getByPlaceholderText(/add a value/i), { target: { value: "S" } });
    fireEvent.keyDown(screen.getByPlaceholderText(/add a value/i), { key: "Enter" });
    expect(onChange).toHaveBeenCalledWith([{ name: "Size", values: ["S"] }]);
  });

  it("removes an option row", () => {
    const onChange = vi.fn();
    render(<OptionsEditor value={[{ name: "Size", values: ["S"] }]} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: /remove option/i }));
    expect(onChange).toHaveBeenCalledWith([]);
  });
});
```

- [ ] **Step 2: Run test (FAIL)**

```bash
npm run test -- OptionsEditor
```

- [ ] **Step 3: Implement** (use `frontend-design` for the visual layer, Paper + hairline rows)

- [ ] **Step 4: Run test (PASS)**

- [ ] **Step 5: Commit**

```bash
git add apps/admin/components/products/options/
git commit -m "feat(admin): OptionsEditor + OptionRow with chip value editor (M7c)"
```

---

### Task 8: `VariantMatrixTable` + `VariantRow` (inline edit)

**Files:**
- Create: `apps/admin/components/products/variants/VariantMatrixTable.tsx`
- Create: `apps/admin/components/products/variants/VariantRow.tsx`
- Create: `apps/admin/components/products/variants/VariantMatrixTable.test.tsx`

**Scope:** Renders the variants field array as a table with columns: (option values spanning columns), Price, SKU, Stock, Weight, Image. Inline edit on focus; commits to form state on blur. Uses `@tesserix/web` `Input` primitive for cells.

Implement with `frontend-design` for the table chrome (hairline rules, serif numerals for counts, no bordered cells — whitespace separates).

- [ ] **Step 1: Failing test — renders a row per variant**

```typescript
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { VariantMatrixTable } from "./VariantMatrixTable";

const variants = [
  { key: "Color=Red|Size=M", price: "19.99", sku: "R-M", stock: 5, weight: 0.2 },
  { key: "Color=Red|Size=L", price: "19.99", sku: "R-L", stock: 3, weight: 0.2 },
];

describe("VariantMatrixTable", () => {
  it("renders a row per variant with option values as cells", () => {
    render(<VariantMatrixTable variants={variants} currencyCode="USD" onPatch={() => {}} />);
    expect(screen.getByText("Red")).toBeInTheDocument();
    expect(screen.getByText("M")).toBeInTheDocument();
    expect(screen.getByText("L")).toBeInTheDocument();
    expect(screen.getAllByDisplayValue("19.99")).toHaveLength(2);
  });
});
```

- [ ] **Step 2: Run test (FAIL)**
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run test (PASS)**
- [ ] **Step 5: Commit**

```bash
git add apps/admin/components/products/variants/VariantMatrixTable.tsx apps/admin/components/products/variants/VariantRow.tsx apps/admin/components/products/variants/VariantMatrixTable.test.tsx
git commit -m "feat(admin): VariantMatrixTable with inline editable cells (M7c)"
```

---

### Task 9: `VariantBulkBar` — column bulk actions

**Files:**
- Create: `apps/admin/components/products/variants/VariantBulkBar.tsx`
- Create: `apps/admin/components/products/variants/VariantBulkBar.test.tsx`

**Scope:** Small inline bar above the variant matrix with actions like "Set all prices…", "Set all stock…", "Set all weights…". Opens a small inline input, applies to all variant rows on commit. Idempotent and undo-able within the form (user can still edit individual rows after).

- [ ] **Step 1: Failing test — applying "Set all prices" emits a patch with every row's price updated**
- [ ] **Step 2–4: Implement + verify**
- [ ] **Step 5: Commit**

```bash
git add apps/admin/components/products/variants/VariantBulkBar.tsx apps/admin/components/products/variants/VariantBulkBar.test.tsx
git commit -m "feat(admin): VariantBulkBar column-apply actions (M7c)"
```

---

### Task 10: `ProductForm` tab integration + dirty tracking

**Files:**
- Modify: `apps/admin/components/products/ProductForm.tsx`
- Create: `apps/admin/components/products/form/ProductFormTabs.tsx`
- Create: `apps/admin/components/products/form/GeneralTab.tsx` (extract current form body from ProductForm)
- Create: `apps/admin/components/products/form/OptionsTab.tsx`
- Create: `apps/admin/components/products/form/VariantsTab.tsx`
- Modify: `apps/admin/lib/validation/product-form.ts` — add zod schemas for options/variants/media

**Scope:** Refactor `ProductForm` to host a tab bar. Extract the existing form body into `GeneralTab`. Add `MediaTab`, `OptionsTab`, `VariantsTab`. All tabs share the same RHF `FormProvider`. Dirty tracking spans all tabs via RHF `formState.isDirty`. When options change, derive variants via `generateVariants` and write the result to `form.variants` + `form.removed_variant_ids`.

Wire the cross-tab effect in a `useEffect` inside `ProductForm` (not inside `OptionsTab`) to keep the derivation above any tab-level re-render loops:

```typescript
const options = useWatch({ control, name: "options" }) as OptionDraft[];
const currentVariants = useWatch({ control, name: "variants" }) as VariantDraft[];
const removedIds = useRef<string[]>([]);

useEffect(() => {
  try {
    const { variants, removedIds: newRemoved } = generateVariants(options ?? [], currentVariants ?? [], baseDefaults);
    setValue("variants", variants, { shouldDirty: true });
    removedIds.current = [...removedIds.current, ...newRemoved];
    setValue("removed_variant_ids", removedIds.current, { shouldDirty: true });
  } catch (err) {
    setError("variants", { type: "cap", message: (err as Error).message });
  }
}, [options]);
```

- [ ] **Step 1: Extract `GeneralTab`**

Move current form body out of `ProductForm.tsx` into `GeneralTab.tsx`. Verify the existing M7b Playwright tests still pass.

```bash
cd apps/admin
npm run test:e2e -- products-detail
```

- [ ] **Step 2: Add `ProductFormTabs` shell**

Use `@tesserix/web` Tabs primitive if present, otherwise a simple hairline-underlined nav (frontend-design call).

- [ ] **Step 3: Mount `MediaTab`, `OptionsTab`, `VariantsTab`**

- [ ] **Step 4: Wire the options → variants derivation effect**

- [ ] **Step 5: Extend zod schema in `product-form.ts`**

Add `options`, `variants`, `media`, `removed_variant_ids` field shapes.

- [ ] **Step 6: Run existing tests**

```bash
npm run test
npm run lint
npm run build
```

- [ ] **Step 7: Commit**

```bash
git add apps/admin/components/products/ProductForm.tsx apps/admin/components/products/form/ apps/admin/lib/validation/product-form.ts
git commit -m "feat(admin): ProductForm tab shell + options→variants derivation (M7c)"
```

---

### Task 11: Server action + API client for aggregate PATCH

**Files:**
- Modify: `apps/admin/app/products/actions.ts`
- Modify: `apps/admin/lib/api/marketplace-api.ts`

**Scope:** Extend `updateProductAction` to serialize the full aggregate (options + variants + media + removed_variant_ids) to the backend `PATCH /products/:id`. Existing signature can stay; the payload grows. Error handling maps typed backend errors (like `variant_cap_exceeded`) to form-level messages via `form.setError("root", …)` through the action's return value.

- [ ] **Step 1: Extend `updateProduct` client in `marketplace-api.ts`** to accept the new aggregate fields.
- [ ] **Step 2: Extend `updateProductAction`** to forward them.
- [ ] **Step 3: Add a test** (Vitest, mocking `fetchAdminApi`) that verifies the POST body shape.
- [ ] **Step 4: Run tests**
- [ ] **Step 5: Commit**

```bash
git add apps/admin/app/products/actions.ts apps/admin/lib/api/marketplace-api.ts
git commit -m "feat(admin): aggregate PATCH in updateProductAction (M7c)"
```

---

### Task 12: Component tests — full coverage sweep

**Files:** Add any missing RTL tests to hit 80% coverage on new components.

- [ ] **Step 1: Run coverage**

```bash
cd apps/admin
npm run test -- --coverage
```

- [ ] **Step 2: Identify gaps in new files under `components/products/{media,options,variants}` and `lib/products/`**

- [ ] **Step 3: Add missing tests one file at a time**

Each add should follow TDD (fail → pass → commit) though for coverage-fill tests it's acceptable to batch: add 3–4 small cases, run, commit.

- [ ] **Step 4: Verify 80% lines + branches for every new file**

- [ ] **Step 5: Commit**

```bash
git add apps/admin/components/products/ apps/admin/lib/products/
git commit -m "test(admin): M7c component coverage to 80% threshold"
```

---

### Task 13: Playwright E2E — variants flow and media flow (two tests)

**Files:**
- Create: `apps/admin/tests/e2e/products-variants-flow.spec.ts`
- Create: `apps/admin/tests/e2e/products-media-flow.spec.ts`

**Scope:** Two separate tests per the spec split. Each runs against a live marketplace-api and a seeded store.

**Variants flow:**
1. Navigate to `/products/new`
2. Fill General tab (title, handle, status=draft)
3. Switch to Options tab → add "Color" option with values Red, Blue → add "Size" option with values S, M
4. Switch to Variants tab → assert 4 rows visible
5. Use `VariantBulkBar` "Set all prices" → enter 29.99 → apply
6. Inline-edit Red-S sku to "R-S-001"
7. Save
8. Navigate to `/products/:id` (from redirect)
9. Switch to Variants tab → assert all 4 rows present with price 29.99, sku R-S-001 preserved

**Media flow:**
1. Seed a product with 1 existing image via backend helper
2. Navigate to `/products/:id`
3. Switch to Media tab
4. Upload 2 images via dropzone (test fixtures)
5. Open crop dialog on one image → apply default crop
6. Drag-reorder: move the cropped image to position 0 (primary)
7. Switch to Variants tab → pick the cropped image for a specific variant via `VariantImagePicker`
8. Save
9. Reload page
10. Assert Media tab shows 3 images, cropped one is primary, variant still references it
11. Reopen crop dialog on the cropped image → assert the source is the ORIGINAL blob (not the already-cropped version)

```typescript
// skeleton — apps/admin/tests/e2e/products-variants-flow.spec.ts
import { test, expect } from "@playwright/test";

test.describe("M7c variants flow", () => {
  test("create product with options + variants, set prices via bulk, persist on reload", async ({ page }) => {
    await page.goto("/products/new");
    await page.getByLabel("Title").fill("Linen tee");
    await page.getByLabel("Handle").fill("linen-tee");

    await page.getByRole("tab", { name: "Options" }).click();
    await page.getByRole("button", { name: /add option/i }).click();
    await page.getByPlaceholder(/option name/i).last().fill("Color");
    await page.getByPlaceholder(/add a value/i).last().fill("Red");
    await page.keyboard.press("Enter");
    await page.getByPlaceholder(/add a value/i).last().fill("Blue");
    await page.keyboard.press("Enter");

    await page.getByRole("button", { name: /add option/i }).click();
    await page.getByPlaceholder(/option name/i).last().fill("Size");
    await page.getByPlaceholder(/add a value/i).last().fill("S");
    await page.keyboard.press("Enter");
    await page.getByPlaceholder(/add a value/i).last().fill("M");
    await page.keyboard.press("Enter");

    await page.getByRole("tab", { name: "Variants" }).click();
    await expect(page.getByRole("row")).toHaveCount(5); // header + 4 data rows

    await page.getByRole("button", { name: /set all prices/i }).click();
    await page.getByLabel(/new price/i).fill("29.99");
    await page.getByRole("button", { name: /apply/i }).click();

    await page.getByRole("button", { name: /save/i }).click();
    await page.waitForURL(/\/products\/[a-f0-9-]+/);

    await page.reload();
    await page.getByRole("tab", { name: "Variants" }).click();
    await expect(page.getByDisplayValue("29.99").first()).toBeVisible();
    await expect(page.getByRole("row")).toHaveCount(5);
  });
});
```

- [ ] **Step 1: Write both test skeletons**
- [ ] **Step 2: Seed fixtures (test images under `apps/admin/tests/fixtures/`)**
- [ ] **Step 3: Run the suite locally against a running marketplace-api**

```bash
npm run test:e2e -- products-variants-flow products-media-flow
```

Both should PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/admin/tests/e2e/
git commit -m "test(admin): Playwright E2E for M7c variants + media flows"
```

---

### Task 14: Impeccable chain pass

**Files:** varies — touch-ups across new components only.

**Scope:** Run the impeccable skill chain across every new merchant-facing surface. This is a **gate**, not a polish-if-time-permits step.

- [ ] **Step 1: Run `critique` on each new screen**

```
Invoke: critique skill on MediaTab, OptionsTab, VariantsTab, MediaCropDialog, VariantMatrixTable
```

For each, record the 10-point score. Anything <7.5 goes back through `polish`/`arrange`/`typeset` before proceeding.

- [ ] **Step 2: Run `polish` on any component flagged**

- [ ] **Step 3: Run `arrange`** on the layout of MediaTab and VariantsTab (spacing rhythm, hairline structure)

- [ ] **Step 4: Run `typeset`** on any display text (serif headlines, numeric tabular figures for the variant matrix price/stock columns)

- [ ] **Step 5: Run `audit`** (WCAG 2.1 AA, bundle size, prefers-reduced-motion) — generates a scored report; fix any P0/P1 findings

- [ ] **Step 6: Run `adapt`** to verify responsive breakpoints for tablet and mobile admin usage

- [ ] **Step 7: Re-run `critique`** on every modified surface; assert score ≥ 7.5

- [ ] **Step 8: Commit touch-ups**

```bash
git add apps/admin/components/products/
git commit -m "style(admin): impeccable chain pass on M7c surfaces — critique ≥ 7.5"
```

---

### Task 15: M7c verification + PR

**Files:** none (git + gh only)

**Scope:** Final verification and PR creation.

- [ ] **Step 1: Run the full admin test suite**

```bash
cd apps/admin
npm run test
npm run test:e2e
npm run lint
npm run build
```

All green.

- [ ] **Step 2: Run marketplace-api tests (backend changes from Task 1)**

```bash
cd services/marketplace-api
go test ./... -race
```

- [ ] **Step 3: Verify checklist from spec §8**

- [ ] All new Go files 80%+ coverage
- [ ] All new TS files 80%+ coverage
- [ ] Both Playwright tests green
- [ ] No new `go vet` warnings
- [ ] No new ESLint errors
- [ ] Paper · Ink · Moss tokens only — no new hex values (grep the diff)
- [ ] WCAG 2.1 AA
- [ ] `prefers-reduced-motion` honored
- [ ] No new dialogs except hard-delete confirm (grep for new Dialog imports)
- [ ] `critique` score ≥ 7.5 on every new surface (from Task 14)
- [ ] `mark8ly/.impeccable.md` exists
- [ ] No secrets committed

- [ ] **Step 4: Delete `.planning/m7c-backend-gaps.md`** (if it still exists from Task 1)

```bash
rm -f .planning/m7c-backend-gaps.md
git add -A
git commit -m "chore: remove M7c backend gap log" || true
```

- [ ] **Step 5: Push branch**

```bash
git push -u origin HEAD
```

- [ ] **Step 6: Open PR**

```bash
gh pr create --title "feat(admin): products M7c — variants + rich media editor" --body "$(cat <<'EOF'
## Summary
- Adds Media, Options, Variants tabs to the product detail form (extends M7b ProductForm)
- Full variant matrix editor with per-row price/sku/stock/weight + column bulk actions
- Rich media editor: GCS signed-URL upload, drag-reorder, client-side crop with re-editable originals, alt text, per-variant image assignment
- Backend gaps closed in Task 1: `gcs_path_original` column + recrop endpoint + 500-variant cap mirror + aggregate PATCH support

Implements the M7c plan: `docs/superpowers/plans/2026-04-10-products-m7c-admin-ui-variants-media.md`
Design spec: `docs/superpowers/specs/2026-04-10-products-admin-ui-slice-2-design.md`

## Test plan
- [x] `generateVariants` unit tests (9 cases incl. option reorder + 500-cap)
- [x] `mediaUploadClient` unit tests (happy path + 403 retry + failure)
- [x] `cropImage` canvas coordinate rounding test
- [x] Component tests for OptionsEditor, VariantMatrixTable, VariantBulkBar, MediaGrid
- [x] Playwright E2E: products-variants-flow (create with 2 options → 4 variants → bulk prices → save → reload)
- [x] Playwright E2E: products-media-flow (upload → crop → reorder → per-variant assign → save → reload → re-crop from original)
- [x] Backend integration tests for aggregate PATCH, 500-variant cap, recrop endpoint
- [x] Impeccable chain: critique ≥ 7.5, polish + arrange + typeset + audit + adapt all run
EOF
)"
```

- [ ] **Step 7: Link the PR back into the plan file**

```bash
# After PR opens, update the Status section of this plan from "Pending" → "Complete — PR #NN"
```

Commit that doc update as a final chore commit.

---

## Exit criteria

All of the following must be true before M7c is considered done:

1. PR merged to `main`
2. All 15 tasks checked off
3. Both Playwright E2E tests green in CI
4. Backend `go test ./... -race` green
5. Frontend `npm run test && npm run test:e2e && npm run lint && npm run build` green
6. `critique` score ≥ 7.5 recorded on every new merchant-facing surface
7. No new hex values outside the token system
8. No new dialogs except the existing hard-delete confirm
9. Spec §8 verification checklist fully satisfied
10. `.planning/m7c-backend-gaps.md` removed

---

## Estimated effort

Large. Task 1 (backend verification + fixes) is the biggest unknown — could be half the total effort if multiple gaps exist. Task 2 (`generateVariants`) is small but foundational. Tasks 3–10 are the bulk of frontend work. Tasks 12–15 are verification, not free: plan a meaningful share of the effort for the impeccable chain pass and the E2E tests, not just the happy-path build.
