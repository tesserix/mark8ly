# Spec — mobile-admin product form depth (2026-07-16)

Aligning the mobile-admin product editor with the web admin's. Written after tracing every
claim below to the handler or struct that proves it — see **Wire truth**. Nothing here is
inferred from a route name.

## Why

The web admin's product form has five tabs (General, Media, Options, Variants, Tax). Mobile
edits five fields: title, description, active toggle, and the primary variant's price + stock.
**8 of the store's 12 active products are multi-variant**, so today mobile cannot honestly
represent most of the live catalogue — it shows one variant's price as if it were the product's.

## Locked decisions (user, 2026-07-16)

1. **Mobile is a deliberate on-the-go subset, not a full port.** The test for any feature is
   "would a merchant do this from a phone?" Config-heavy surfaces (billing, domains, themes,
   team, arbitrage, tax config) stay web-only. Mobile links out rather than reimplements.
2. **The product form is in scope at full depth**: options + full variants, categories, SKU,
   weight/dimensions, media alt text + reorder, and crop.
3. **Handle and per-product Tax rate override are OUT** — web-only. Handle is an SEO/URL
   decision that breaks live storefront links when changed; tax rate is store config. Neither
   is a stockroom task.
4. **Crop means both** pick-time crop *and* re-crop of already-uploaded media (web parity).
5. Carried over from the contract work: app adapts to the backend; types inferred from zod
   (`z.infer`); money is `z.union([number, string]).transform(Number).pipe(finite)`, never
   `z.coerce.number()`; contract breaks fail loudly naming the field path.

## Decomposition

`#0` — **FAB fix.** SHIPPED as `b37d4e6a`. The Add-product FAB was `position: absolute,
bottom: theme.spacing.lg` (24) while the Dock occupies `insets.bottom + 4 .. + 64` (~102) and
renders above it, so Add-product was invisible and unreachable — it looked like create-product
didn't exist. `new.tsx` and the FAB's `router.push` were both fine all along. Fixed by applying
`useDockClearance()` at the call site, the hook the Dock already exports for exactly this and
which the same screen's FlatList already used (`index.tsx:96`). Verified on the simulator.

`#1` — **Product form depth.** Client-only. No backend change.

`#2` — **Re-crop existing media.** Backend route registration + a mobile crop UI.

`#1` and `#2` are separable and `#1` does not depend on `#2`. Ship `#1` first.

## Wire truth (verified 2026-07-16 — do NOT re-derive)

Mobile has its own route surface: `services/marketplace-api/internal/handlers/admin/mobile_routes.go`
(`RegisterAdminMobile`), GIP bearer auth + a 60 req/min per-user rate limiter, mounted at
`/mobile/admin/stores/:storeId`. It is **not** the same registration as the web's `routes.go`.

**Everything sub-project #1 needs already exists on that surface.** In detail:

| Need | Endpoint / struct | Evidence |
|---|---|---|
| Options + variants | `PATCH /products/:id` | `mobile_routes.go:62` |
| Categories list | `GET /categories` | `mobile_routes.go:95` |
| Media alt + reorder | `PATCH /products/:id/media/:mediaId` | `mobile_routes.go:75` |
| Variant edit | `PATCH /products/:id/variants/:variantId` | `mobile_routes.go:86` |

**`PATCH /products/:id` binds `UpdateProductRequest`** (`products.go:160`) and **branches**
(`products.go:172`):
- if `options` **or** `variants` **or** `removed_variant_ids` are non-nil → `UpdateAggregate`:
  scalars + options + variants + categories in **one transaction, one `product.updated` event**.
- else if any scalar is set → `UpdateBasics`.

Mobile must mirror that branch: send the aggregate only when options/variants actually changed,
basics otherwise. Do not always send everything.

**`UpdateProductRequest`** (`validation.go:296`) accepts: `handle`, `title`, `description`,
`status` (`oneof=draft active archived`), `tags`, `seo_title`, `seo_description`,
`primary_category_id`, `tax_code`, `tax_rate_override`, `tax_category`, `options`, `variants`,
`media`, `category_ids`, `removed_variant_ids`. All pointer-typed to distinguish unset from zero.

**Weight, dimensions, SKU are VARIANT fields, not product fields.**
`CreateProductVariantInput` (`validation.go:256`) carries: `id` (empty on create — preserves
identity on update so the aggregate matches rows without SKU heuristics), `sku` (**required**,
max 100), `barcode`, `price` (required), `compare_at_price`, `cost_price`, `currency_code`,
`weight_grams`, `length_cm`, `width_cm`, `height_cm`, `inventory_quantity`, `inventory_policy`
(`oneof=deny continue`), `low_stock_threshold`, `option_values`, `position`.

