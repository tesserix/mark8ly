# Dashboard, Support & Help — Unified Design Spec

**Date:** 2026-04-10
**Status:** Approved

## 1. Overview

Three milestones completing the admin experience: a data-rich dashboard landing page with setup checklist and revenue sparkline, a basic support ticket system, and a markdown-based help center with contextual links from every feature page. All inside the existing `apps/admin` Next.js app and `marketplace-api` backend.

### Build order

1. **D1 — Dashboard** (stat cards, sparkline, setup checklist, recent orders, top products, low stock)
2. **D2 — Tickets** (basic create/list/view/reply, 3 statuses, 3 priorities)
3. **D3 — Help Center** (markdown articles in repo, search, contextual "Learn more" links)

### Constraints

- Same binary (`marketplace-api`), same admin app (`apps/admin`)
- Dashboard queries existing tables — no new analytics service, no FDW
- Tickets are simple — 3 statuses (open/resolved/closed), no assignees, no attachments
- Help content is markdown in `apps/admin/content/help/` — no CMS, no external docs site
- Old app (`mark8ly_backup`) is reference only

## 2. Sidebar Update

Remove Analytics section. Add Support section:
```
Products       → /products
Orders         ▸ All Orders, Returns & Refunds, Abandoned Carts
Customers      ▸ All Customers, Reviews
Marketing      ▸ Coupons, Gift Cards, Loyalty, Campaigns
Support        ▸ Tickets, Help Center
Settings       ▸ Store Settings, Storefront, Stores, Team, Payments, Shipping, Tax,
                 Domains, Subscription, Account, Audit Logs, Notifications
```

6 sidebar sections total.

## 3. Data Model

### 3.1 No migration for D1

Dashboard queries existing tables: orders, products, payment_transactions, customer_profiles (if C2 shipped), reviews (if C3 shipped). All aggregation queries.

### 3.2 Migration 000018 — Tickets (D2)

```sql
CREATE TABLE tickets (
    id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID          NOT NULL,
    store_id          UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    ticket_number     VARCHAR(20)   NOT NULL,
    subject           VARCHAR(300)  NOT NULL,
    description       TEXT          NOT NULL,
    status            VARCHAR(20)   NOT NULL DEFAULT 'open',
    priority          VARCHAR(10)   NOT NULL DEFAULT 'medium',
    submitted_by_name VARCHAR(200)  NOT NULL,
    submitted_by_email VARCHAR(300) NOT NULL,
    resolved_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, ticket_number)
);
CREATE INDEX tickets_store_status_idx ON tickets (store_id, status);

CREATE TABLE ticket_replies (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id       UUID          NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    author_type     VARCHAR(20)   NOT NULL,
    author_name     VARCHAR(200)  NOT NULL,
    author_email    VARCHAR(300),
    content         TEXT          NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX tr_ticket_idx ON ticket_replies (ticket_id);
```

Status values: `open`, `resolved`, `closed`
Priority values: `low`, `medium`, `high`
Author type: `merchant` (store staff), `platform` (Mark8ly support team)

### 3.3 No migration for D3

Help content is markdown files in `apps/admin/content/help/`. No database.

## 4. D1 — Dashboard

### 4.1 API

`GET /admin/stores/:storeId/dashboard` — single endpoint returning all dashboard data:

```json
{
  "stats": {
    "revenue_today": "1250.00",
    "revenue_week": "8430.00",
    "revenue_month": "32100.00",
    "revenue_change_pct": 12.5,
    "revenue_trend": [120, 180, 95, 210, 165, 190, 240],
    "orders_today": 12,
    "orders_pending": 3,
    "orders_fulfilled": 8,
    "orders_cancelled": 1,
    "customers_total": 142,
    "customers_new_this_week": 7,
    "pending_reviews": 4
  },
  "recent_orders": [
    { "id": "...", "order_number": "ORD-0042", "customer_email": "...", "grand_total": "89.00", "status": "confirmed", "created_at": "..." }
  ],
  "top_products": [
    { "id": "...", "title": "...", "revenue": "4200.00", "units_sold": 42, "image_url": "..." }
  ],
  "low_stock": [
    { "id": "...", "title": "...", "variant_title": "...", "quantity": 3, "low_stock_threshold": 10 }
  ],
  "setup_checklist": {
    "has_store": true,
    "has_product": true,
    "has_payment_provider": true,
    "has_shipping_carrier": false,
    "has_tax_configured": true,
    "has_custom_domain": false,
    "has_storefront_theme": false,
    "has_test_order": false
  }
}
```

