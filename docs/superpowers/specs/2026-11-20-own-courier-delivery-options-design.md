# Own courier: flat-rate delivery options

Date: 2026-11-20 · Status: design, awaiting approval

## Why

Sugarcoat (Souri, India) sells laptop skins for ₹500–600. Delhivery's live quote is ₹250–300 per parcel, about half the product price, and buyers balk at it. Souri already ships with a local courier (Anjani) that charges ₹50–100: staff pack the parcel, drop it at the courier's outlet, and get a tracking number to pass on.

Today Mark8ly can only price shipping from a live carrier quote (Delhivery in India; ShipEngine and NinjaVan elsewhere). A seller cannot set their own price or ship with their own courier. This design lets them.

## What success looks like

- Souri switches his store to Own delivery, sets "Standard ₹100" (and optionally "Express ₹200"), and buyers pay ₹100 at checkout.
- When staff drop a parcel off, Souri marks the order shipped, picks Anjani, pastes the tracking number, and the buyer gets the usual "on its way" email with a link that opens Anjani's tracking page for that parcel.
- Works the same in every country Mark8ly supports, in the store's currency.

## Goals

1. A seller defines their own delivery options: name, flat price per order, optional delivery estimate, on/off.
2. The seller chooses **one** way to ship per store: Mark8ly's integrated carriers (Delhivery / ShipEngine / NinjaVan, as today) **or** Own delivery. Never both at once (user decision, 20 Nov: simpler to set up and for buyers to understand).
3. A seller saves their couriers once: name plus an optional tracking-page link template.
4. A seller marks an order shipped with a saved courier and a tracking number; the buyer is emailed and sees a working tracking link.

## Non-goals

