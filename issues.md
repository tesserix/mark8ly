# Mark8ly — Remote E2E Audit Issue Log

**Audit date:** 2026-04-11
**Targets:**
- Admin: https://india-store-admin.mark8ly.com/
- Storefront: https://india-store.mark8ly.com/

**Admin creds:** provided via `ADMIN_AUDIT_EMAIL` / `ADMIN_AUDIT_PASSWORD` env vars at audit time — not committed.
**Storefront:** anonymous only (sign-in broken — do not attempt)

**Mode:** B — dedicated test tenant, destructive OK. Tests may create/edit/delete records freely.

**Driven by:** Playwright exploratory specs
- `apps/admin/tests/e2e/remote-audit.spec.ts` (run with `ADMIN_AUDIT=1 ADMIN_BASE_URL=... npx playwright test remote-audit.spec.ts`)
- `apps/storefront/tests/e2e/remote-audit.spec.ts` (run with `STOREFRONT_AUDIT=1 STOREFRONT_BASE_URL=... npx playwright test remote-audit.spec.ts`)

**Raw artifacts:**
- `apps/admin/tests/e2e/.audit/admin-findings.json` + `admin-screens/*.png`
- `apps/storefront/tests/e2e/.audit/storefront-findings.json` + `storefront-screens/*.png`

---

## Severity legend

- **P0** — blocker: feature broken, page crashes, data loss risk, security
- **P1** — major: primary flow impaired, obvious bug, broken UI
- **P2** — minor: cosmetic, non-blocking UX, copy, accessibility
- **P3** — nit: polish, consistency, micro-inconsistency

---

## Summary

| Severity | Admin | Storefront | Total |
|---|---|---|---|
| P0 | 9 | 5 | 14 |
| P1 | 9 | 5 | 14 |
| P2 | 10 | 3 | 13 |
| P3 | 3 | 1 | 4 |
| **Total** | **31** | **14** | **45** |

Headline blockers:
- **Admin login bakes `https://0.0.0.0:4202/` into the prod bundle** — CORS-failed fetch to `admin.mark8ly.com` races with form fill and wipes the login form state.
- **Storefront checkout calls `http://localhost:8088/...` in prod** — payment methods cannot load.
- **6 routes return 404 across both apps** (`/marketing`, `/support`, `/settings/general`, `/categories`, `/orders`).
- **`/settings/audit-logs` crashes** with a Server Components render error.
- **Admin Settings sub-nav is missing 7 of 12 children** — Team, Account, Notifications, Subscription, Audit logs, Tax, General are all unreachable via the sidebar.
- **Storefront home is 100% placeholder content** — no real store layout, "RESERVED FOR PRODUCT PHOTOGRAPHY" visible to real customers.
- **Cross-tenant leak on `/pick-tenant`** — the india-store-admin subdomain shows a "demo store" card from another tenant.

---

## Admin issues

### [P0] Login flow leaks local dev URL into production bundle (CORS blocked)
- **App:** admin
- **Route:** `/` → `/login`
- **Repro:** Load `https://india-store-admin.mark8ly.com/` in a clean browser.
- **Expected:** Page prefetches, if any, stay on the same origin or a legitimate auth origin.
- **Actual:** Browser makes a fetch to `https://admin.mark8ly.com/login?returnUrl=https%3A%2F%2F0.0.0.0%3A4202%2F` (redirected from `https://india-store-admin.mark8ly.com/?_rsc=17yrj`) — a production bundle carrying the literal `https://0.0.0.0:4202/` (local dev port) as `returnUrl`. The response has no `Access-Control-Allow-Origin` header, so the preflight is blocked and the request fails with `net::ERR_FAILED`.
- **Console:** `Access to fetch at 'https://admin.mark8ly.com/login?returnUrl=https%3A%2F%2F0.0.0.0%3A4202%2F' … has been blocked by CORS policy`
- **Impact:** This is both a **build-time config bug** (localhost URL baked into prod) AND a **CORS misconfig** on `admin.mark8ly.com`. It directly causes the login-form race below.
- **Evidence:** `admin-findings.json` route `/login (sign-in)`.

