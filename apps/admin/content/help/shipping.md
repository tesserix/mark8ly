---
title: "Configuring Shipping"
category: "Store Setup"
order: 4
---

## Shipping Zones

Shipping zones let you define different rates for different geographic regions. Navigate to **Settings > Shipping** to create and manage your zones. A typical setup might include a domestic zone for your home country and an international zone for everywhere else.

Each zone contains one or more shipping rates. You can name these rates anything that makes sense to your customers, such as "Standard Shipping," "Express Delivery," or "Economy International."

## Rate Types

Mark8ly supports three rate structures. **Flat rate** charges a fixed amount regardless of cart contents and is the simplest to set up. **Weight-based** rates calculate shipping cost based on the total weight of items in the cart, using tiers you define. **Free shipping** can be offered unconditionally or as a threshold, such as free shipping on orders over a certain amount.

You can combine rate types within a single zone. For example, offer free shipping on orders above fifty dollars and a flat rate for smaller orders in the same domestic zone.

## Product Weights

For weight-based shipping to work accurately, each product variant needs a weight value. Enter weights in grams when creating or editing products. If a product does not have a weight, Mark8ly excludes it from weight calculations, which could result in undercharging for shipping.

Consistent weight data also helps with fulfillment accuracy. When packing orders, the expected weight serves as a quick sanity check that the right items are in the box.

## Fulfillment Workflow

When an order is placed, it starts with a fulfillment status of "unfulfilled." As you pack and ship items, update the fulfillment status to reflect progress. Mark8ly supports partial fulfillment for orders where some items ship before others.

Adding a tracking number to a fulfillment triggers a notification email to the customer with their tracking link. Customers appreciate proactive shipping updates, and providing tracking reduces support inquiries about order status.

## International Shipping Considerations

Selling internationally introduces customs, duties, and longer delivery times. Clearly communicate expected delivery windows on your storefront and in confirmation emails. Consider whether you want to offer delivered-duty-paid pricing or let customers handle import charges.

Mark8ly does not currently calculate duties and taxes for international shipments, so factor these costs into your pricing strategy or document them clearly in your shipping policy page.
