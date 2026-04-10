---
title: "Order Management"
category: "Operations"
order: 7
---

## Order Lifecycle

Every order in Mark8ly follows a clear lifecycle: **pending**, **confirmed**, **fulfilled**, and optionally **cancelled**. When a customer completes checkout, the order is created with pending status. Once payment is confirmed by your gateway, the status moves to confirmed automatically.

You manage fulfillment manually from the order detail page. When you pack and ship items, mark the order as fulfilled and add tracking information. For orders with multiple items shipping separately, use partial fulfillment to track each shipment independently.

## Viewing Orders

The Orders section shows all orders across your store with filters for status, payment status, and date range. Each row displays the order number, customer email, total amount, status, and how long ago it was placed.

Click any order to see its full detail: line items with quantities and prices, shipping and billing addresses, payment information, fulfillment history, and any notes. The order detail page is your primary workspace for managing individual orders.

## Processing Payments

Payment status tracks separately from order status. An order can be confirmed with payment still pending if you accept manual payment methods. The payment statuses are: **pending**, **authorized**, **paid**, **failed**, **refunded**, and **partially refunded**.

For card payments through Stripe or Razorpay, the payment flow is automatic. The gateway authorizes the charge during checkout and captures it when the order is confirmed. Failed payments result in an order that needs attention, either a retry from the customer or manual cancellation by you.

## Refunds and Cancellations

Issue refunds from the order detail page by clicking the refund action. You can refund the full amount or a partial amount. The refund is processed through the original payment gateway and typically appears on the customer's statement within five to ten business days.

Cancelling an order changes its status and optionally restores inventory. If payment was already captured, you will need to issue a refund separately. Always communicate with the customer about cancellations and expected refund timelines.

## Order Notifications

Mark8ly sends automatic email notifications at key points in the order lifecycle: order confirmation, shipping confirmation with tracking, and refund confirmation. These emails use your store's branding and can be previewed under notification settings.

Customers appreciate timely communication. If there is a delay in fulfillment, consider reaching out proactively rather than waiting for the customer to ask. Building trust through transparent communication leads to repeat business.