### [P0] Login form state wipes mid-typing, blocks sign-in
- **App:** admin
- **Route:** `/login`
- **Repro:** Open `/login`, type email, type password, click Sign in — sometimes the form submits with empty fields and shows "Email is required / Please enter your password" inline validation errors even though the user typed them.
- **Actual:** The form component re-mounts while the user is filling it (caused by the CORS-failed RSC prefetch above bouncing the page), wiping input state. The audit reproduced this reliably on the first run until the spec was hardened to refill + retry.
- **Impact:** Real users may fail to sign in on first try and see misleading "required" errors. Race-dependent — may be intermittent in the wild.
- **Evidence:** `admin-screens/signin.png` (first empty-form screenshot), flakiness between audit run 1 and run 2.

### [P0] `/pick-tenant` forces store picker on single-store subdomain AND exposes another tenant
- **App:** admin
- **Route:** `/pick-tenant` (after sign-in from `india-store-admin.mark8ly.com`)
- **Repro:** Sign in at `india-store-admin.mark8ly.com/login` → lands on `/pick-tenant`.
- **Expected:** `india-store-admin` subdomain implies tenant `india-store` — should auto-select and land on `/dashboard`. Owner of "india store" should not see unrelated tenants.
- **Actual:** Picker shows **two cards**: "demo store" (role: staff) and "india store" (role: owner). The "demo store" likely belongs to a different testing account but is surfaced on the india-store subdomain. Cross-tenant visibility + unnecessary picker step.
- **Security note:** If "demo store" belongs to another real customer, this is a **tenancy boundary violation**. Investigate whether the demo-store staff role is legitimate or a leftover tuple.
- **Evidence:** First audit run's `admin-screens/signin.png`.

### [P0] `/marketing` returns 404 — sidebar nav link to nowhere
- **App:** admin
- **Route:** `/marketing`
- **Repro:** Click "Marketing" in sidebar.
- **Actual:** `GET /marketing → 404`. Renders "This page doesn't exist" error page. The sidebar still has "Marketing" at the top level with a chevron suggesting submenus.
- **Evidence:** `admin-screens/marketing.png`, `failedRequests` has `GET /marketing status=404`.

### [P0] `/support` returns 404 — sidebar nav link to nowhere
- **App:** admin
- **Route:** `/support`
- **Repro:** Click "Support" in sidebar.
- **Actual:** `GET /support → 404`. Renders "This page doesn't exist". Sidebar exposes the link but the route doesn't exist.
- **Evidence:** `admin-screens/support.png`, `failedRequests` has `GET /support status=404`.

### [P0] `/settings/audit-logs` crashes with Server Components render error
- **App:** admin
- **Route:** `/settings/audit-logs`
- **Repro:** Navigate directly (there is no sidebar link — the Settings sub-nav is missing this item).
- **Actual:** Renders the "We couldn't load these settings" error boundary. Console: `Error: An error occurred in the Server Components render. The specific message is omitted in production builds … A digest property is included … digest 3416896015`.
- **Impact:** Audit logs are completely inaccessible in prod. Digest should correlate to server-side stack trace in logs.
- **Evidence:** `admin-screens/settings_audit_logs.png`, `consoleErrors` on that route.

### [P0] `/settings/general` silently redirects to `/settings/stores` (broken route)
- **App:** admin
- **Route:** `/settings/general`
- **Repro:** `GET https://india-store-admin.mark8ly.com/settings/general`
- **Actual:** The page renders the Stores page (URL stays `/settings/general` in first load, then navigation stabilizes on `/settings/stores`). The `/settings/general` route is a ghost from the recent IA rename (commit `a9ca5a2` renamed general → stores in the e2e tests) — the server still serves it but with wrong content.
- **Expected:** Either a clean 301 redirect to `/settings/stores` or a proper 404.
- **Evidence:** `admin-screens/settings_general.png` (identical to `settings_stores.png`).

