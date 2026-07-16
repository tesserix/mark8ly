# Spec — mobile-admin lists: sub-projects B / C / D (+ notifications, CI gates)

**Date:** 2026-07-16
**Predecessor:** sub-project A (contract foundation), `4574c922..0294cefa`, shipped + seen working.
**Goal:** 161 real products and 1 real customer exist in the Bondi store and the app shows **zero**.
Make them appear, correctly, and make the class of bug that hid them impossible to reintroduce
silently.

## Why this exists

Every list hook reads `.items` off a response that has no `items` key. The API sends
`{data, meta}`; `.items` is `undefined`; the screens render `data?.items ?? []` → an empty list. No
error, no warning. This is not a regression from A — it is the remaining audited mismatch set.

A built and tested `paginated()` for exactly this and left it with **no production caller**. This
spec is the caller.

## Live wire truth (re-verified 2026-07-16, this session, real token vs prod)

Everything below was confirmed by hitting `api.mark8ly.com` directly, not inherited from the audit.

| endpoint | envelope | reality |
|---|---|---|
| `GET /products` | `{data, meta}` | **161 products** (149 `draft`), app shows 0 |
| `GET /customers` | `{data, meta}` | 1 customer, app shows 0 |
| `GET /orders` | `{data, meta}` | genuinely empty — **stays blank after the fix, correctly** |
| `GET /notifications` | **`{notifications, page, per_page, total}`** | empty; a *third* envelope |

### Corrections to the handoff

The handoff asserted all four lists share `{data, meta}`. Three of its claims are wrong:

1. **Notifications is not `{data, meta}`.** Live: `{"notifications":[],"page":1,"per_page":20,"total":0}`
   (`notifications.go:85`). `paginated()` would throw on it.
2. **`markAllRead` calls a route that does not exist.** App: `POST /notifications/mark-all-read`.
   Route (`mobile_routes.go:159`): `PATCH /notifications/read-all`. Wrong method *and* path → 404
   always. Not in the original 31-mismatch audit. Latent today only because the list is always empty,
   so the "Mark all" button never renders.
3. **Customer names are absent, not null.** `customers_dto.go:24-26` marks `first_name`/`last_name`/
   `phone`/`avatar_url` as `omitempty`; the live record omits all four. Schemas need `.optional()`,
   **not** `.nullable()`. The app's `Customer` type requires `first_name: string` — that would fail
   validation on the only real customer in existence.

### New findings this session (none in any prior audit)

4. **The Inactive product filter 400s.** `GET /products?status=inactive` →
   `400 validation_failed: 'Status' failed on the 'oneof' tag`. Backend accepts `draft|active|archived`.
5. **The Low Stock product filter is silently ignored.** `GET /products?low_stock=true` → `200` with
   `meta.total: 161` — identical to unfiltered. The tab would show the entire catalog while claiming
   to show low stock. Worse than a 400: it lies.
6. **Every price renders in the wrong currency.** `formatCurrency` hardcodes `currency: "USD"` in
   `ProductRow.tsx:12`, `customers/[id].tsx:27` and elsewhere. The Bondi store is **AUD** and each
   variant carries `currency_code: "AUD"`.
7. **Two `_layout.tsx` type errors are real runtime bugs**, not baseline noise.
   `Notifications.removeNotificationSubscription` no longer exists on the module — the unmount cleanup
   **throws today**. `shouldShowAlert` was superseded by `shouldShowBanner`/`shouldShowList`.

### Real product shape (live)

```json
{"id","store_id","handle","title","description","status","tags":[],"categories":[],"options":[],
 "variants":[{"id","sku":"BND-49-…","price":"21","currency_code":"AUD","inventory_quantity":100,
              "inventory_policy":"deny","option_values":[],"position":0}],
 "media":[{"id","url","storage_key",…}]}
```

The app's fiction: `Product {id,name,status,price:number,compare_at_price,sku,stock,thumbnail_url,created_at}`.
Not one of `name`/`price`/`sku`/`stock`/`thumbnail_url`/`created_at`/`compare_at_price` exists on the wire.
`price` is a **quoted string** `"21"` — live proof of locked decision #4.

## Locked decisions (carried from the audit, reaffirmed)

1. App adapts to the backend; backend touched only where data genuinely doesn't exist.
2. Products go variant-aware, like the web admin.
3. Types inferred from zod schemas (`z.infer`) — types can never drift from validation again.
4. Money: `z.union([number, string]).transform(Number)` — both wire forms are real.
5. Contract breaks fail loudly, naming the field path.

## Decisions made in this session