Queries:
- Revenue: `SELECT SUM(grand_total) FROM orders WHERE store_id = ? AND status NOT IN ('cancelled') AND created_at >= ?`
- Revenue trend: 7-day grouped by date
- Orders: `SELECT status, COUNT(*) FROM orders WHERE store_id = ? AND created_at >= today GROUP BY status`
- Recent orders: `SELECT ... FROM orders WHERE store_id = ? ORDER BY created_at DESC LIMIT 5`
- Top products: `SELECT product_id, SUM(line_total) as revenue, SUM(quantity) as units FROM order_items JOIN orders ... GROUP BY product_id ORDER BY revenue DESC LIMIT 5`
- Low stock: `SELECT ... FROM product_variants WHERE store_id = ? AND inventory_quantity <= low_stock_threshold AND inventory_quantity > 0`
- Checklist: individual EXISTS queries per item

All cached in-memory 60s.

### 4.2 Setup Checklist

| Step | Label | Query | Link |
|------|-------|-------|------|
| 1 | Create your store | stores count > 0 | /settings/stores |
| 2 | Add your first product | products count > 0 | /products/new |
| 3 | Configure payment provider | payment_gateway_configs active > 0 | /settings/payments |
| 4 | Configure shipping carrier | shipping_carrier_configs active > 0 | /settings/shipping |
| 5 | Set up tax settings | always true (supported_countries seeded) | /settings/tax |
| 6 | Connect a custom domain | custom_domains active > 0 | /settings/domains |
| 7 | Customize your storefront | storefront_theme not default | /settings/storefront |
| 8 | Place a test order | orders count > 0 | /products |

Checklist auto-hides when all 8 items are complete. Shows a progress indicator: "4 of 8 complete".

### 4.3 UI Layout

```
/dashboard
├── Setup checklist card (top, collapsible, hides when complete)
│   ├── Progress bar (moss-700 fill)
│   └── Checklist items with links
├── Stat cards row (4 cards):
│   ├── Revenue today (serif numeral, % change badge)
│   ├── Orders today (count + pending/fulfilled breakdown)
│   ├── Total customers (+ new this week)
│   └── Pending reviews (or "No pending reviews")
├── Revenue sparkline (7-day, single moss-700 line, no axes, no grid)
│   └── Hover shows date + amount tooltip
├── Two-column section:
│   ├── Left: Recent orders (5 rows — number, customer, total, status badge, time ago)
│   └── Right: Top products (5 rows — image thumbnail, title, revenue, units)
└── Low stock alerts (conditional, only if items exist)
    └── List with product title, variant, current qty, threshold, "View" link
```

### 4.4 Design

- Stat cards: white elevated surface, serif numerals (Source Serif 4), `fontFeatureSettings: '"tnum" 1'`, ink-900 value, opacity-60 label
- Revenue change badge: moss-700 bg for positive, signal for negative, small pill
- Sparkline: single moss-700 stroke, no fill, 2px line, rounded caps, ~80px tall. Recharts `<Line>` with grid/axes hidden
- Setup checklist: hairline-ruled list, moss-700 filled circle + checkmark for complete, ink-900/20 empty circle for incomplete
- Recent orders / top products: hairline-ruled rows, no table borders, hover:opacity transition
- Low stock: signal-colored left border accent on each row

## 5. D2 — Tickets

### 5.1 API

**Admin:**
- `GET /admin/stores/:storeId/tickets` — list with status filter, search, pagination
- `POST /admin/stores/:storeId/tickets` — create (body: subject, description, priority)
- `GET /admin/stores/:storeId/tickets/:id` — detail with replies
- `POST /admin/stores/:storeId/tickets/:id/reply` — add reply
- `PATCH /admin/stores/:storeId/tickets/:id` — update status (resolve/close/reopen)

