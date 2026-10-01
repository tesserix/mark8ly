# Custom products: buyer-supplied images and personalisation

> Status: design, not scheduled. Nothing here should start before the
> go-live items in `docs/GO-LIVE-PUNCHLIST.md` are closed — phase 3
> rewrites the cart's identity key, which is live checkout code.

## What this delivers

A merchant can sell a product whose final form the buyer decides: a
personalised figurine from an uploaded photo, a printed t-shirt from
uploaded artwork, an engraved plate with a name and a date.

Concretely:

- The merchant adds **personalisation fields** to a product — an image
  upload, a short text, a long message, a choice list, a yes/no add-on.
- The buyer fills them on the product page, and the filled values ride
  the cart line through checkout into the order.
- The merchant opens the order and downloads the artwork, with the text
  values on screen and on the packing slip.

## What a merchant can sell with this, and what still blocks them

Covered outright, because all of these are "buyer supplies an image
and/or some text, merchant prints or engraves it": **printed t-shirts
and apparel, mugs, keychains, phone cases, tote bags, engraved plates
and jewellery, photo prints, personalised cards, photo-collage items**
(`max_images > 1`). One variant or forty, it makes no difference.

**Figurines sell too, and nothing generates anything.** The buyer
uploads a photo, crops it, and the merchant opens the order, downloads
the original at full resolution, and sculpts or prints from it with
whatever tools they already use. What the buyer sees before paying is
their cropped photo and the merchant's mockup. There is no 3D model
anywhere in the flow — not for the buyer, not for the merchant.

That is a decision, taken 2026-10-01 after designing the alternative:
image-to-3D generation is deferred in full, including the
merchant-catalog-authoring case. The design is kept at
[`2026-10-01-image-to-3d-generation-design.md`](./2026-10-01-image-to-3d-generation-design.md)
for when it comes back. **This document is self-sufficient without it**
— every category above, figurines included, sells on what is specified
here alone.

Three things this design does **not** give them, each a real constraint
on the categories above rather than a nicety:

1. **Print area is per field, not per variant.** A 3XL shirt has a
   bigger printable area than a S, and a mug handle moves the usable
   rectangle. Decision 1 below hangs fields off the product, which means
   one `print_area` for all sizes. Fine for keychains and mugs, wrong
   for apparel sold across a wide size range. The fix is a
   `print_area` override per variant on the field — a small addition,
   but it should be a deliberate phase, not a surprise during phase 4b.
2. **No made-to-order lead times.** Nothing in the estate says "ships in
   2–3 weeks" on a per-product basis; the current workaround is the
   "Always in stock" checkbox. A personalised item is made-to-order
   almost by definition, so this gap bites this category hardest, and it
   is cheap to close relative to its value.
3. **No print-on-demand fulfilment.** The merchant gets an artwork file
   and a packing slip. Most people selling personalised mugs do not own
   a mug printer — they use a POD service. Nothing here routes an order
   and its artwork to Printful, Printify, Gelato or a local printer's
   intake. For the apparel and drinkware categories specifically, this
   is the largest practical gap in the design, and it is a separate
   integration project of its own shape (per-merchant credentials in
   OpenBao, a provider seam, order-state mapping, their shipping taking
   over ours).

None of the three blocks a merchant from selling. All three decide
whether they enjoy it at volume.

## Product decisions

**1. Fields hang off the product, not the variant.** Variants are the
sellable axis and own price and stock (`internal/product/models.go` —
"Product has no money or stock"). Personalisation is orthogonal: a shirt
in 3 sizes × 2 colours is 6 variants and *one* "upload your artwork"
field. Per-variant fields would multiply the merchant's work by six and
buy nothing. A field that applies to only some variants is a non-goal.

**2. The field kinds are a closed set of five:** `image`, `text`,
`textarea`, `select`, `checkbox`. That covers figurines (image + name)
and apparel (image + size already being a variant). Not in v1: arbitrary
file types (PDF/AI/SVG — a different print pipeline and a different
malware surface), colour pickers, dates, conditional fields.