- Booking pickups, printing labels, or pulling tracking updates from own couriers. Most have no API; the seller does this by hand, as today.
- International rates. The storefront only accepts addresses in the store's own country (existing behaviour), so every order is domestic.
- Prices by weight, zone, postcode or item count. One flat price per order per option. Sellers who need "same city ₹50 / elsewhere ₹150" create two options and the buyer picks (the seller trusts the buyer's choice; see Open risk).
- Any change to how carrier (Delhivery/ShipEngine/NinjaVan) shipping works.

## Anti-goals (would make this a failure even if it works)

- A buyer can pay a price other than one the seller set (forged or stale option).
- A store with only own options can't take an order, or a store with carriers loses them.
- A buyer gets a broken or unsafe tracking link.
- Setting up a simple ₹100 flat rate takes more than a minute.

## The seller's experience

### Settings → Shipping

At the top of the page, one choice: **How do you ship?** — *Mark8ly couriers* (today's carrier cards) or *Own delivery*. New and existing stores default to Mark8ly couriers, so nothing changes until a seller switches. Switching to Own delivery hides carrier rates at checkout but keeps the carrier configuration (keys, warehouse, settings) so switching back is one click; switching back hides own options and keeps them too.

When Own delivery is selected, the **Own delivery** card shows:

- **Delivery options**: a list, up to 5. Each row: name (e.g. "Standard", "Express", "Local pickup"), price (store currency, ≥ 0, so ₹0 local pickup works), optional estimate text shown to buyers (e.g. "3–6 days"), on/off switch. Reorderable; order = checkout order. The first time a seller opens the card, one "Standard" row is pre-filled with price empty.
- **Couriers**: a list. Each: name (e.g. "Anjani", "Australia Post"), optional tracking link containing `{number}`, e.g. `https://example-courier.in/track?awb={number}`. A **Test link** button opens the link with a sample number so the seller can check it.
- A **free shipping over** amount for own options (see Checkout).

Validation: name 1–40 chars; price 0–99,999 with the store currency's decimals; estimate ≤ 40 chars; tracking link must be `https://`, must contain `{number}` exactly once, ≤ 500 chars; courier name 1–40 chars.

### Which options checkout shows

- Store set to **Own delivery**: only its enabled own options. If none are enabled, the existing no-shipping fallback shows (and the settings page warns the seller).
- Store set to **Mark8ly couriers**: carrier rates exactly as today. Own options are never shown.

### Shipping an order

On the admin order page, an order paid with an own option shows **Mark as shipped** instead of the carrier label form:

1. Pick a saved courier (dropdown; defaults to the last one used).
2. Paste the tracking number (optional if the courier has no tracking, but encouraged).
3. Save → a shipment is recorded as shipped (status `in_transit`), the existing one-time "shipped" email goes out, and the buyer's order page shows the courier and tracking link.

The seller can edit the courier/number afterwards (typo fix) without re-sending the email. Marking **delivered** stays optional, using the existing status button.

An order paid with a carrier rate keeps today's label flow, even if the store has since switched to Own delivery. Each order follows the method it was paid with (`orders.shipping_carrier`), so in-flight orders finish the way they started. **Mark as shipped** is offered only on own-delivery orders.

*Design calls (from review):*
- **One shipment per order.** Mark as shipped covers the whole order, including orders whose stock comes from several warehouses: it links all of the order's stock allocations to the new shipment and sets the order's fulfilment status to fulfilled, the same end state the per-warehouse label path reaches. The seller does not also need to press Fulfil.
- **Only once.** Mark as shipped is offered only while the order has no active (non-cancelled) shipment. Once marked shipped, mistakes are fixed with the edit-tracking action, never a second shipment, so the buyer gets exactly one "on its way" email per order.

## The buyer's experience

- Checkout lists own options like carrier rates: name, estimate if set, price. Free shipping applies when the order subtotal reaches the store's threshold (shown as "Free" for own options).
- Shipped email: "Shipped with Anjani · Tracking AWB123" with a **Track your parcel** link when the courier has a tracking link.
- Order page (`/orders/[id]`, `/account/orders/[id]`): courier and tracking number, the number linked when a link exists.

## How it fits the current system (decisions)

These are calls made in this design, based on how the code works today.

1. **Option identity.** Carrier rates are identified only by `service` ("standard"/"express") with no id. Own options get a stable id, and the checkout `shipping_service` value for them is `own:<option uuid>`. `orders.shipping_service` (VARCHAR 40) holds it; `orders.shipping_carrier` = `own`.
2. **Server re-check.** `calculateShipping` (`checkout_ext.go`) gains an `own:` branch that runs before any carrier lookup: load the option for this store, require it enabled, price = its price (then free-shipping threshold, then free-shipping coupon, as today). An unknown, disabled or malformed `own:` value is a validation error (anti-goal 1). *Design call:* the existing silent "fall back to the first rate" for **carrier** rates (`calculateShipping` and `selectShippingPrice`) is left unchanged in this release — changing it alters live carrier checkout, which is a non-goal; it is a separate follow-up.
3. **Snapshot on the order.** Add `orders.shipping_option_name` and `orders.shipping_option_estimate` (nullable) so the order keeps what the buyer chose even if the seller later renames or deletes the option.
4. **Shipping mode is the switch.** A store-level `shipping_mode` (`carrier` default | `own`) decides which branch runs. In `own` mode the rates endpoint returns only own options and checkout accepts only `own:` values; a carrier service is a validation error, and the carrier is never consulted (so "no active carrier config" can't occur). In `carrier` mode an `own:` value is a validation error and everything else is unchanged.
5. **Rates endpoint.** In `own` mode, `POST /shipping-rates` returns the enabled own options as rate rows (`service: own:<id>`, `carrier: own`, `name`, free-text `estimate`, `price`, `currency_code`); in `carrier` mode it is unchanged. Response rows gain an optional `name` and free-text `estimate` field; existing fields are unchanged so old clients keep working.
6. **Free shipping threshold.** Today `free_shipping_min` lives on each carrier config row. Own delivery uses its own store-level threshold stored with the own-delivery settings. Only the active mode's threshold ever applies.
7. **Manual shipment.** (See the design calls under "Shipping an order": one per order, fulfils the order, links allocations, refused if an active shipment exists.) `shipments` requires `carrier VARCHAR(20) NOT NULL` and `ship_from`/`ship_to` JSONB NOT NULL. A manual shipment sets `carrier = 'own'`, a new `courier_id` (nullable FK) and `courier_name` snapshot, `tracking_number`, a new `tracking_url` (resolved at save time from the template), `ship_to` = the order's address, `ship_from` = the store's default warehouse address or `{}` if none, `status = in_transit`, `shipped_at = now()`. It then reuses the existing one-time shipped-email dispatch.
8. **Tracking URL everywhere.** Add `tracking_url` to `shipments`, to the storefront order-detail projection, to `orderdoc.ShipmentInfo`, to the shipped email templates (HTML + text), and render it as a link in `OrderTimeline`. Carrier shipments leave it empty (unchanged behaviour). The URL is built server-side by substituting the URL-escaped tracking number into the template; only `https://` results are stored.
9. **Storage.** New tables, migration 000150+:
   - `store_delivery_options` (id, tenant_id, store_id, name, price NUMERIC(12,2), estimate, enabled, position, created/updated) — max 5 per store enforced in the service.
   - `store_couriers` (id, tenant_id, store_id, name, tracking_url_template, created/updated).
   - Store-level `shipping_mode` and own-delivery free-shipping threshold (columns on an existing store settings row or a small `store_delivery_settings` row; builder's choice).
   - Both new tables are tenant-scoped and must be added to `tenantpurge`'s plan (the purge-coverage test enforces this).
10. **Admin API.** Under the existing admin store routes: get/set shipping mode, list/replace delivery options (one PUT of the whole list keeps ordering simple), list/create/update/delete couriers, `POST …/orders/:id/shipments/manual` (courier_id, tracking_number), `PATCH …/shipments/:id/tracking` for corrections. All behind the same store-scoped auth and capability as the existing shipping settings and shipment routes.

## Constraints

- Prices and display use the store currency; no FX.
- Respect existing guards: log-shipping smoke guard (no `fmt.Print*` in service code), route manifest (new admin routes are frontend-facing and must be in the manifest), tenant purge coverage.
- Mobile admin and the (unmounted) mobile storefront are out of scope; the web admin and storefront carry the feature. The API should not prevent mobile from adopting it later.

## Open risk

- A buyer could pick a cheaper option that doesn't match their location (e.g. "Same city ₹50" from another state). We accept this: sellers who split by location see the address on the order and can contact the buyer. Not worth location rules in v1.

## Left to the builder

- Exact UI layout and copy within the existing settings and order-page design system.
- Whether the own-delivery free-shipping threshold is a column or a small settings row.
- Test strategy beyond: server re-check rejects unknown/disabled/forged options; free shipping applies to own options; manual shipment sends the email once and renders the link; purge covers the new tables.