Ticket number: sequential per store, format `TKT-0001`. Generated via `SELECT COALESCE(MAX(...), 0) + 1` inside transaction (same pattern as order numbers).

### 5.2 UI Pages

```
/support/tickets              — list: status tabs (Open | Resolved | Closed)
                                search by subject/number, priority filter
                                each row: number, subject, priority badge, status badge, time ago
                                empty state per tab

/support/tickets/new          — create form:
                                subject input (required)
                                description textarea (required, min 20 chars)
                                priority select (low/medium/high, default medium)
                                submit button → redirect to ticket detail

/support/tickets/[id]         — detail:
                                ticket header (number, subject, status badge, priority badge, created date)
                                description block
                                reply thread (chronological, merchant vs platform visually distinct)
                                reply form at bottom (textarea + submit)
                                action buttons: Resolve / Close / Reopen (context-dependent)
```

### 5.3 Design

- Status badges: moss-700/10 bg for open, moss-700 bg for resolved, ink-900/10 for closed
- Priority badges: ink-900/40 for low, ink-900/60 for medium, signal for high
- Reply thread: merchant replies left-aligned with ink-900 name, platform replies right-aligned with moss-700 accent
- Empty state: "No open tickets. Need help? Check our Help Center or create a ticket."

## 6. D3 — Help Center

### 6.1 Content Structure

```
apps/admin/content/help/
├── getting-started.md
├── products.md
├── orders.md
├── payments.md
├── shipping.md
├── tax.md
├── customers.md
├── reviews.md
├── marketing.md
├── domains.md
├── team.md
├── subscription.md
├── storefront.md
└── troubleshooting.md
```

Each file has frontmatter:
```markdown
---
title: "Setting Up Payments"
category: "Store Setup"
order: 4
---

# Setting Up Payments

Mark8ly supports Stripe, Razorpay, and PayPal...
```

Categories: "Getting Started", "Store Setup", "Operations", "Marketing", "Troubleshooting"

### 6.2 UI Pages

```
/support/help                 — landing page:
                                search bar (filters articles by title/content match)
                                category grid (5 categories, each with article count)
                                featured articles list

/support/help/[slug]          — article page:
                                breadcrumb (Help Center → Category → Article)
                                title (serif heading)
                                rendered markdown (max-w-2xl, generous line-height)
                                "Was this helpful? Yes / No" feedback (stored nowhere — just UI comfort)
                                "Related articles" links at bottom
                                "Back to Help Center" link
```

### 6.3 Contextual Links

`HelpLink` component used across settings and feature pages:

```tsx
<HelpLink slug="payments" />
// renders: <a href="/support/help/payments" class="text-xs text-moss-700 opacity-60">Learn more →</a>
```

Added to page headers on: payments, shipping, tax, domains, products, orders, marketing settings.

### 6.4 Rendering

Server-side markdown rendering via `next-mdx-remote` or a simple `marked` + `DOMPurify` pipeline. No MDX components needed — plain markdown is sufficient. Styled with Tailwind prose classes adapted to Paper/Ink/Moss tokens.

### 6.5 Search

Client-side search on the help landing page. Load all article frontmatter (title, category, first 200 chars) at build time. Filter on keystroke. No backend search endpoint needed for ~15 articles.

## 7. Fixes from Specialist Reviews

### 7.1 Security fixes (from security review)

**Cache key MUST be `${tenantId}:${storeId}`** — in-memory cache scoped to prevent cross-tenant data leakage. Never serve cached data to a caller whose tenantId differs.

**All aggregation queries include `AND tenant_id = ?`** — not just store_id. Prevents cross-tenant data access via enumerated store UUIDs.

**Ticket access control: verify tenant_id + store_id on GET by ID** — prevents IDOR where staff from store A reads store B's tickets by guessing UUID.

**Ticket reply `author_type` validated server-side** — reject `platform` author_type from merchant sessions. Prevents impersonation of Mark8ly support.

**Dashboard response: `Cache-Control: private, no-store`** — prevents CDN/Cloudflare from caching tenant-specific revenue data.

