# In-store sales (POS-lite)

> Status: design, awaiting the user's approval. Date: 2026-11-14.

## Why

Sellers who already run a mark8ly shop also sell in person: small shop
counters, weekend market stalls, and stores like sugarcoat (laptop skins,
India) with a physical counter. Today an in-person sale never reaches
mark8ly, so stock drifts and online buyers can order something that sold at
the market that morning.

The model is what the user saw at a small shop on 14 Nov (Shopify POS): an
iPad to add products and build the bill, the amount keyed into a separate
EFTPOS terminal by hand, "paid" confirmed in the app, and a receipt link
with product photos sent to the customer.

**The point for mark8ly:** one catalogue and one stock count across online
and in-person, so selling both ways stops causing oversells.

## Success

- Ringing up a sale takes about as long as keying the amount into the
  EFTPOS terminal.
- Stock, orders, the dashboard and reports include in-store sales with no
  extra step.
- The customer leaves with a receipt that looks like the shop's own.

## Goals

1. An **In-store sale** screen in admin, built for a tablet.
2. **Record payment taken outside mark8ly**: card on the seller's own
   EFTPOS terminal, cash (with change worked out), or bank transfer.
3. In-store sales are **normal orders**, tagged in-store, that decrement
   stock exactly like online orders.
4. A **receipt page** with product photos, sent by **email** or **printed**.
5. A **Cashier** role that can only sell.
6. Available on **every plan, Trial included**.

## Non-goals (now)

- **Offline mode.** A connection is required (see Constraints).
- **Texting receipts.** Comes later; the receipt page is built so SMS only
  adds a send action. Needs a provider and a cost decision.
- **Integrated card readers** (Stripe Terminal, Tyro, Square). Sellers
  already own EFTPOS terminals.
- **Hardware:** receipt printers, cash drawers, barcode scanners.
- **Staff operations:** shifts, clock-in, cash-up / end-of-day per cashier,
  PIN switching on a shared device.
- **In-store refunds by cashiers.** Refunds stay with owners and managers
  through the existing refund flow.

## Anti-goals (would make this a failure even if it works)

- The app says **"paid" before the server has recorded the order.** Stock
  and money must always agree.
- A **required personalisation field blocks a counter sale.** The seller can
  ask the customer; blocking only slows them down.
- A **cashier can change prices**, see other orders or customers, or reach
  settings.
- The receipt link **exposes the customer's email or phone** to anyone who
  has the link after it has been shared around.

## Decisions (made with the user)

| Topic | Decision |
|---|---|
| Who it's for | Small shops, weekend markets, and sellers with a physical store (sugarcoat) |
| Connection | Required. The cart survives a dropped connection; the sale completes only when the server confirms. No offline mode |
| Personalisation | Same fields as online, shown on the sale screen but **optional** in person, plus **one free-text note per sale** |
| Receipts | **Email and print now**, **text later**. A link to a receipt page with photos, like the Shopify example |
| Staff | Staff can sell too, but **only sell**. Each person uses their own account, on their own device or taking turns on a shared iPad |
| Plans | All plans, including Trial |
| Custom items | Yes: a seller can ring up an item that is not in the catalogue (name, price, note). Price deals ("two for $30") are discounts, not price edits |

## The flow

1. Seller opens **In-store sale** (tablet layout). A grid of product photos
   with search and category tabs. Only active products appear; archived and
   draft do not.
2. Tap a product. Pick a variant if it has options. Personalisation fields
   appear, optional; the seller can skip them.
   **Custom item** adds a line that is not in the catalogue: name, price,
   optional note. No stock, no photo (the receipt shows a placeholder).
3. The cart (side panel on a tablet, bottom sheet on a phone) shows items,
   quantities, line totals and the total. The seller can add the note and,
   if permitted, a discount.
4. **Charge A$X** → choose how the customer paid: *Card (EFTPOS)*, *Cash*
   or *Bank transfer*. Cash takes the amount tendered and shows the change.
5. **Paid** → the server creates the order as paid and decrements stock in
   one transaction. Only after the server confirms does the screen show
   success. If stock is short, the server refuses and the screen says which
   line, so the seller can adjust; nothing is recorded.
6. Receipt screen: **Email receipt** (type an address), **Print**, or
   **No receipt**. Then **New sale**.

**Dropped connection:** the cart is kept on the device (local storage),
the screen shows "No connection, sale not recorded yet", and **Paid** is
retried with the same idempotency key, so a retry can never create two
orders. The seller must not treat the sale as done until it confirms.

## In-store orders

- Created through the same order service as online orders, so totals,
  tax, order numbers and stock behave identically.
- Tagged with a **channel** (`in_store` vs `online`) and the **payment
  method** (`card_external`, `cash`, `bank_transfer`). Payment status is
  `paid` at creation; fulfilment is `fulfilled` at creation (handed over at
  the counter), except where the seller marks it "to be made / collected"
  (sugarcoat cuts skins to order). *Builder's call how to express that
  toggle; default fulfilled.*
- Record **who rang it up** (the staff user id).
- **Custom item lines** are order items with no product or variant id
  (both are already nullable, "deliberately NOT FKs") and are excluded
  from stock. A price deal on a catalogue item is a line or cart discount,
  so the catalogue price and per-product reports stay true.
