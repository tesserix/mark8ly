# Handoff — mobile-admin sub-projects B/C/D (2026-07-16)

Work in `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly` (Tesserix "Mark8ly" — Go microservices
+ Next.js + an Expo mobile-admin app). Commit directly to `main`, single-line conventional messages,
no signatures, no PRs. Same for `tesserix-k8s` (one level up).

## Read first
`~/.claude/projects/-Users-Mahesh-Sangawar-personal-tesserix-new-mark8ly/memory/`
- **`mobile_admin_contract_mismatches.md`** — START HERE. Has the **live wire truth** (captured
  2026-07-16), the demo-api-client landmine, and the gotchas below. Don't re-derive any of it.
- `mobile_admin_nativewind_metro_landmines.md` — build/runtime traps. Read before touching Expo.
- `istio_gip_issuer_not_configured.md` + `incident_gateway_stale_eds_504.md` — only if you touch
  Istio or deploy anything.
- `MEMORY.md` — index.

Prior session's execution ledger (gitignored scratch, may be stale but rich):
`.superpowers/sdd/progress.md`. Sub-project A's plan: `docs/superpowers/plans/2026-07-15-mobile-admin-contract-foundation.md`.

## State: sub-project A is SHIPPED and SEEN WORKING

`4574c922..0294cefa` on main (11 commits). **The dashboard renders with live prod data** — confirmed
on a simulator 2026-07-16: signed in as demo@mark8ly.com → straight to the dashboard (no "No store
yet") showing `CUSTOMERS 1`, matching the live API exactly. First time in 2 months anyone got past
the store picker.

A built the machinery B/C/D now cash in:
- `packages/mobile-shared/api/schema-helpers.ts` — `money`, `pageMeta`, **`paginated(item)`**,
  `dataOnly(item)`. `paginated` is built and tested but **has no production caller yet — that is
  YOUR job**.
- `api/schemas/stores.ts`, `api/schemas/dashboard.ts` — wire-truthful, types via `z.infer`.
- `api/client.ts` — on schema mismatch throws `ApiError(500,"contract_mismatch","<field.path>: <msg>")`
  **and console.errors it** with the request path. Watch the metro log — that message is your debugger.
- `types.ts` re-exports the inferred types (dashboard + Store). `PaginatedResponse` (the fictional
  `{items,…}`) still exists — **orders/products/customers/notifications still use it. The last
  sub-project to migrate deletes it.**

Gates (from `apps/mobile-admin`): **jest 132/132 across 17 suites · tsc = exactly 2**.

## YOUR JOB — 161 real products are invisible in the app

Verified live 2026-07-16 with a real token:

| endpoint | API sends | app reads | reality |
|---|---|---|---|
| `/products` | `{data:[20], meta:{total:**161**}}` | `.items` → `undefined` | **161 products, app shows 0** |
| `/customers` | `{data:[1], meta:{total:1}}` | `.items` → `undefined` | 1 exists, app shows 0 |
| `/orders` | `{data:[], meta:{total:0}}` | `.items` → `undefined` | genuinely EMPTY — stays blank post-fix |

The user's words: *"i was not able to see any customer or products."* That is this, exactly — not a
regression from A. **Swapping the hooks to `paginated()` is the unlock.**

The hooks: `apps/mobile-admin/lib/hooks/use-{orders,products,customers,notifications}.ts`, each
`useQuery<PaginatedResponse<T>>` calling `createXApi(client).list()`. The screens read `.items`.

### Decomposition (from the original audit — B/C/D/E)
- **B** Customers + dashboard field renames (dashboard part DONE in A) → really just customers.
- **C** Orders — `line_items` (Go emits `items`), refund needs `refund_request_id`, cancel needs
  `reason`, `MarkFulfilled` never binds the body.
- **D** Products variant-aware — **the big one**, see wire truth below.
- **E** Backend gaps (Go + deploy) — customer `recent_orders` + `average_order_value` **don't exist
  in the backend at all**, so B cannot fully land without E. `POST /variants` route absent. `low_stock`
  filter. Orders multi-status filter.

**⚠️ B is entangled with E.** Decide that scope before planning B.

### The real product shape (live, 2026-07-16) — D is confirmed as the largest
```json
{"id","store_id","handle","title","description","status","tags":[],"categories":[],"options":[],
 "variants":[{"id","sku","price":"21","currency_code":"AUD","inventory_quantity":100,
              "inventory_policy":"deny","option_values":[],"position":0}],
 "media":[{"id","url","storage_key",…}]}
```
App's fiction: `Product {id,name,status,price:number,compare_at_price,sku,stock,thumbnail_url,created_at}`.
`name`→`title` · `price`→`variants[0].price` **as a QUOTED STRING `"21"`** · `sku`→`variants[0].sku` ·
`stock`→`variants[0].inventory_quantity` · `thumbnail_url`→`media[0].url` · no top-level
`compare_at_price`/`created_at`. The quoted `"21"` is live proof of why `money` is a `number|string`
union. Locked decision: **products go variant-aware, like the web admin.**

## 🔴 DO THIS FIRST — the demo-client landmine is armed and will bite you 4×

The opus whole-branch review found it; A already hit this class **twice**.
`apps/mobile-admin/lib/demo-api-client.ts:106`:
```ts
function page<T>(items: T[]): PaginatedResponse<T> {
  return { items, total: items.length, next_cursor: null, has_more: false };
}
```
It fabricates `{items,…}` for `/orders`, `/products`, `/customers`, `/notifications` + the `resolve()`
fallback. **Harmless today only because the hooks share the same fiction.** The moment you swap a hook
to `paginated()`, demo mode returns `{items}`, `res.data` is `undefined`, and react-query v5 throws
*"Query data cannot be undefined"* — reproducing A's Task 5 bug.

**Neither gate catches it**: `resolve()` returns `unknown` behind six `as ApiClient[...]` casts
(`demo-api-client.ts:180-186`) that also **silently drop the `schema` argument**, so demo mode skips
validation entirely.

**The review's recommendation, and mine: make `createDemoApiClient` actually apply the passed schema
before you touch any hook.** That converts this whole bug class into a compile/parse error instead of
four silent runtime breaks. `EXPO_PUBLIC_AUTH_BACKEND=demo` is the demo build (Expo Go/simulator with
no GoogleService-Info.plist).

## Gotchas that cost real time — do not rediscover these

- 🔴 **The tsc gate is VACUOUS without `--pretty false`.** `npx tsc --noEmit 2>&1 | grep -c "error TS"`
  returns **0 while 2 real errors exist** (tsc emits ANSI colour codes, so the literal `error TS`
  never appears). Always:
  `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` → baseline **2** (pre-existing
  `app/(tabs)/_layout.tsx` expo-notifications). **Count them — never grep by filename** (a per-file
  grep passed vacuously and missed 6 real errors). jest's summary lines are colour-coded too, so
  `grep "^Tests:"` silently matches nothing — read the TAIL.
- 🔴 **A single-file `npx jest __tests__/use-store.test.tsx` HANGS FOREVER** without `--forceExit` —
  that test (and `demo-api-client-stores.test.tsx`) renders a react-query QueryClient (open handle)
  and jest runs one suite in-band. The FULL suite (`npx jest`) is fine — workers get torn down. This
  wedged an agent for 10 minutes.
- A bare `export type {X} from "./y"` creates **NO local binding** — `types.ts`'s `CustomerDetail`
  references `RecentOrder` locally, so it needs `import type` + `export type`.
- **NEVER** `npm ci` / `npm install` / `--package-lock-only` / `rm -rf node_modules` — metro runs
  against this tree. **Never touch anything inside any `node_modules/`** (an A implementer deleted a
  nested zod despite being told not to; it was unrecoverable).
- `packages/mobile-shared` resolves everything from the monorepo root. Tests in
  `apps/mobile-admin/__tests__/` ONLY. jest.mock fns INSIDE the factory.
- Don't touch metro.config.js / tsconfig.json / jest.config.js / babel.config.js / tailwind /
  app.config.js / eas.json.
- **CI runs NONE of these gates** — `ci.yml:108` excludes `@repo/mobile-admin` + `@repo/mobile-shared`
  from turbo lint/check-types/build, and there is no `turbo run test` job at all. The compile-error
  leverage is enforced only on a laptop. Worth a conscious decision before leaning on it.

## Recipes

**Mint a token + hit prod** (API key is public-by-design, in the gitignored plist):
```bash
cd apps/mobile-admin
KEY=$(plutil -extract API_KEY raw GoogleService-Info.plist)
T=$(curl -s -X POST "https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=$KEY" \
  -H "Content-Type: application/json" \
  -d '{"email":"demo@mark8ly.com","password":"Admin@123","tenantId":"MP-Internal-e986p","returnSecureToken":true}' \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['idToken'])")
S=8b69eea9-2537-4d36-9d99-bafcbad02dbc
curl -s -H "Authorization: Bearer $T" "https://api.mark8ly.com/api/v1/mobile/admin/stores/$S/products"
```
Creds: `demo@mark8ly.com` / `Admin@123`, tenant `MP-Internal-e986p`, store `8b69eea9-…` ("The Bondi
Store"), marketplace tenant `8c302556-b647-4824-8ce4-73f547ca456e`.

**Driving the simulator:** `xcrun simctl io <UDID> screenshot out.png` and
`simctl launch|terminate com.mark8ly.admin` work. Booted sim: `AD109A46-2F99-43C3-8AAA-FEE68DC8499E`.
**You CANNOT tap programmatically** — `idb` isn't installed and AppleScript is blocked
(`osascript is not allowed assistive access -1719`). **The human does the taps; you screenshot around
them.** `simctl uninstall` does NOT clear the session — Firebase Auth lives in the iOS **Keychain**,
which survives app deletion; use `simctl erase` for a truly clean state.
Metro: real mode on :8081 (`npx expo start --dev-client --port 8081`, **NO demo flag** — a stale demo
metro silently serves the demo bundle). It was running as of this handoff.

**Prod DB** (peer auth, no password):
`kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- psql -U postgres -d mark8ly_marketplace_api`

**gh is authed as the WORK account** and cannot see `tesserix/mark8ly` (404). Run
`gh auth switch --user mahesh-sangawar` first; switch back after. Git push over SSH works regardless.

## Suggested approach

Use `superpowers:brainstorming` → spec → `superpowers:writing-plans` →
`superpowers:subagent-driven-development`, then the **opus whole-branch review** — do not skip it, it
found the Important that all 6 per-task reviews missed on A (and on the feature before that).
**Verify its claims yourself**; one A-session reviewer produced a false positive and another
mis-cited line numbers.

Suggested order: **demo-client schema hardening → B (customers) → C (orders) → D (products)**, with E
scoped alongside B since customer `recent_orders`/`average_order_value` don't exist server-side.
Or just do the **envelope swap across all four lists first** — smallest change that makes the 161
products appear — and treat the field renames as follow-ups. Your call; brainstorm it.

## Open / deferred (not blocking B/C/D)

- **Lockfile is stale** — `packages/mobile-shared/package.json` says zod `^4.4.3` but the lock still
  pins the nested `3.25.76` and declares `^3.23.0`, so **root `npm ci` hard-fails**. CI (`npm install`)
  and Docker (never copies this package.json) are unaffected. **Fix: stop metro, run a plain
  `npm install`, verify the diff only touches the zod entries, commit.** User deferred this.
- **"Your session ended" misfire** — unproven. `client.ts` fires `onUnauthorized("no-session")` when
  `getToken()` is null while `app/_layout.tsx` renders `<Slot/>` without gating on `loading`. Seen once
  on a cold launch (ambiguous — a session had existed); NOT seen after a reinstall. Real test needs
  `simctl erase` or an in-app sign-out. Fix = gate `<Slot/>` on `loading`.
- `money` accepts hex/scientific/octal strings (`"0x1F"`→31, `"1e3"`→1000). Whole-branch review
  confirmed: **do not fix** — `shopspring/decimal` never marshals those forms.
- Union errors collapse the cause: a `money` failure yields `stats.revenue_today: Invalid input`
  (path correct — the load-bearing half — but the reason is gone).
- `apps/mobile-admin/lib/demo-api-client.ts` `DEMO_PRODUCTS`/`DEMO_ORDERS`/`DEMO_CUSTOMERS` still use
  the old fictional field names — D/B/C must fix each as they migrate.
- `extra.eas.projectId` is still the placeholder `'your-eas-project-id'` — blocks `eas build`.
- Apple sign-in device tap-through still parked. `rawNonce: ""` in `signInWithAppleNative()` is the
  prime suspect if Apple linking fails.
- Marketing/Settings/Stores-mgmt pages have **no mobile API endpoints** — needs marketplace-api work.
