---
title: "Order Management"
category: "Operations"
order: 7
---

Every order in Mark8ly has its own detail page where you'll handle payment, fulfillment, refunds, and customer communication. This article walks through the full lifecycle.

## Order lifecycle

| Status | What it means |
| --- | --- |
| **pending** | Checkout completed, waiting for payment confirmation. |
| **confirmed** | Payment captured (or accepted manually). Ready to fulfill. |
| **fulfilled** | All items shipped. Tracking has been added. |
| **cancelled** | Order voided. Inventory optionally restored. |

Most orders progress automatically from **pending** → **confirmed** as soon as the gateway captures the payment. Fulfillment is always manual — you control when items leave your warehouse.

## Viewing orders

The **Orders** view lists every order across your store with filters for status, payment status, and date range. Each row shows:

- Order number
- Customer email
- Total amount
- Order and payment status
- Time since the order was placed

Click any order to open its detail page — your primary workspace for managing that order. You'll find line items, addresses, payment information, fulfillment history, and notes all in one place.

## Processing payments

Payment status is tracked separately from order status, because an order can be confirmed before payment is captured (for manual payment methods). The states are:

- **pending** — waiting for a charge.
- **authorized** — funds reserved, not yet captured.
- **paid** — funds captured and settled.
- **failed** — gateway declined.
- **refunded** / **partially refunded** — money returned to the customer.

For Stripe and Razorpay, the flow is fully automatic: the gateway authorizes during checkout and captures when the order is confirmed. Failed payments produce an order that needs your attention — either a retry from the customer or a manual cancellation.

## Refunds and cancellations

Refunds are issued from the order detail page. You can refund:

- The **full amount**, or
- A **partial amount** (e.g. one item out of several).

Refunds process through the original gateway and typically appear on the customer's statement within 5–10 business days.

**Cancelling** an order changes its status and optionally restores inventory. If payment was already captured, you'll need to issue a refund separately — cancellation does not automatically return funds.

> Always communicate with the customer about cancellations and expected refund timelines. Silence here is the most common cause of support escalations.

## Partial fulfillment

For orders shipping in multiple packages — for example, when one item is on backorder — use **partial fulfillment**. Each fulfillment gets its own tracking number and triggers a separate shipping email so customers know exactly what's on the way.

## Notifications

Mark8ly sends automatic emails at key points in the lifecycle:

- Order confirmation
- Shipping confirmation (with tracking)
- Refund confirmation

These emails use your store's branding and can be previewed under notification settings.

> If there's a delay in fulfillment, reach out proactively rather than waiting for the customer to ask. Transparent communication is the cheapest customer-retention tool you have.