The web only *appears* to have these on "General" because it projects the single variant up.
**Mobile must do the same projection for single-variant products and move them per-variant once
options exist** — otherwise the form lies about where the data lives.

**`CreateProductMediaInput`** (`validation.go:283`): `storage_key` (required), `url` (required),
`alt`, `position`, `media_type` (`oneof=image video`).
**`UpdateMediaWireRequest`** (`validation.go:85`): `alt`, `position`, `url`, `variant_id`,
`storage_key` — all optional pointers. → **204 No Content**. Reorder is a `position` PATCH.

Carried from the prior session, still load-bearing:
- **Variants come back UNSORTED** — a real product returns positions `2,3,4,0,1`. `variants[0]`
  is the WRONG variant. Sort by `position`. `lib/product-display.ts` owns this.
- Prices are quoted strings (`"199"`, `"19.99"`); currency **AUD**.
- Customer names are Go `omitempty` → **absent, not null** → `.optional()`, never `.nullable()`.
- `/products` is `{data, meta}`, 161 products, default `page_size=20`, max 100.

### 🔴 The options request/response asymmetry — the trap that must not return

`CreateProductOptionInput` (`validation.go:251`) is:

```go
type CreateProductOptionInput struct {
    Name   string   `json:"name" binding:"required,max=100"`
    Values []string `json:"values" binding:"required,min=1"`
}
```

`UpdateProductRequest.Options` is `*[]CreateProductOptionInput` — so on the **request**,
`values` is `string[]`. On the **response**, the same field name comes back as
`[{id, value, position}]` (already modelled in `packages/mobile-shared/api/schemas/products.ts`,
whose comment records this).

**These are two different shapes sharing one name.** A previous session modelled the response
using the request shape; it would have blanked all 161 products the day any merchant added an
option. **Request and response options MUST be separate schemas**, and a test must pin both.

## Sub-project #1 — product form depth

### Files

`[id].tsx` is already 571 lines and must not absorb this. It becomes composition only.

New, one job each:
- `packages/mobile-shared/api/schemas/products.ts` — replace `categories: z.array(z.unknown())`
  (it punted) with a real schema; add separate request/response option schemas.
- `packages/mobile-shared/api/categories.ts` — wraps `GET /categories`.
- `components/products/OptionsEditor.tsx`
- `components/products/VariantEditor.tsx` — price, stock, SKU, weight, dimensions
- `components/products/CategoryPicker.tsx`
- `components/products/MediaGrid.tsx` — reorder + alt text
- `components/products/ImageViewer.tsx` — extracted out of `[id].tsx` (noted as clean in handoff)

### Pick-time crop

`allowsEditing: true` on `launchImageLibraryAsync` in `[id].tsx`'s `handleAddMedia`. Crops before
upload; no backend change.

⚠️ **Interaction with `51d2e80b`.** That commit removed `requestMediaLibraryPermissionsAsync()`
because asking for library permission opts into the legacy flow, where "Limited Access" strands
the user in iOS's limited-library management sheet and the picker never opens. **Do not
reintroduce a permission request.** `allowsEditing` is orthogonal to permission and safe.
(Note: `51d2e80b` itself is still unverified on device at time of writing.)

## Sub-project #2 — re-crop existing media

### The wire (verified)

`POST /products/:id/media/:mediaId/recrop`, registered on the **web** router only
(`routes.go:271`) — **absent from `mobile_routes.go`**.

Request (`validation.go`, `RecropMediaRequest`):
`{crop_box: {x, y, width, height}, rotation, filename, content_type}`
- `crop_box` is `binding:"required"`; `x,y >= 0`, `width,height > 0`.
- `content_type` is `oneof=image/png image/jpeg image/webp`.
- **`crop_box` + `rotation` are audit metadata only** — the source comment states "the actual
  pixel work happens in the browser". The client does the real crop.

Response (`RecropMediaResponse`): `{source_original_url, upload_url, new_storage_key, expires_at}`.

Flow: `POST /recrop` → GET `source_original_url` (the **pristine original**, never overwritten,
so re-crops are non-destructive and repeatable) → crop locally → PUT to `upload_url` → commit via
`PATCH /media/:mediaId` with `storage_key: new_storage_key`.

Returns **501 Not Implemented** when the wired uploader can't sign (dev FakeUploader).
**Mobile must surface that as a real message, not a silent no-op.**

### Work

1. **Backend**: register `/recrop` on `mobile_routes.go`'s `mediaGroup`, mirroring `routes.go:271`
   — same handler, same `RoleAdmin` authz. ~4 lines.