**3. Money comes from `select` options and `checkbox`, as an absolute
delta.** `price_delta numeric(12,2)` in the store's currency. `image`
and `text` are always free. No percentages: a percentage of a variant
price that itself varies is a support ticket waiting to happen, and an
absolute delta is trivially auditable against what was charged.

**4. The delta folds into `unit_price` at order time.** The per-field
delta is recorded for explainability, but the money the order carries is
one `unit_price` per line. Every downstream consumer — tax
(`internal/tax`), refunds, invoices, the Delhivery declared value —
is then already correct with no change. This is the single most
blast-radius-reducing decision in the document.

**5. Two personalisations of the same variant are two cart lines.**
Today the cart merges on `(productId, variantId)`
(`apps/storefront/lib/cart.ts:36`). A shirt that says "Asha" and a shirt
that says "Ravi" are not the same line. The merge key gains a
fingerprint of the personalisation payload. See the risk in phase 3.

**6. No proof-approval loop in v1.** A merchant who needs to confirm
artwork before production uses the existing ticket thread. A
`awaiting_proof` order state, a buyer-facing approve/reject, and the
reminder emails that go with it are a second project, and a bigger one
than this.

**7. Personalised lines are not self-serve returnable.** Nobody resells a
mug with a stranger's child on it. The eligibility check in
`internal/order/return_service.go` excludes any line with
personalisations; the merchant can still refund manually. This needs a
line in the store's returns policy copy (`apps/storefront/app/policies/returns`).

## Non-goals

- Made-to-order lead times and "ships in 2–3 weeks" messaging. The
  current workaround is the "Always in stock" checkbox already described
  in `apps/onboarding/app/guides/guides.ts:299`.
- Design templates, clipart libraries, font pickers.
- Image-to-3D generation, in every form. Deferred, with the design
  written and parked — see
  [`2026-10-01-image-to-3d-generation-design.md`](./2026-10-01-image-to-3d-generation-design.md).
  Everything in *this* document remains a prerequisite for it, so
  nothing here needs revisiting when it is picked up.
- Profanity or copyright screening of buyer text and images. The
  merchant is the human in the loop. Worth saying out loud in the
  merchant-facing help copy.

## Data model

Four new tables. All of them are tenant- and store-scoped, which means
`internal/tenantpurge/schema_coverage_integration_test.go` and
`internal/customererasure/coverage_integration_test.go` will fail the
moment the migration lands until the deletion plans name them. That
failure is a feature — see "Privacy" below.

### Catalog side

```
product_personalisation_fields
  id            uuid pk
  tenant_id     uuid not null
  store_id      uuid not null
  product_id    uuid not null
  key           varchar(60)  not null     -- stable machine key
  label         varchar(120) not null
  kind          varchar(20)  not null     -- image|text|textarea|select|checkbox
  required      boolean      not null default false
  position      int          not null default 0
  help_text     varchar(300)
  max_length    int                       -- text/textarea only
  max_images    int                       -- image only, default 1
  price_delta   numeric(12,2)             -- checkbox only
  created_at, updated_at
  FOREIGN KEY (product_id, store_id) REFERENCES products (id, store_id) ON DELETE CASCADE
  UNIQUE (product_id, key)
  CHECK (per-kind: max_length only when kind in ('text','textarea'), etc.)

product_personalisation_options           -- the values of a `select`
  id, field_id uuid not null REFERENCES ... ON DELETE CASCADE
  value       varchar(200) not null
  label       varchar(200) not null
  price_delta numeric(12,2) not null default 0
  position    int not null default 0
  UNIQUE (field_id, value)
```

The composite FK copies the variant pattern (`spec §14.4`) for the same
reason: `store_id` can then never drift from the product's.

Field count per product is capped in the service layer, the way options
are capped at 3 — not in the DB. The cap is a plan limit (below).

### Buyer side, before the order