### [P0] Admin Settings sub-nav is missing 7 of 12 children
- **App:** admin
- **Route:** any `/settings/*`
- **Repro:** Open `/settings/stores`, `/settings/team`, `/settings/audit-logs` etc. Inspect the Settings sub-nav in the left sidebar.
- **Actual:** Sub-nav shows only **STORE** (Stores, Themes, Domains) + **SELLING** (Payments, Shipping). Missing entirely: **Team**, **Account**, **Notifications**, **Subscription**, **Audit logs**, **Tax** (now merged into Payments? but still a routed page), **General** (dead route). User has no sidebar way to reach /settings/team, /settings/account, /settings/notifications, /settings/subscription, or /settings/audit-logs — they must know the URLs.
- **Impact:** 7 critical settings surfaces are hidden. Team management, subscription/billing, and audit logs are all unreachable via nav.
- **Evidence:** `admin-screens/settings_stores.png`, `admin-screens/settings_team.png`, `admin-screens/settings_audit_logs.png` — all show the same incomplete sub-nav.

### [P0] Storefront subdomain forces admin through `/pick-tenant` with cross-tenant data
_See "[P0] `/pick-tenant` forces store picker…" above — listed once under admin._

---

### [P1] Admin `/settings/tax` redirects to `/settings/payments` with no tax-only view
- **App:** admin
- **Route:** `/settings/tax`
- **Repro:** `GET /settings/tax`
- **Actual:** URL resolves to a combined "Payments & Tax" page. The Tax section inside it says "Tax calculation is determined by your store's country (IN) and cannot be changed here" — i.e., the section is read-only. There is no sidebar link to Tax (again, sub-nav gap). IA mismatch: the route exists but is essentially a dead redirect.
- **Expected:** Either remove the `/settings/tax` route or give Tax a real dedicated page with controls.
- **Evidence:** `admin-screens/settings_tax.png`.

### [P1] Prefetch storm: every admin page aborts 5–16 GETs to sibling admin routes
- **App:** admin
- **Route:** all
- **Repro:** Navigate any admin route. Watch `failedRequests` in audit findings.
- **Actual:** Every admin route shows aborted GETs to `/dashboard`, `/orders`, `/products`, `/settings/*` as `net::ERR_ABORTED`. `/settings/general` hits 16 aborted prefetches, `/settings/tax` hits 14, `/settings/audit-logs` hits 11.
- **Analysis:** Next.js `<Link>` prefetches sibling pages on viewport visibility; when the user clicks another link mid-prefetch, Chrome aborts the in-flight request (expected Chrome behavior). But 16 per page is excessive — it suggests `prefetch={true}` on every sidebar and sub-nav link without conditional gating. This bloats bandwidth, pollutes server logs, and makes real failures harder to spot.
- **Fix direction:** Set `prefetch={false}` on sidebar links, or upgrade to Next.js 16 hover-intent prefetch. Audit Link usage in `components/shell/AdminShell.tsx`.
- **Evidence:** `admin-findings.json` — see `failedRequests[*].failure == "net::ERR_ABORTED"` counts per route.

### [P1] Cloudflare `/cdn-cgi/rum` beacon aborts on every page load
- **App:** admin (and storefront)
- **Repro:** Navigate any page.
- **Actual:** `POST https://india-store-admin.mark8ly.com/cdn-cgi/rum` and `POST https://india-store.mark8ly.com/cdn-cgi/rum` fail with `net::ERR_ABORTED` on every route.
- **Impact:** Cloudflare Real User Monitoring is broken — you're paying for visibility you're not getting. Low user-visible impact but zero observability on real-user performance.
- **Notes:** Likely a Cloudflare Web Analytics misconfig on the zone. Check the Cloudflare dashboard.

