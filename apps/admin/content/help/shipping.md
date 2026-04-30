---
title: "Configuring Shipping"
category: "Store Setup"
order: 4
---

Shipping in Mark8ly is built around **zones** (geographic regions) and **rates** (what you charge customers in each zone). Configure both under **Settings → Shipping**.

## Shipping zones

A zone is a group of countries or regions that share the same shipping options. A typical setup is:

- **Domestic** — your home country.
- **International** — everywhere else, or split into specific regions (EU, North America, Asia-Pacific) if you want different rates.

Each zone holds one or more rates. You can name rates anything that makes sense to your customers — `Standard Shipping`, `Express Delivery`, `Economy International`, and so on.

## Rate types

Mark8ly supports three rate structures, and you can mix them within a single zone.

| Type | When to use |
| --- | --- |
| **Flat rate** | Simplest setup — one fixed amount regardless of cart contents. |
| **Weight-based** | Charge based on the total weight of items in the cart, using tiers you define. |
| **Free shipping** | Always free, or unlocked when the cart hits a threshold (e.g. orders over $50). |

A common combination: **free shipping over $50** plus a **flat rate for smaller orders**, both in the same domestic zone.

## Product weights

For weight-based shipping to work, each variant needs a `weight` value. Enter weights in **grams**.

> If a product has no weight, Mark8ly excludes it from the weight calculation — which can mean undercharging customers. Audit your catalog for missing weights before enabling weight-based rates.

Accurate weights also help during fulfillment: the expected weight is a quick sanity check that the right items are in the box.

## Fulfillment workflow

When an order is placed, it starts with a fulfillment status of **unfulfilled**. As you pack and ship, update the order to reflect progress.

1. Pack the items.
2. Click **Fulfill** on the order detail page.
3. Add a tracking number and select the carrier.
4. Mark8ly emails the customer a tracking link automatically.

Mark8ly supports **partial fulfillment** for orders where some items ship before others — useful for backordered SKUs or split-warehouse shipping.

## International shipping

Selling internationally adds customs, duties, and longer delivery times to the equation.

- Communicate expected delivery windows on your storefront and in confirmation emails.
- Decide upfront whether to offer **delivered-duty-paid** pricing or let customers handle import charges.
- Document your policy clearly on a `Shipping` page.

> Mark8ly doesn't currently calculate duties and taxes for international shipments. Factor these costs into your pricing strategy or document them on your shipping policy page.