```
personalisation_uploads
  id, tenant_id, store_id, product_id, field_id
  cart_token        varchar(100) not null   -- the existing mk_cart_token identity
  storage_key_original text not null       -- pristine, never rewritten
  storage_key          text not null       -- derived preview (cropped, downscaled)
  crop                 jsonb               -- {x,y,w,h,rotation} in original pixels
  content_hash      varchar(64) not null   -- of the original
  content_type      varchar(100) not null
  size_bytes        bigint not null
  original_filename varchar(300) not null
  width_px, height_px int                   -- of the original; advisory, see "Storage"
  state             varchar(20) not null    -- pending|verified|claimed|expired
  expires_at        timestamptz not null
  created_at
  INDEX (cart_token)
  INDEX (expires_at) WHERE state <> 'claimed'
```

`cart_token` is deliberately the same identity the stock holds already
use (`internal/stockhold/repository.go`, minted by
`apps/storefront/app/api/checkout/cart-holds/route.ts`). An upload is
owned by a cart, not by a logged-in customer, because most buyers are
guests. TTL is **72 hours**, not the holds' 15 minutes: a shopper who
uploads a photo, sleeps on the decision and comes back must not find an
empty slot where their photo was.

### Buyer side, after the order

```
order_item_personalisations
  id
  order_item_id uuid not null REFERENCES order_items(id) ON DELETE CASCADE
  field_key     varchar(60)  not null   -- snapshot
  field_label   varchar(120) not null   -- snapshot
  kind          varchar(20)  not null
  text_value    text                    -- text/textarea/select label/checkbox
  price_delta   numeric(12,2) not null default 0  -- what was actually charged
  storage_key_original text             -- image only; what the merchant prints from
  storage_key   text                    -- image only; the preview the buyer approved
  crop          jsonb                   -- so full-res can be re-derived
  content_hash, content_type, size_bytes, original_filename
  position      int not null default 0
```

Everything is a snapshot, with no FK back to the field. This follows
`OrderItem.TitleSnapshot` / `SKUSnapshot` and the existing note that
`order_items.product_id` is deliberately not a foreign key: an order is
a record of what was sold, and must survive the merchant renaming,
reordering or deleting the field next week.

## Storage, and the part that is actually hard

Product media today land in one bucket and are served as
`https://storage.googleapis.com/<bucket>/<key>`
(`internal/handlers/admin/account.go:320`,
`internal/handlers/admin/branding.go:394`). That bucket is public-read.

**Buyer artwork cannot go there.** A photograph of someone's child,
reachable by anyone holding the URL, is a different category of object
from a product shot. With uniform bucket-level access a "private prefix"
in a public bucket is not private.

Decision: **a second, private bucket**, `MARKETPLACE_PRIVATE_GCS_BUCKET`,
with no `allUsers` binding and a lifecycle rule as a backstop. The
merchant and the buyer both reach objects only through short-lived
signed GETs — `GCSUploader.SignedReadURL` already exists
(`internal/media/gcs.go:155`) and was built for the recrop flow. Bucket
creation and IAM are infra work; there is no terraform in this repo.

Three further facts that each need a decision:

**A V4 signed PUT cannot cap the upload size.** The signature covers the
method, key, content type and expiry — not the length. Two ways out: a
POST policy document with `x-goog-content-length-range`, or accept the
PUT and check afterwards. v1 takes the second: the confirm step has to
run anyway, and it calls the existing `Uploader.Verify` which already
returns `Size`. Over-cap uploads are rejected and the object deleted.
Residual risk: a hostile client can write up to the cap repeatedly
inside the URL's TTL. Mitigated by the per-cart-token rate limit
(`internal/ratelimit`) and the bucket lifecycle rule, not eliminated.

**`internal/media` has no delete.** There is no `Delete` method on
`GCSUploader` and no code path anywhere in marketplace-api that removes
a GCS object — `grep` for `bucket.Object` finds only `gcs.go` and
`internal/csvjob/gcsstore.go`. Expired uploads, over-cap rejects and
erasure all need one. Adding `Delete(ctx, key)` to the `Uploader`
interface (or a sibling `Deleter` capability, following the
`MetadataSetter` precedent) is a prerequisite, not a detail.

**HEIC.** Every photo an iPhone buyer picks is HEIC unless something
converts it. Most print pipelines cannot read it. v1 allow-lists
`image/jpeg`, `image/png`, `image/webp` only, and
`apps/mobile-storefront` converts on pick before upload. The web
storefront rejects HEIC with a message that says what to do about it,
which is a worse experience than converting in the browser and is the
right trade for v1.