### [P1] Admin dashboard greets user with uppercase email, not name
- **App:** admin
- **Route:** `/dashboard`
- **Actual:** Page renders "WELCOME BACK, MAHESH.SANGAWAR@GMAIL.COM" in the small-caps eyebrow — uppercasing an email address is a copy/design bug. Emails should never be rendered uppercase (breaks visual recognition, some hosts are case-sensitive). And using the email as a greeting is impersonal.
- **Expected:** "Welcome back, Mahesh" or "Welcome back" (no personalization) or use business display name.
- **Evidence:** `admin-screens/dashboard.png`.

### [P1] Store name persists as lowercase "india store" everywhere
- **App:** admin
- **Route:** `/settings/stores`, `/dashboard`, sidebar switcher
- **Actual:** The store is rendered as `india store` in the sidebar, in the /pick-tenant picker, in the Stores settings page, and as the page title. No capitalization applied.
- **Expected:** Merchant-provided casing should be preserved (or at least title-cased for display). If the merchant typed "India Store", show "India Store".
- **Evidence:** `admin-screens/settings_stores.png`, `admin-screens/dashboard.png`.

### [P1] Settings/Stores timezone default is "Australia/Sydney" for an India store
- **App:** admin
- **Route:** `/settings/stores`
- **Actual:** `Timezone` field shows "Australia/Sydney" as a placeholder/default. Country is "IN". The default is leaking from some geographically irrelevant fallback.
- **Expected:** Default to `Asia/Kolkata` for India, or to empty with a "select" placeholder.
- **Evidence:** `admin-screens/settings_stores.png`, also visible in the storefront header chip on `storefront-screens/root.png`.

### [P1] Products empty state has two different "New product" button styles
- **App:** admin
- **Route:** `/products`
- **Actual:** Two CTAs on the same screen with inconsistent styling:
  1. Top-right: outline pill with moss "+ New product" label and icon
  2. Center empty-state: dark pill "+ New product"
- **Expected:** One canonical button style per design system. Both CTAs should use the same `@tesserix/web` `Button` variant (primary dark).
- **Evidence:** `admin-screens/products.png`.

### [P1] `/products/new` handle preview uses `mark8ly.com` not the store subdomain
- **App:** admin
- **Route:** `/products/new`
- **Actual:** Handle field help text says `Leave empty to auto-generate from the title. mark8ly.com/<handle>` — the URL shown is the root domain, not `india-store.mark8ly.com/<handle>` where the product will actually live.
- **Impact:** Misleads the merchant into thinking product URLs are at the marketing site, not their storefront.
- **Evidence:** `admin-screens/products_new.png`.

