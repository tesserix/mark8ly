# In-store receipts: page, email, resend (#1106, #1107) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After an in-store sale the seller can email the customer a link to a receipt page on the store's own address, or print it; the link keeps showing items and totals for good, and the receipt can be re-sent from the order page.

**Architecture:** A receipt link is a long random token whose SHA-256 is stored in a new `receipt_links` table (one order can have many links; old ones keep working). marketplace-api serves the receipt through the storefront API by token, and mints and emails links through an in-store admin route and the existing order-document mailer. The storefront renders `/r/<token>` with a print stylesheet; the admin adds a receipt step to "Sale recorded" and a "Send receipt" action on in-store orders.

**Tech Stack:** Go (gin, gorm, Postgres), Next.js storefront and admin, vitest.

**Spec:** `docs/superpowers/specs/2026-11-14-in-store-sales-design.md` ("Receipt page"; "A receipt email is not marketing consent")

## Global Constraints

- Served on the store's own address: `<store>.mark8ly.com/r/<token>` (custom domains: the storefront serves the same route on any host it already resolves).
- "Each line: product photo, title, variant / personalisation summary, quantity, price. Subtotal, discount, total in the store currency, and GST included."
- "Buy again / link to the shop."
- "Items and totals stay visible on the link indefinitely. Contact and payment details are shown only on a fresh link and hide after a window; afterwards the page offers 'Send a new link' to the email on the order (never revealing it)."
- "A print stylesheet so the iPad's normal print (AirPrint) gives a clean receipt. No printer integration."
- Email: "short message with the order number, the store name and the link, through the existing order-document mailer and templates."
- "The address is saved on the order for receipts only and is not added to campaigns or segments."
- Never store the raw token; never put an email address in a URL.

## Review Focus

1. A guessed or expired-format token → plain 404, no hint whether an order exists. → Task 1 test `TestReceipt_UnknownToken404`.
2. A token from store A used on store B's address → 404. → Task 1 test `TestReceipt_OtherStore404`.
3. "Send a new link" pressed repeatedly → at most one email per 10 minutes per order, and the response never reveals the address. → Task 1 test `TestReceiptResend_RateLimitedAndSilent`.
4. A cashier sending a receipt for someone else's sale → refused; staff and above may send for any in-store order. → Task 1 test `TestReceiptLink_CashierOwnSalesOnly`.
5. An in-store order with no email: Print works, Email asks for an address, and nothing is ever mailed to `''`. → Task 3 tests.

## Rulings made while planning