Dimensions (`width_px`/`height_px`) are **client-supplied and advisory**.
They drive only the "this image may print blurry" warning shown to the
buyer. The server does not decode the image, and nothing security- or
money-relevant depends on the numbers.

## Crop, preview, and the resolution trap

Yes to both, and most of the machinery exists.

### Crop

The merchant-side recrop flow is the template:
`internal/product/service_media_recrop.go` signs a GET for the pristine
original and a PUT for a freshly keyed cropped blob, the browser does
the crop on a canvas, and `gcs_path_original` is *never touched so
future recrops still read the pristine source*. On the frontend,
`apps/admin/components/products/media/MediaCropDialog.tsx` +
`cropImage.ts` already wrap `react-easy-crop`.

Lift the dialog and `cropImage.ts` into `packages/ui` and the storefront
gets a crop UI for roughly nothing.

**But invert what the derived blob is for.** In admin, the cropped blob
*is* the product image. For buyer artwork it must not be, because a
browser canvas re-encode is lossy and size-capped: a 48 MP iPhone photo
cropped through `canvas.toBlob()` comes out materially smaller than
what it went in as, and nobody notices until a figurine ships with a
blurry face. So:

- `storage_key_original` is the pristine upload. It is what the merchant
  downloads and what any downstream pipeline reads.
- `crop` is the rectangle, in original pixel coordinates.
- `storage_key` is a **preview only** — browser-cropped, downscaled to
  something like 1024 px long edge, cheap to serve.

The merchant's download applies `crop` to the original at full
resolution, so the crop the buyer chose is honoured without the buyer's
browser being the thing that decides print quality. v1 can ship the
download as "original + the rectangle on screen" and defer server-side
cropping, which needs an image library marketplace-api does not yet
have.

### Preview

Two kinds, and the cheap one is the one that sells.

**Resolution warning.** Drive it off the advisory `width_px`/`height_px`
against a per-field `min_px` the merchant sets. "This image may look
soft at the size you've chosen" shown *before* add-to-cart prevents the
refund request after delivery. This is the single cheapest thing in this
document.

**2D mockup composite.** Give the `image` field an optional merchant
mockup image plus a `print_area` rectangle (percentages of the mockup),
and the buyer's crop renders inside it — a CSS/canvas overlay, no
pipeline, no provider, no cost per view:

```
product_personalisation_fields
  mockup_storage_key text          -- image kind only
  print_area         jsonb         -- {x,y,w,h} as % of the mockup
  min_px             int           -- drives the resolution warning
```

For a printed t-shirt this is 90% of the perceived value of a 3D
preview at about 2% of the cost, and it is honest in a way a generated
3D mesh is not: it shows the buyer's own artwork in the merchant's own
photograph, not a machine's guess at a result.

### Where the buyer sees it — all five places, not just the first

Everything above happens on the product page. The buyer must also see
their own artwork at every later point where they could still change
their mind, or the feature feels like it swallowed their photo:

1. **Product page**, during and after the crop. Covered above.
2. **Cart line.** `CartItem.imageUrl` is the *product* image today
   (`apps/storefront/lib/cart.ts:27`); a personalised line shows the
   buyer's own thumbnail instead, with the text values beneath it. This
   is also what makes two lines of the same variant legible as two
   different things — decision 5 is unreadable to a human without it.
3. **Checkout review**, immediately above the pay button. The last
   moment a mistake is free to fix. A buyer who notices the wrong photo
   here costs a cart edit; one who notices on delivery costs a refund
   and a remake.
4. **Order confirmation email.** The merchant surfaces section says to
   echo the *text* values; echo a thumbnail too where the email client
   allows it. "You ordered a mug with this photo on it" closes the loop.
5. **Buyer's order detail** (`apps/storefront/app/orders/[id]`,
   `/account/orders`). Post-purchase, this is the only record the buyer
   has of what they asked for, and the only thing a support
   conversation can refer to.