- Customer: none, or matched/created by the email typed on the receipt
  screen. `orders.customer_email` is `NOT NULL` today; in-store orders
  without a receipt email need that relaxed or a defined placeholder.
  *Builder's call; must not send mail to a placeholder.*
- Skip online-only steps: shipping, checkout confirmation emails, abandoned
  cart, payment-provider webhooks.
- Visible in the Orders list with an **In-store** badge and a channel
  filter; counted in the dashboard and reports.

## Receipt page

Modelled on the Shopify example the user received (order #61123, Micro
Aquatic Shop):

- Served on the **store's own address**: `<store>.mark8ly.com/r/<token>`,
  or the connected custom domain. The token is long and random.
- Store logo and theme; order number; "In-store purchase" with date and
  time.
- Each line: product photo, title, variant / personalisation summary,
  quantity, price. Subtotal, discount, total in the store currency, and
  **GST included**.
- **Buy again** / link to the shop, to turn a counter customer into an
  online one.
- **Privacy:** items and totals stay visible on the link indefinitely.
  Contact and payment details are shown only on a fresh link and hide after
  a window; afterwards the page offers "Send a new link" to the email on
  the order (never revealing it). *Window length is the builder's call.*
- **Print:** a print stylesheet so the iPad's normal print (AirPrint) gives
  a clean receipt. No printer integration.
- **Email:** short message with the order number, the store name and the
  link, through the existing order-document mailer and templates.
- The receipt can be resent from the order page in admin.

### Tax invoices (Australia)

A receipt from a GST-registered seller must show the GST included. For a
sale of **A$82.50 or more (GST-inclusive)** it must also qualify as a tax
invoice, which needs the seller's **ABN** and identity. mark8ly has no ABN
field today. Add an optional business number to store settings and print it
on receipts when set; when the store is GST-registered, label the receipt
"Tax invoice". Non-AU stores (sugarcoat, India) show their own tax
treatment from the existing tax settings, without the AU wording.

## Cashier role

- A new **store** relation `cashier` in `infra/openfga/model.fga`, with a
  new permission `can_sell_in_store`. Owner, manager and staff also get
  `can_sell_in_store`.
- A cashier **can**: open In-store sale, read active products and stock,
  create in-store orders, send or print receipts for them, and list the
  sales they rang up themselves (today, for checking a receipt was sent).
- A cashier **cannot**: edit products or prices, see other orders or any
  customer list, see reports or settings, refund, invite members.
- **Discounts and custom items:** off for cashiers unless the owner enables
  "Cashiers can give discounts and custom prices" for the store. One switch
  covers both, since a typed price is the easiest way to undercharge.
- Invited by email like other members; own login. In admin, a cashier sees
  only the sale screen. The sale screen has a **Switch person** action that
  signs out to the login screen for shared iPads.
- Note: today store `staff` can manage the catalogue, so `staff` is not a
  substitute.

## Plans

On every plan including Trial. If it gets a plangate feature row at all, it
is enabled everywhere; cashier seats count toward any existing member
limits like other roles.

## Constraints

- **Connection required.** Market EFTPOS terminals already run over 4G or
  the phone's hotspot; the tablet can share it. Offline sync was rejected:
  conflicts on the last unit, and it defeats the oversell goal.
- **Stock is authoritative on the server.** Use the existing availability
  model (`internal/stockhold`: available = quantity minus live holds), so
  an online checkout holding the last unit is respected.
- **Idempotency:** every Paid uses a client-generated key, reusing the
  orders `UNIQUE (store_id, idempotency_key)` guarantee.
- **Client components must not call `MARKETPLACE_API_URL`** (in-cluster);
  use server actions or same-origin routes, as with media actions (#1093).
- **Secrets** per AGENTS.md if any new credential appears (none expected
  until SMS).

## Calls made in review (not discussed; check these)

- **Payment methods** default to Card (EFTPOS), Cash and Bank transfer, plus
  **Other** with a short label (e.g. UPI for sugarcoat in India). Labels
  only; nothing is charged through mark8ly.
- **Refunding an in-store sale** (owner/manager only): record a refund made
  outside mark8ly (back on the terminal or in cash), which restocks and
  marks the order refunded without calling any payment provider.
- **Out-of-stock products** follow the product's existing inventory policy:
  "continue selling" products can be rung up at zero stock, others cannot.
- **A receipt email is not marketing consent.** The address is saved on the
  order for receipts only and is not added to campaigns or segments.

## Left to the builder

- Screen layout details, the product grid's density, and how variants and
  personalisation are picked on the tablet.
- Whether the cart keeps one personalisation answer per line or per unit
  (follow the online cart).
- The receipt token format and the contact-details window.
- Where the channel and payment-method values live (columns vs existing
  `payment_provider`).
- The "made to order / collect later" fulfilment toggle's shape.
- Placeholder vs nullable `customer_email`.

## Later

- Text receipts (provider, per-message cost, who pays).
- Cash-up / end-of-day per device or cashier.
- Integrated card readers.
- Offline mode, if markets prove it necessary.