### [P1] `/products/new` defaults: price `19.99` and stock `0`
- **App:** admin
- **Route:** `/products/new`
- **Actual:** Price field shows `19.99` as placeholder (in INR that's ₹19.99 — too cheap to be a sensible default for any real product). Stock field defaults to `0` — so a new product can never be purchased without the merchant remembering to edit stock.
- **Expected:** Price empty (no placeholder value) or a locale-appropriate example; stock default should match whether inventory tracking is on (`null` / "unlimited" if off, or prompt to enter).
- **Evidence:** `admin-screens/products_new.png`.

### [P1] Orders empty state has no list affordances — no table, no filters, no CTA
- **App:** admin
- **Route:** `/orders`
- **Actual:** Just "No orders yet" heading + two-line description. No table skeleton, no filter bar, no "Create test order" CTA, no tip about placing a test order. Users can't imagine what the page will look like.
- **Evidence:** `admin-screens/orders.png`.

### [P1] `/settings/stores` owner email, slug, and timezone all deferred to "contact support"
- **App:** admin
- **Route:** `/settings/stores`
- **Actual:** Three fields ship with "contact support" fallback copy:
  - Store URL slug: "Changing your slug would break existing links. Contact support if you need a new one."
  - Owner email: "Your sign-in email. Contact support to transfer ownership."
  - Timezone: "Timezone editing is coming in a follow-up. Contact support if you need it changed now."
- **Impact:** Three basic store settings that merchants will eventually need are all gated behind a support ticket. The user is the founder — there is no "support".
- **Evidence:** `admin-screens/settings_stores.png`.

---

### [P2] Eyebrow + H1 duplication pattern on every admin page
- **App:** admin
- **Route:** all
- **Actual:** The section top-bar shows `<SECTION>` eyebrow + page heading, and the body repeats `<SECTION>` eyebrow + H1 again. Examples:
  - Dashboard: top "OVERVIEW / Dashboard", body "OVERVIEW / Dashboard / WELCOME BACK … / Dashboard" — the word "Dashboard" appears 3×.
  - Products: top "PRODUCTS / Products", body "CATALOGUE / Products" — the word "Products" appears 3×.
  - Orders: top "OPERATIONS / Orders", body "OPERATIONS / Orders" — doubled.
- **Expected:** Either the top bar carries a breadcrumb (India Store > Products) OR the body page header, not both with the same title. Design system says the H1 does the work, the eyebrow is context.
- **Evidence:** all `admin-screens/*.png`.

### [P2] "OVERVIEW" eyebrow in body competes with "Dashboard" section title
- **App:** admin
- **Route:** `/dashboard`
- **Actual:** Body has `OVERVIEW` eyebrow followed by H1 `Dashboard`. The eyebrow adds no new information.
- **Evidence:** `admin-screens/dashboard.png`.

### [P2] Top-right user pill truncates email awkwardly
- **App:** admin
- **Route:** all
- **Actual:** `mahesh.sangawar@gmail....` — truncated mid-word with trailing dots. The pill is too narrow to hold a long email.
- **Expected:** Show avatar + short display name, hover/click for full email. Or truncate with proper ellipsis before `@` (`mahesh…@gmail.com`).
- **Evidence:** every admin screenshot.

### [P2] Sidebar duplication: "INDIA STORE" label above "SWITCH STORE" dropdown
- **App:** admin
- **Route:** all
- **Actual:** The sidebar shows the label "INDIA STORE" as an eyebrow + a "SWITCH STORE" dropdown containing `india store / OWNER`. The store name appears twice stacked.
- **Expected:** Pick one — either the top label or the dropdown, not both.
- **Evidence:** every admin screenshot.

### [P2] Sidebar chevrons imply submenus where there aren't any
- **App:** admin
- **Route:** sidebar nav
- **Actual:** "Customers" has a `›` chevron but (based on audit) doesn't appear to have children. "Marketing" and "Support" have chevrons but the routes 404. "Settings" has a chevron AND shows an expanded sub-nav — inconsistent with how other items behave.
- **Evidence:** every admin screenshot.

### [P2] Empty-state headings use same H1 weight as page titles — visual competition
- **App:** admin
- **Routes:** `/products` ("No products yet"), `/orders` ("No orders yet")
- **Actual:** The empty-state heading is rendered at the same size/weight as the page title, creating two H1s on the same page with no hierarchy.
- **Expected:** Empty-state heading should be smaller/lighter, or integrated into a bordered empty-state card.
- **Evidence:** `admin-screens/products.png`, `admin-screens/orders.png`.

### [P2] Country/currency shown as codes ("IN", "INR") instead of full names
- **App:** admin
- **Route:** `/settings/stores`, `/dashboard` welcome copy, `/settings/payments` ("your store's country (IN)")
- **Actual:** Fields read `IN` and `INR`. Body copy says "(IN)".
- **Expected:** "India" and "Indian Rupee (INR)" / "INR — ₹". Codes are for system use, not the admin UI.
- **Evidence:** `admin-screens/settings_stores.png`, `admin-screens/settings_tax.png`.

### [P2] Case inconsistency: "Payments & Tax" vs "Payments & tax"
- **App:** admin
- **Route:** `/settings/payments` (aka `/settings/tax`)
- **Actual:** Top bar eyebrow says `Payments & Tax`; body H1 says `Payments & tax`.
- **Evidence:** `admin-screens/settings_tax.png`.

### [P2] Unfinished features admitted in prod body copy
- **App:** admin
- **Routes:** `/products/new` ("Plain text only for now; rich text lands in a follow-up"), `/settings/stores` ("Timezone editing is coming in a follow-up")
- **Impact:** Customers shouldn't see "coming in a follow-up" disclaimers — ship the feature or hide the surface.
- **Evidence:** `admin-screens/products_new.png`, `admin-screens/settings_stores.png`.

### [P2] Admin 404 page content is bottom-anchored, leaving huge empty top
- **App:** admin
- **Routes:** `/marketing`, `/support`
- **Actual:** The "ERROR 404 / This page doesn't exist" text sits roughly halfway down the viewport with nothing above it. Visually uncentered.
- **Evidence:** `admin-screens/marketing.png`, `admin-screens/support.png`.

---

### [P3] Signed-in chrome shows "OWNER" badge next to avatar — redundant with sidebar
- **App:** admin
- **Actual:** Top-right header shows a green `OWNER` pill next to avatar. The sidebar Store Switcher also shows `OWNER` under the store name. Two pills for the same fact.

### [P3] `/settings/themes` "Themes & Branding" vs "Themes & branding" case inconsistency
- **App:** admin
- **Route:** `/settings/themes`
- **Actual:** Eyebrow title-case, H1 sentence-case.
- **Evidence:** `admin-screens/settings_themes.png`.

### [P3] `/settings/themes` Corner style section appears empty/cropped
- **App:** admin
- **Route:** `/settings/themes`
- **Actual:** Near the bottom of the page, the "Corner style" row has no visible preview tiles or empty state — possibly cut off by the viewport, possibly empty.
- **Evidence:** `admin-screens/settings_themes.png` (bottom).

---

## Storefront issues

### [P0] Checkout calls `http://localhost:8088/` in production for payment methods
- **App:** storefront
- **Route:** `/checkout`
- **Repro:** `GET https://india-store.mark8ly.com/checkout` — inspect network tab.
- **Actual:** Browser attempts `GET http://localhost:8088/api/v1/storefront/stores/india-store/payment-methods` and fails with `net::ERR_CONNECTION_REFUSED`. Same pattern as the admin `0.0.0.0:4202` leak — a local dev URL baked into the production bundle.
- **Impact:** Payment methods cannot load → **checkout is broken end-to-end** in prod. Customers cannot pay.
- **Fix direction:** Audit `next.config.*`, `.env.production`, and any `process.env.NEXT_PUBLIC_*_URL` defaults. The URL should come from a per-environment secret, not a hardcoded fallback.
- **Evidence:** `storefront-findings.json` route `/checkout`; see `consoleErrors` and `failedRequests`.

### [P0] Storefront `/categories` returns 404
- **App:** storefront
- **Route:** `/categories`
- **Actual:** Server returns 404. The page renders a generic "Page not found / Continue shopping / Browse categories" (where the CTA "Browse categories" loops back to the same 404).
- **Impact:** No category index page. Customers cannot browse by category.
- **Evidence:** `storefront-screens/categories.png`, `consoleErrors` shows `Failed to load resource: 404`.

### [P0] Storefront `/orders` returns 404
- **App:** storefront
- **Route:** `/orders`
- **Actual:** Same 404 page as `/categories`. The route exists in the codebase (`app/orders/`) but serves 404 in prod.
- **Evidence:** `storefront-screens/orders.png`, `failedRequests` has `GET /orders status=404`.

### [P0] Storefront home is 100% placeholder content
- **App:** storefront
- **Route:** `/`
- **Actual:** The live home page for `india-store.mark8ly.com` ships with editorial placeholder copy and dummy tiles:
  - "india store — A considered storefront for india store — a place where the catalog feels written, not assembled."
  - "Three pieces we love right now" section with `N° 01 / N° 02 / N° 03` grey placeholder tiles labelled "RESERVED FOR PRODUCT PHOTOGRAPHY".
  - "Built for the long shelf life" editorial section with dummy "Workshop" image.
  - "Cover story" placeholder card on the right.
- **Impact:** Real customers land on a page advertising three products that don't exist. This is a **trust-destroying** empty state — worse than a plain "coming soon" page.
- **Notes:** The "We chose fewer things and we chose them well" + "Three pieces we love right now" copy also hardcodes the number three, which will always be wrong for the first N-1 products and won't update with the catalog.
- **Evidence:** `storefront-screens/root.png`.

### [P0] Storefront sign-in link visible in nav but flow is broken (user-reported)
- **App:** storefront
- **Route:** every page with top nav (`/`, `/checkout`, `/gift-cards`)
- **Actual:** Top nav shows `Home / Shop / Cart / Sign in` — the Sign in link is visible but (per user) the sign-in flow is broken.
- **Expected:** If sign-in is known broken, hide the nav link until it's fixed, or route it to a "sign-in coming soon" page.
- **Evidence:** `storefront-screens/root.png`, `storefront-screens/checkout.png`, `storefront-screens/gift_cards.png`.

---

### [P1] Storefront top nav overflows the viewport (both sides clipped)
- **App:** storefront
- **Route:** `/gift-cards` (and any page with the full nav)
- **Actual:** At 1440×900 viewport, the top nav left-clips the store name ("india store" → "Store") and right-clips the last nav item ("Sign in" partially hidden at right edge). The content container does not account for its own padding.
- **Expected:** Nav should fit within the container's max-width with safe gutters on both sides.
- **Evidence:** `storefront-screens/gift_cards.png`.

### [P1] `/cart` and `/account` have NO site chrome (no header/nav/footer)
- **App:** storefront
- **Routes:** `/cart`, `/account`
- **Actual:** These pages render bare content (just the heading + body) with no top nav, no store header, no footer. Other pages (`/`, `/checkout`, `/gift-cards`) have the full chrome.
- **Impact:** Cart and account feel "decapitated" — the customer loses navigation context.
- **Evidence:** `storefront-screens/cart.png`, `storefront-screens/account.png`.

### [P1] `/account` shows "My Account" sub-nav even when signed out
- **App:** storefront
- **Route:** `/account`
- **Actual:** Page renders Dashboard / Orders / Addresses sub-nav tabs and the H1 "My Account", with body "Please sign in to view your account." — but there is NO sign-in button or link anywhere on the page. The user is told to sign in but given no affordance to do so.
- **Expected:** Either redirect anonymous users to a sign-in page, or show a prominent "Sign in" CTA, or hide the sub-nav until authenticated.
- **Evidence:** `storefront-screens/account.png`.

### [P1] Storefront home header shows "Australia/Sydney" timezone chip for India store
- **App:** storefront
- **Route:** `/`
- **Actual:** The header shows `Currency INR / Country IN / Australia/Sydney` — the timezone chip is wrong and globally visible.
- **Expected:** `Asia/Kolkata` or hide the chip entirely (timezone is not customer-facing information).
- **Evidence:** `storefront-screens/root.png`.

### [P1] Storefront `/gift-cards` is "coming soon" placeholder in prod
- **App:** storefront
- **Route:** `/gift-cards`
- **Actual:** "Gift Cards / Gift cards are coming soon. Check back later to purchase a gift card for someone special." Live URL in prod advertising a feature that doesn't exist.
- **Expected:** Hide the nav entry and return 404, or finish the feature.
- **Evidence:** `storefront-screens/gift_cards.png`.

---

### [P2] Storefront country shown as "IN" instead of "India"
- **App:** storefront
- **Route:** `/`
- **Actual:** Header chip shows `Country IN`. Same code-instead-of-name issue as admin.
- **Evidence:** `storefront-screens/root.png`.

### [P2] Storefront home editorial copy contradicts an empty catalog
- **App:** storefront
- **Route:** `/`
- **Actual:** Copy claims "We chose fewer things and we chose them well" and "Three pieces we love right now" while the store has zero products. Copy assumes catalog content exists.
- **Expected:** Either a real empty-state or copy that gracefully handles the "no products" case.
- **Evidence:** `storefront-screens/root.png`.

### [P2] Storefront font on cart/account/gift-cards pages looks different from home
- **App:** storefront
- **Actual:** The home page heading ("india store") uses Source Serif; /cart page H1 "Cart" uses a different serif weight/spacing; /account "My Account" yet another; /gift-cards "Gift Cards" appears sans-serif-ish. Possibly different font loading across routes or inconsistent heading components.
- **Evidence:** compare `storefront-screens/root.png` vs `cart.png` vs `account.png` vs `gift_cards.png`.

---

### [P3] Storefront cart empty-state link arrow inconsistent
- **App:** storefront
- **Route:** `/cart`, `/checkout`
- **Actual:** `/cart` shows "Continue shopping →"; `/checkout` shows "Continue shopping" (no arrow). Inconsistent trailing glyph.
- **Evidence:** `storefront-screens/cart.png`, `storefront-screens/checkout.png`.

---

## Cross-cutting themes (fix once → fix many)

1. **Hardcoded localhost URLs baked into the production bundle.** Seen twice (`0.0.0.0:4202` in admin login RSC, `localhost:8088` in storefront checkout). Root cause is almost certainly a `process.env.*_URL ?? "http://localhost:..."` fallback in config that survives into the prod build. **Grep the repo for `localhost:`, `0.0.0.0:`, and `|| "http://` in any file compiled into client bundles** and replace with build-time validation (throw on missing).

2. **Eyebrow + H1 duplication across every admin page.** Single component pattern (probably `AdminPage.tsx`) is doing both, or two different layout primitives are fighting. Normalize into one header primitive.

3. **Country / currency / timezone rendering.** Codes leak into UI copy across both apps (`IN`, `INR`, `Australia/Sydney`). Centralize i18n display formatting.

4. **Settings IA.** Recent rename (themes/stores/general) left a trail of dead routes, hidden sub-nav children, and broken redirects. A full re-audit of `app/(admin)/settings/**` routes vs the sidebar sub-nav model is needed.

5. **Prefetch budget.** Every admin page is over-prefetching. One `prefetch={false}` pass on sidebar links in `components/shell/AdminShell.tsx` will fix a pile of `ERR_ABORTED` entries.

6. **"Contact support" as a feature placeholder.** Multiple flows defer to support (slug change, owner transfer, timezone edit). For a self-serve SaaS, this is the wrong escape hatch.

7. **Placeholder copy shipped to prod** (storefront home tiles, gift cards "coming soon", rich text "in a follow-up"). Ship or hide — don't advertise absence.

---

## Not yet covered (next audit pass)

The audit crawled static routes only. Still unexercised:
- **Admin CRUD flows**: create/edit/delete product, place test order, invite teammate (blocked by Settings IA). Planned for the next audit pass now that sign-in is reliable.
- **Admin multi-store switching**: click the SWITCH STORE dropdown and verify the demo-store leak boundary.
- **Storefront PDP**: no products exist, so `/products/[handle]` could not be crawled. Needs a seed product before the audit can cover PDP, add-to-cart, and cart→checkout with items.
- **Admin `/products/[id]` edit page**: not crawled because no products exist. Same seeding dependency.
- **Admin forgot-password, reset-password, accept-invite**: not crawled — these need out-of-band flows (email tokens).
- **Keyboard-only navigation and screen reader** checks: the design system claims WCAG 2.1 AA but the audit only sampled visual state.
- **Mobile breakpoints**: audit ran at 1440×900 only. Given the storefront nav overflows there, mobile is likely worse.