**The preview URL cannot be cached in the cart.** It is a short-lived
signed GET, and the cart persists in `localStorage` across days. So the
cart line stores `upload_id` and nothing else, and every render fetches
a fresh signed URL. Surfaces 4 and 5 are post-order and read from
`order_item_personalisations`, so they re-sign against
`storage_key_original`'s sibling preview and are unaffected by the
upload TTL.

**And the 72-hour TTL has a cart interaction that must not be
discovered in production.** An upload is swept 72 hours after it is
made; a cart line referencing it survives in `localStorage`
indefinitely. Checkout validation requires the upload be `verified`,
so a stale line fails the order — correctly, but with a generic error
if nothing anticipates it. Required behaviour:

- the cart's render of a line whose `upload_id` no longer resolves
  shows "your photo has expired — upload it again", in place of the
  thumbnail, with the rest of the line intact;
- that line is blocked from checkout client-side, with the message
  attached to the line rather than the pay button;
- the server's rejection names the field, so the storefront can mark
  the right line even when the client missed it.

Sweeping an upload whose cart line is still live is the one case where
the TTL is user-hostile. A cheap mitigation worth taking: when the
preview endpoint is hit for an upload, extend `expires_at`. A shopper
still looking at their cart is still shopping, and a browse is proof of
intent the sweeper should respect.

A real 3D viewer is a separate matter — nothing in this monorepo carries
`three`, `@react-three/*` or `model-viewer` today, and adding a viewer
to the storefront bundle for the benefit of the few products that have a
mesh is a conversion cost for every product that doesn't. It belongs in
the 3D document, lazily imported, on the products that earn it.

## Endpoints

Storefront (public, store-scoped, no FGA per `spec §2 decision 12`,
rate-limited per `cart_token`):

| Route | Does |
|---|---|
| `POST /storefront/stores/:slug/personalisation/upload-url` | validates the field belongs to a live product in this store and is `kind=image`; inserts a `pending` row; returns `{upload_id, url, storage_key, expires_at}` |
| `POST .../personalisation/uploads/:id/confirm` | `Verify()` against GCS, enforce size + content-type, store advisory dims, `pending` → `verified` |
| `PATCH .../personalisation/uploads/:id/crop` | mints a PUT for a new preview key and records the rectangle; the original is untouched |
| `DELETE .../personalisation/uploads/:id` | buyer removes before checkout; scoped by `cart_token`; deletes both objects |
| `GET .../personalisation/uploads/:id/preview` | short-lived signed GET so the buyer sees their own thumbnail; scoped by `cart_token` |

Each is fronted by a storefront BFF route under
`apps/storefront/app/api/` following the `cart-holds` pattern, so the
`cart_token` cookie stays on the storefront origin and is never a
cross-domain concern.

Admin:

- `GET|POST|PATCH|DELETE /admin/stores/:storeId/products/:id/personalisation-fields`
  and `.../fields/:fieldId/options`.
- `GET /admin/stores/:storeId/orders/:id/personalisations/:pid/download`
  → a signed GET for production use. Audited
  (`internal/audit`): downloading a buyer's photograph is an access
  event worth a row.
- Order detail responses carry personalisations per item.

`route-manifest.json` is generated and asserted — regenerate with
`go test ./internal/routemanifest -run TestRouteManifestMatchesMountedRoutes -update`.
`internal/handlers/storefront/route_parity_test.go` needs an entry for
every admin route that deliberately will not exist on mobile (the field
CRUD; mobile-admin views and downloads, it does not author).

## Checkout

`CheckoutItemRequest` (`internal/handlers/storefront/checkout.go:72`)
gains:

```go
Personalisation []CheckoutPersonalisationRequest `json:"personalisation"`
// { field_id, upload_id?, text?, option_id?, checked? }
```

Server-side validation, all of it, because every byte of that struct
comes from the browser:

- every `required` field on the product is present, and no field from
  another product or store is;
- `option_id` belongs to the named `field_id`;
- `text` is within the field's `max_length`;
- `upload_id` is `verified` **and carries the same `cart_token` as the
  request** — without this, a shopper can attach someone else's upload
  by id;