- **Scope:** envelope **and** field renames per domain. Envelope-only would make 161 blank rows appear.
- **Mapping lives in wire-truthful schemas; screens adapt.** Not `.transform()`-to-app-shape (that
  rebuilds the fiction layer and can't express multiple variants), not a hook adapter layer (an
  indirection with no owner). Follows A's dashboard precedent (`3e31abf4`).
- **B stays pure-app; E is not pulled in.** `average_order_value` is derived client-side
  (`order_count ? total_spent / order_count : 0`) — arithmetically identical to any server version,
  zero deploy risk. The **Recent Orders card is removed**; it needs a backend join that doesn't exist.
  Rejected: `GET /orders?search=<email>` (ILIKE partial-match would silently show another customer's
  orders).
- **Customer `addresses` stays unrendered.** It's on the wire and free, but it's net-new UI, not a
  contract fix. Follow-up.
- **Notifications: envelope + fields + route fix, no deep-link.** `resource_type`→route mapping would
  be pure guesswork against an endpoint with zero live data. Tapping becomes a no-op until real
  notifications exist.
- **Products: Inactive→Draft, Low Stock tab removed.** `status=draft` is live-verified (149/161).
  Low Stock needs backend work to mean anything.
- **CI gates land this session.**

## Design

Six units (0–5), in dependency order. Each is one commit, gates green.

### 0 — Demo-client hardening (must be first)

`lib/demo-api-client.ts:180-186` casts six methods through `as ApiClient[...]`, which **silently
drops the `schema` argument**. Demo mode therefore skips all validation. `page<T>()` (line 106)
fabricates `{items, total, next_cursor, has_more}` — a shape no endpoint returns.

This is armed. The moment a hook moves to `paginated()`, demo mode returns `{items}`, `res.data` is
`undefined`, and react-query v5 throws *"Query data cannot be undefined"*. Sub-project A hit this
bug class **twice**; the opus review found this third instance sitting live.

**Change:** give the demo client the real client's signature and actually run `schema.parse()` on the
resolved fixture. Drop the casts.

**Why first:** it converts the whole class from four silent runtime breaks into a loud parse error at
dev time, and it *forces* every demo fixture to become wire-truthful as its domain migrates. The
fixtures (`DEMO_PRODUCTS`, `DEMO_ORDERS`, `DEMO_CUSTOMERS`) all still use fictional field names and
must be rewritten alongside their domain.

### The three envelopes (reference — not a unit of work)

| helper | shape | used by | status |
|---|---|---|---|
| `paginated(item)` | `{data, meta}` | products, orders, customers | exists (A), **no caller yet** |
| `dataOnly(item)` | `{data}` | stores | exists (A), in use |
| `legacyPaged(key, item)` | `{<key>, page, per_page, total}` | notifications only | **new — built in unit 5** |

`legacyPaged` is deliberately **not** a separate early unit. It has exactly one caller, and building
a helper ahead of its caller is what left A's `paginated()` stranded and unexercised — the direct
cause of this session. It gets written in unit 5, next to the code that uses it.

Notifications gets its own helper rather than being bent into `paginated()`. The `legacy` name marks
it as the odd one out, worth normalising server-side one day.

### 1 — B: customers

- Wire-truthful `api/schemas/customers.ts`; `z.infer` types re-exported from `types.ts`.
- `first_name`/`last_name`/`phone`/`avatar_url` → `.optional()` (Go `omitempty`).
- `total_spent` is a JSON number (`float64`); `money` handles it.
- Detail = flat object + `addresses`, **not** `{data}`-wrapped.
- Derive `average_order_value`; delete the Recent Orders card and the now-unused `RecentOrder` import.
- `block` requires a `reason` — currently omitted → 400.

### 2 — C: orders

- `items`, not `line_items` (`orders_dto.go:180`).
- Money fields (`subtotal`/`shipping_total`/`tax_total`/`discount_total`/`grand_total`) are
  `decimal.Decimal` → quoted strings → `money`.
- `item_count` doesn't exist → `items.length`.
- `customer_name` is `omitempty` → `.optional()`.
- Fix `cancel` (needs `reason`), `refund` (needs `refund_request_id`), `MarkFulfilled` (never binds
  the body → tracking number silently discarded).
- **Orders stay visibly empty. That is correct, not a failure.**

### 3 — D: products (largest)

- Wire-truthful variant-aware `api/schemas/products.ts`.
- New `lib/product-display.ts` — pure, unit-tested helpers so `variants[0]` doesn't scatter:
  `primaryVariant`, `productPrice`, `productSku`, `productStock`, `productThumb`, `productCurrency`.