**Help article slug: validate against allowlist** — use `path.basename(slug)` and check against known filenames before filesystem read. Prevents path traversal (`../../.env`).

**Help markdown rendering: use `marked` + `sanitize-html`** (not DOMPurify which needs DOM). Server-safe sanitization pipeline. Or `next-mdx-remote` with `rehype-sanitize` plugin.

**Ticket reply rendering: use `textContent` or DOMPurify on frontend** — never `dangerouslySetInnerHTML` for user-submitted reply content.

### 7.2 Architecture fixes (from architect review)

**Ticket number: use `UPDATE stores SET ticket_seq = ticket_seq + 1 RETURNING ticket_seq`** — not `SELECT MAX + 1`. Same fix as the orders sequence issue. Add `ticket_seq INT NOT NULL DEFAULT 0` column to stores table in migration 000018.

**Dashboard indexes: add composite indexes** in a separate migration or in D1 handler setup:
- `CREATE INDEX IF NOT EXISTS orders_store_status_created_idx ON orders (store_id, status, created_at)`
- `CREATE INDEX IF NOT EXISTS order_items_order_product_idx ON order_items (order_id, product_id, line_total, quantity)`

**Cache: consider Redis over in-memory** for multi-pod consistency. For MVP, in-memory with `${tenantId}:${storeId}` key is acceptable but document the trade-off (pods serve different data for up to 60s).

### 7.3 UX fixes (from UX review)

**Revenue sparkline inline with stat card** — don't separate sparkline from revenue card. Embed mini-sparkline (~60px) inside the revenue stat card to avoid two competing "revenue moments."

**Setup checklist: group into 3 phases** — "Store Setup" (store, product, storefront), "Payments & Shipping" (payment, shipping, tax), "Launch" (domain, test order). Show phase progress. Keep a persistent "Setup guide" link in settings after auto-hide.

**Empty dashboard state** — when `setup_checklist.has_test_order === false`, show the checklist prominently and hide stat cards/sparkline/tables. First-session merchants should see onboarding, not empty zeroes.

**Ticket tabs: show counts** — "Open (3)" not just "Open". Lets merchants triage at a glance.

**HelpLink contrast: remove opacity-60** — use `text-[color:var(--moss-700)]` at full opacity. Moss on paper-200 is already secondary without dimming below WCAG AA contrast.

**Sparkline zero-data state** — render dashed baseline with "No sales yet" ghost label instead of a flat active line.

**Help search: index full body truncated at 500 chars + all H2/H3 headings** — covers deeper content matches.

## 8. Security

- Dashboard: scoped by `tenant_id + store_id`, `Cache-Control: private, no-store`, cache keyed `${tenantId}:${storeId}`
- Tickets: scoped by `tenant_id + store_id`, IDOR check on get-by-id, reply content sanitized (bluemonday server-side + textContent/DOMPurify client-side), author_type validated against session
- Help content: static markdown, slug validated against allowlist, rendered with `sanitize-html` server-side

## 9. Testing

- **D1:** Dashboard endpoint returns correct stats (seed orders + products, verify counts). Setup checklist accurately reflects state. Revenue trend has 7 data points. Empty store returns zeroes + empty dashboard state. Cache key isolation (store A data never served to store B). All queries include tenant_id.
- **D2:** Ticket CRUD, status transitions (open→resolved→closed, open→closed, resolved→reopen→open), reply append, sequential ticket number via `UPDATE RETURNING` (concurrent test), search by subject, IDOR rejection (wrong tenant), author_type validation (reject platform from merchant)
- **D3:** Markdown renders correctly, search filters by title + headings, HelpLink component renders correct href, 404 on unknown slug, path traversal rejected (`../../` in slug returns 404)

## 10. Out of Scope

- Real-time dashboard updates (WebSocket) — 60s cache is sufficient
- Advanced analytics (cohort analysis, funnel visualization, export) — follow-up
- Ticket assignments, SLAs, escalation, attachments — follow-up
- Help article versioning or translation — follow-up
- Help article feedback persistence (Yes/No stored) — follow-up
- Platform admin ticket management UI — follow-up (platform-side)
- Redis cache for dashboard — follow-up when multi-pod is needed