- `max_images` per field is respected.

Repricing is the one security-critical function this touches.
`repriceItems` (`internal/handlers/storefront/checkout_reprice.go:54`)
currently overwrites all client money from the catalog and fails closed
on anything it cannot price — the comment there explains that a shopper
could otherwise POST `unit_price: 0.01`. The deltas must be resolved the
same way: read from `product_personalisation_options.price_delta` and
`product_personalisation_fields.price_delta` by id, scoped to the store,
never taken from the request. `catalogPrice` gains the resolved delta
sum; `UnitPrice = variant price + deltas`, and `LineTotal` follows.

On success, inside the order-create transaction: insert
`order_item_personalisations` from the validated input, and flip the
claimed uploads to `claimed` so the sweeper leaves them alone.

## Merchant surfaces

- `apps/admin` product editor: a "Personalisation" section beside the
  options editor in `app/(admin)/products/[id]/page.tsx`.
- `apps/admin` order detail (`app/(admin)/orders/[id]/page.tsx`): per
  line, the text values inline and a thumbnail + download for artwork.
- `apps/mobile-admin` order detail: text values inline, tap to open
  artwork. No authoring.
- Packing slip / invoice (`internal/orderdoc`): text values yes, the
  **image no**. A 300 DPI print file embedded in a PDF packing slip
  helps nobody; a short reference code that matches the admin download
  does.
- Order confirmation email (`internal/emailtemplates`): echo the text
  values back. "You ordered a mug that says Asha" is the single
  highest-value support-ticket preventer in this feature.

Buyer text is user content rendered into HTML, a PDF and an email. Each
of those three paths needs its own escaping checked —
`internal/product/sanitizer.go` sanitises merchant input and is not the
same problem.

## Privacy, and an existing gap this feature makes serious

`internal/customererasure`, `internal/tenantpurge` and
`internal/subscription/harddelete` are all SQL-only. None of them
touches GCS. So a GDPR erasure today deletes the `review_media` row and
leaves the customer's photograph in a public bucket indefinitely.

That is already a bug. With buyer artwork it becomes "we were asked to
delete a photograph of a child and we kept it", which is a different
conversation. This feature therefore has to carry the fix:

1. Add object deletion to `internal/media` (above).
2. `internal/customererasure/plan.go`: delete `order_item_personalisations`
   rows **and their objects**, ordered before the `orders` anonymisation
   step for exactly the reason `review_media` is (the plan locates rows
   through `orders WHERE customer_email`, which the anonymisation
   destroys). Disposition is **delete**, not anonymise, matching
   `review_media`: the artwork is not aggregate-bearing and no financial
   record needs it.
3. `internal/tenantpurge/purge.go` and
   `internal/subscription/harddelete/sweeper.go`: register both new
   buyer-side tables. `personalisation_uploads` cascades from nothing —
   it needs an explicit step.
4. A sweeper for `personalisation_uploads` where
   `expires_at < now() AND state <> 'claimed'`: delete the row and the
   object.

The coverage guards in both packages fail on an unregistered
tenant-scoped table, so this cannot be forgotten — it can only be
skipped deliberately, which is the right place for the decision to sit.

## Plan gating

Two new `plangate` features (`internal/plangate/matrix.go`):

- `personalisation_fields` — fields allowed per product.
- `personalisation_upload_mb` — per-image size cap.

`matrix_test.go` asserts every plan declares every feature, so adding
these means touching all plan rows.

**Open question for you.** The obvious commercial move is to gate
personalisation to paid plans. But `apps/onboarding/app/ecommerce-for-makers`
pitches makers directly, and a maker selling personalised work is the
exact person that page is written for — gating it off Free undercuts the
funnel. Recommendation: Free gets **1 field per product and 5 MB**,
Starter 5 fields / 15 MB, above that 10 / 25. That is enough for "upload
your photo" on Free and not enough to run a print shop.

## Testing

Beyond the usual repository/service integration tests:

- **Repricing**: a line with deltas prices correctly; a forged
  `price_delta` in the request is ignored; a `select` option from
  another product is rejected; an unknown `field_id` fails the checkout
  closed rather than defaulting to zero.