2. `packages/mobile-shared/api/media.ts` — `recrop()` + response schema.
3. `components/products/CropSheet.tsx` — pan/zoom rect over the image, emits a pixel-space rect.
4. Wire recrop → fetch → manipulate → PUT → PATCH commit.

### Dependencies — all already installed, no `npm install`

Verified present in `apps/mobile-admin/package.json` **and** on disk:
- `expo-image-manipulator` ~56.0.0 — does the pixel work the browser canvas does:
  `manipulateAsync(uri, [{crop: {originX, originY, width, height}}, {rotate}], {format})`
- `react-native-gesture-handler` ~2.31.1, `react-native-reanimated` 4.3.1 — the crop rect UI
- `react-native-svg` 15.15.4

No Skia, no canvas, **no new dependency**. This matters: `npm install` is forbidden in this repo
(see Landmines).

### Risks

- The crop UI is the only genuinely novel component; everything else is a port of a working web
  implementation (`MediaTab.tsx`, `lib/api/marketplace-api.ts`, e2e `products-media-flow.spec.ts`).
- **Pixel-space vs display-space rect conversion is where this class of feature goes subtly
  wrong.** The conversion must be a pure function with its own unit tests, independent of the UI.
  Send the same rect to the manipulator and as `crop_box` so they cannot drift.
- **UNVERIFIED, must check during planning:** what `UpdateMedia` does with `url` vs `storage_key`
  on the commit PATCH. The load-bearing invariant from the add-media flow is that `url` carries
  the **storage key**, not a CDN URL — the backend builds the public URL itself
  (`service_single_media.go:91-97`) and the web admin does the same. **Never hardcode
  `https://cdn.mark8ly.com`.** Confirm against `service_single_media.go` before implementing.

## Testing

- jest in `apps/mobile-admin` (206 today) — components + schema round-trips.
- vitest in `packages/mobile-shared` (83 today).
- **Pinning test for the options request/response asymmetry** — request `values: string[]`,
  response `values: [{id, value, position}]`. Non-negotiable.
- **Pinning test that `url` carries the storage key**, extending the existing
  `__tests__/add-product-media.test.tsx`.
- Unit tests for the crop rect conversion (#2), independent of the UI.
- Unit test that variants are sorted by `position` before any `[0]` access.

### Gates (exact commands)

From `apps/mobile-admin`: `npx jest` → 206/206 (grows with new tests).
From `apps/mobile-admin` **and** `packages/mobile-shared`:
`npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` → `0`.

## Landmines

- 🔴 **`--pretty false` is MANDATORY.** Without it, `grep -c "error TS"` returns **0 while errors
  exist** (ANSI colour). **Count; never grep by filename.** jest summary lines are coloured too —
  **read the TAIL**; `grep "^Tests:"` matches nothing.
- 🔴 **Run BOTH tsc gates.** `packages/mobile-shared` has its own tsconfig extending the ROOT
  (strict, `noUncheckedIndexedAccess`); `apps/mobile-admin` uses the laxer Expo base. The
  mobile-admin gate is **blind** to real mobile-shared errors.
- 🔴 **A file nothing imports is invisible to tsc.** A green gate on a new unimported schema
  proves nothing.
- 🔴 **NEVER `npm ci` / `npm install` / `rm -rf node_modules`** — metro runs against this tree.
  A plain `npm install` is NOT scoped: it mass-bumps the expo cascade and re-creates the nested
  zod 3. If the lock ever needs work, edit it **surgically**.
- Single-file `npx jest <file>` **hangs forever** without `--forceExit`.
- Don't touch `metro.config.js` / `tsconfig.json` / `jest.config.js` / `babel.config.js` /
  tailwind / `app.config.js` / `eas.json`.
- Driving the sim: `xcrun simctl openurl <UDID> "mark8ly-admin://products"` navigates without
  tapping. **Deep-link to the LIST, never `://products/<id>`** — the latter pushes detail with no
  list beneath, so Back exits to the dashboard. A prior session reported that as a navigation bug;
  it was an artifact of its own deep-linking. You cannot tap (`idb` absent, AppleScript `-1719`);
  the human taps, you screenshot.

## Out of scope

- Handle, per-product tax rate override (locked: web-only).
- The other 55 web routes — marketing, support, settings' 24 pages, products/import,
  products/categories management, customers/reviews, orders/returns. Settings is a separate
  discussion the user has explicitly deferred ("from settings web we can discuss what can be
  added").
- Real pagination (100 of 161 products reachable; `page_size=100` is an interim) — known,
  tracked, not this spec.
- Order detail migration — 6 of its 12 fields don't exist on the wire; needs a seeded order.
- Create-wizard image upload — `new.tsx` collects images before a product id exists; needs a
  different flow. The orphaned `ProductMediaPicker.tsx` is shaped for exactly that.