- Screens read `title`/`variants[]`/`media[]` via those helpers.
- `formatCurrency` takes a currency code; drop the hardcoded USD.
- Inactive tab → Draft (`status=draft`). Low Stock tab removed.
- `create` needs `title` + `variants[]` (min 1) — the current body always 400s.
- Media upload's multipart POST hits a JSON endpoint expecting a 3-step signed-URL flow
  (`upload-url` → PUT → `POST media`). **Out of scope** — flagged as a follow-up; it has always been
  broken and fixing it is its own sub-project.
- `POST /variants` route doesn't exist (only `PATCH /products/:id/variants/:variantId`).
  `createVariant`/`listVariants` are dead client methods — remove rather than leave as traps.

### 4 — Notifications

- Write `legacyPaged(key, item)` in `schema-helpers.ts`, then use it:
  `legacyPaged("notifications", …)`; `is_read`→`read`, `message`→`body` (optional).
- `markAllRead`: `POST /notifications/mark-all-read` → `PATCH /notifications/read-all`.
- Type→colour map corrected to the real constants (`models.go:16-30`): `new_order`, `low_stock`,
  `return_requested`, `payment_received`, `review_submitted`, `order_cancelled`, `order_fulfilled`,
  `system_alert`.
- `deep_link` doesn't exist → tap is a no-op.
- **This removes the last `.items`, so `PaginatedResponse` is deleted.** Per A's note: the last
  sub-project to migrate deletes it.

### 5 — CI gates

- Fix the two `_layout.tsx` errors (finding 7) — required, since a gate that fails on arrival is no
  gate.
- Remove `!@repo/mobile-admin` and `!@repo/mobile-shared` from the `ci.yml:108` filter list.
- Add a `turbo run test` job.
- Both packages already declare `check-types` and `test` scripts.

## Error handling

Unchanged from A and load-bearing here: on schema mismatch `client.ts` throws
`ApiError(500, "contract_mismatch", "<field.path>: <msg>")` and `console.error`s it with the request
path. That message is the debugger for this whole spec — watch the metro log.

Known limitation, accepted: union errors collapse the cause — a `money` failure reports
`stats.revenue_today: Invalid input`. The path (the load-bearing half) is correct; the reason is lost.

## Testing

- Schema tests per domain, asserting against **captured real payloads**, not invented ones. The
  products fixture must include the quoted `price: "21"` and an absent `first_name` for customers —
  the two shapes that would have caught this class two months ago.
- `product-display.ts` unit tests: no variants, one variant, many variants, missing media.
- Demo-client test proving `schema.parse()` is actually applied (the regression guard for unit 0).
- Gates from `apps/mobile-admin`, run after **every** unit:
  - `npx jest` → all green, suite count **≥ 132** (132 is today's baseline; it only goes up).
  - `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` → **2** until unit 5, **0** after it
    (unit 5 fixes both pre-existing errors). Any other number is a regression — read the errors, do
    not assume.

## Verification

Gates are necessary, not sufficient — A's whole point is that green gates hid 31 mismatches for two
months. **The 161 products must be seen rendering on the simulator**, with real metro and a real
build, before this is called done. The human does the taps; I screenshot around them.

## Out of scope (recorded, not done)

- Backend `recent_orders` + `average_order_value` on customer detail (sub-project E).
- Real `low_stock` filtering in the Go products handler (E).
- Orders multi-status filter (E).
- Product media 3-step signed-URL upload flow.
- Customer `addresses` rendering.
- Notification deep-linking.
- Marketing / Settings / Stores-management pages — no mobile API endpoints exist at all.
- Lockfile fix — **deferred to the very end of the session** (needs metro stopped; metro must stay up
  for simulator verification).
- `extra.eas.projectId` placeholder blocks `eas build`.

## Landmines (do not rediscover — these cost real time in A)

- **The tsc gate is vacuous without `--pretty false`.** ANSI colour codes mean the literal `error TS`
  never appears; `grep -c` returns 0 while errors exist. **Count — never grep by filename** (a
  per-file grep passed vacuously and missed 6 real errors). jest summary lines are colour-coded too.
- **Single-file `npx jest <file>` hangs forever** without `--forceExit` for suites rendering a
  react-query QueryClient. The full suite is fine.
- **Never** `npm ci` / `npm install` / `rm -rf node_modules` — metro runs against this tree. Never
  touch anything inside any `node_modules/`.
- `export type {X} from "./y"` creates **no local binding** — use `import type` + `export type` when
  the name is referenced locally.
- Tests live in `apps/mobile-admin/__tests__/` only. `jest.mock` fns inside the factory.
- Don't touch metro.config.js / tsconfig.json / jest.config.js / babel.config.js / tailwind /
  app.config.js / eas.json.