- **Upload ownership**: cart A cannot claim cart B's `upload_id`.
- **Cross-store**: a field id from another store's product is rejected.
- **Required fields**: omitting one fails checkout, not the add-to-cart.
- **Snapshot durability**: delete the field after the order; the order
  detail still renders.
- **Coverage guards**: erasure, tenant purge, hard-delete sweep.
- **Sweeper**: expired-and-unclaimed is deleted, claimed is not.
- Playwright e2e in `apps/storefront/tests`: upload → cart → checkout →
  admin download.

## Risks

1. **The cart merge-key refactor.** `addItem`/`removeItem`/`setQty` go
   from `(productId, variantId)` to a line key, which touches
   `CartProvider.tsx`, `cart/page.tsx`, `AddToCartButton.tsx`, the
   checkout submit path and `apps/mobile-storefront/lib/cart-store.ts`.
   This is live checkout code. It also invalidates carts persisted in
   shoppers' `localStorage` under the old shape — the provider needs a
   version tag and a migrate-or-drop on read. Do it as its own PR, with
   no feature attached, so the diff is reviewable.
2. **Private bucket is outside this repo.** Phase 1 and 2 cannot ship to
   production before it exists.
3. **Size enforcement is after the fact**, not in the signature.
4. **HEIC rejection on web** is a real buyer-facing papercut.
5. **GCS spend** is now driven by shopper behaviour rather than merchant
   behaviour. Two objects per upload now, not one.
6. **An upload swept out from under a live cart line.** Mitigated by
   extending `expires_at` on preview reads and by an explicit
   expired-line state, not eliminated — a cart idle for a week will
   still lose its photo, and that has to read as an explanation rather
   than a bug.
7. **The canvas crop is a quality trap** if the derived blob is ever
   mistaken for the print source. Name the columns so the mistake is
   hard to make, and assert it in a test that downloads what the admin
   download endpoint returns and checks it against the original's
   dimensions.

## Suggested delivery order

Each phase is independently shippable and leaves the estate working.

| # | Phase | Notes |
|---|---|---|
| 0 | Private bucket + `Delete` on `internal/media` + the erasure/purge blob gap | Infra + the pre-existing bug. Blocks everything. |
| 1 | Catalog: fields/options tables, service, admin CRUD, admin UI | Invisible to buyers. Plan gating lands here. |
| 2 | Upload endpoints + confirm + preview + sweeper + privacy registration | Still invisible; testable with curl. |
| 3 | Cart line-key refactor, alone | The risky one. No feature in the diff. |
| 4 | Storefront personalisation form + add-to-cart | First buyer-visible phase. Resolution warning ships here — it is three lines and prevents refunds. |
| 4b | Crop dialog lifted to `packages/ui`, 2D mockup preview, buyer thumbnails on cart + checkout review + order detail, expired-upload handling | Separable from 4; the form works without it, but the cart is confusing without the thumbnails. |
| 5 | Checkout validation + repricing + order snapshot | The money. |
| 6 | Merchant fulfilment: order detail panel, full-res download, download-all, packing slip, confirmation email | **Not polish.** With 3D deferred, this phase *is* the figurine feature — it is the merchant's entire production workflow. |
| 7 | `apps/mobile-storefront` upload, `apps/mobile-admin` view | HEIC conversion lives here. |

Phase 6 is not optional polish. An order whose artwork the merchant
cannot retrieve is worse than no feature, because it has already taken
the buyer's money. With image-to-3D deferred it carries more weight
still: downloading the buyer's photograph and reading their text is the
*whole* of what a figurine merchant gets from this platform, so it has
to be better than adequate. Two things that follow from that, and that a
"merchant can see the image" reading of the requirement would miss:

- **Download-all per order**, and ideally across the unfulfilled queue.
  A merchant batching a morning's figurines should not click through
  twelve orders and twenty-nine images one at a time.
- **The download must be the pristine original**, with the buyer's crop
  applied at full resolution or supplied alongside as coordinates —
  never the preview blob. See the resolution trap above; this is the
  phase where that mistake would actually be made.