- Fresh window: **7 days** from the link's creation. Cost if wrong: a customer opening an old link sees no payment method — they can ask for a new link.
- Contact detail shown on a fresh link: the masked email (`j***@gmail.com`) and the payment method; nothing more exists on an in-store order.
- Links are minted only on request (Email or Print), not at sale time. Cost if wrong: none.
- Storefront base URL: the existing `{slug}` template the order-document mailer uses. Custom-domain links: later. Cost if wrong: the link shows the mark8ly subdomain for a store with a custom domain.
- Migration number 000146 (000145 is in #1118). **#1118 must merge first**: golang-migrate never applies a lower version after a higher one.

---

### Task 1: API — receipt links, receipt read, resend, email

**Files:**
- Create: `services/marketplace-api/migrations/000146_receipt_links.{up,down}.sql` — `receipt_links(token_hash char(64) PRIMARY KEY, order_id uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE, store_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now())`, index `(order_id, created_at DESC)`.
- Create: `services/marketplace-api/internal/instore/receipt.go` — `type ReceiptService`, `NewReceiptService(db, mailer ReceiptMailer, urlBase string)`, `Issue(ctx, storeID, orderID uuid.UUID) (url string, err error)`, `Send(ctx, storeID, orderID uuid.UUID, email string) (url string, err error)` (sets `customer_email` when given, then Issue + mail), `Load(ctx, storeID uuid.UUID, token string) (*Receipt, error)`, `Resend(ctx, storeID uuid.UUID, token string) error`.
- Modify: `internal/orderdoc/service.go` + `mailer.go` — `SendInStoreReceipt(ctx, orderID uuid.UUID, receiptURL string) error`: KindReceipt envelope with `InStore: true` (heading "Thanks for shopping with us.", lede "Here's your receipt for order X.", no PDF attachment, DocumentURL = receiptURL, no OrderURL).
- Modify: `internal/handlers/admin/instore.go`, `routes.go` — `POST /in-store/sales/:id/receipt` body `{email?: string, send: bool}` → `{url}`.
- Create: `internal/handlers/storefront/receipts.go`, modify `routes.go` — `GET /receipts/:token`, `POST /receipts/:token/resend` (→ 202 always unless the token is unknown → 404).
- Wire both in `cmd/marketplace-api`; regenerate `route-manifest.json`; bump `ExpectedSchemaVersion` to 146.

**Interfaces:**
- Produces (storefront JSON): `{order_number, placed_at, currency_code, lines:[{title, option_summary?, image_url?, quantity, unit_price, line_total}], subtotal, discount_total, tax_total, grand_total, tax_label, fresh: bool, payment_method?, payment_method_label?, receipt_email_masked?}` — the three optional contact fields only when `fresh`.
- `tax_label`: "GST included" when the store's country is AU; otherwise "Tax".

- [ ] **Step 1: Failing tests** (integration unless noted): `TestReceipt_IssueAndLoad` (lines with photo/summary, totals, `fresh:true` with payment method and masked email); `TestReceipt_StaleHidesContact` (backdate `created_at` 8 days → `fresh:false`, no payment/email fields, items still there); `TestReceipt_UnknownToken404`; `TestReceipt_OtherStore404`; `TestReceiptResend_RateLimitedAndSilent` (first resend mails once and adds a link; a second within 10 minutes mails nothing; both 202; body never contains the address); `TestReceiptLink_CashierOwnSalesOnly` (unit, fake service); `TestReceiptSend_SavesEmailAndMails` (email stored, mailer got the receipt URL); orderdoc unit test `in-store receipt has no attachment and links the receipt page`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. Token: 32 bytes `crypto/rand`, base64url without padding; store `hex(sha256(token))`. Resend: if a link for the order was created in the last 10 minutes, do nothing.
- [ ] **Step 4:** `go test -tags integration -p 1 ./internal/instore/ ./internal/handlers/admin/ ./internal/handlers/storefront/ ./internal/orderdoc/ ./internal/routemanifest/ .` → PASS.
- [ ] **Step 5:** Commit `feat(api): in-store receipt links, receipt read and resend`.

### Task 2: Storefront — the receipt page

**Files:**
- Create: `apps/storefront/app/r/[token]/page.tsx`, `apps/storefront/app/r/[token]/actions.ts` (`requestNewReceiptLink(token)`), `apps/storefront/lib/api/receipt-api.ts` (`fetchReceipt(slug, token)`, `resendReceipt(slug, token)`), `apps/storefront/components/receipt/ReceiptView.tsx`
- Test: `apps/storefront/components/receipt/ReceiptView.test.tsx`, `apps/storefront/lib/api/receipt-api.test.ts`

- [ ] **Step 1: Failing tests**: renders store name, order number, "In-store purchase" with date and time, each line with photo (placeholder when none), summary, qty, price; subtotal, discount (only when > 0), total and "GST included A$x"; a fresh receipt shows "Paid by Cash" and the masked email; a stale one shows "Send a new link" instead and no email; "Shop again" links to `/`; fetch maps 404 → null.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. Page: `noindex`, no cart/nav chrome beyond the store name and logo; `@media print` hides the buttons. "Send a new link" → server action → "If there's an email on this order, we've sent a new link to it."
- [ ] **Step 4:** storefront vitest + tsc + lint.
- [ ] **Step 5:** Commit `feat(storefront): in-store receipt page`.

### Task 3: Admin — receipt step after a sale, and resend from the order page

**Files:**
- Modify: `apps/admin/lib/api/instore-api.ts` (`sendInStoreReceipt(storeId, orderId, {email?, send}, session): Promise<MutationResult<{url: string}>>`), `apps/admin/app/(admin)/sell/actions.ts` (`receiptAction(orderId, {email?, send})`)
- Modify: `apps/admin/components/instore/SaleDone.tsx` (email field + "Email receipt", "Print receipt" opens the link in a new tab), `SaleScreen.tsx` (passes order id)
- Create: `apps/admin/components/orders/InStoreReceiptActions.tsx` on the order page for in-store orders ("Email receipt", "Open receipt")
- Test: `SaleDone.test.tsx`, `InStoreReceiptActions.test.tsx`, api tests

- [ ] **Step 1: Failing tests**: Email receipt sends `{email, send:true}` and confirms "Receipt sent to …"; an empty or invalid address keeps the button disabled; Print calls `{send:false}` and opens the returned URL; a failure shows the server message; order page offers the actions only for `channel === "in_store"` and pre-fills nothing (address never shown in full beyond what the order already shows).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. `SaleScreen` keeps the recorded order id in `done`.
- [ ] **Step 4:** Full admin suite, tsc, lint.
- [ ] **Step 5:** Commit `feat(admin): email or print an in-store receipt, and resend it`.
