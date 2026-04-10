---
title: "Troubleshooting Guide"
category: "Troubleshooting"
order: 14
---

## Common Issues

Most issues merchants encounter fall into a few common categories: payment gateway errors, storefront display problems, and inventory discrepancies. This guide covers the most frequent problems and their solutions.

Before diving into specific issues, try the basics: clear your browser cache, try a different browser, and check that your internet connection is stable. Many apparent platform issues are actually local browser or network problems.

## Payment Gateway Errors

If customers report that checkout is failing, first verify your payment gateway credentials under **Settings > Payments**. Expired or rotated API keys are the most common cause of checkout failures. Test the gateway using your provider's test mode to isolate whether the issue is with Mark8ly or the gateway.

If the gateway credentials are correct but payments still fail, check your payment provider's dashboard for error details. Common issues include insufficient funds, card declines due to fraud detection, and regional restrictions on certain card types.

## Products Not Showing on Storefront

If a product is not visible on your storefront, check its status in the admin. Only products with **Active** status appear publicly. Draft and archived products are hidden from customers. Also verify that the product has at least one variant with a price greater than zero.

If the product is active but still not showing, check whether it belongs to a published category. Products without a category may not appear in your storefront's navigation, though they are still accessible via direct URL.

## Inventory Discrepancies

Inventory counts can become inaccurate if manual adjustments are not recorded or if multiple team members update stock simultaneously. Perform periodic physical inventory counts and reconcile them with Mark8ly's records.

If inventory shows zero but you have stock on hand, update the quantity on the variant detail page. Consider enabling low-stock alerts to catch discrepancies before they affect customers. The dashboard's low-stock alerts section highlights variants that have fallen below their configured threshold.

## Email Notifications Not Sending

Mark8ly uses SendGrid to deliver transactional emails like order confirmations and shipping notifications. If customers are not receiving emails, ask them to check their spam folder first. Some email providers aggressively filter automated messages.

If emails are consistently not delivering, check the notification settings to ensure they are enabled for each event type. Contact support if you suspect a deliverability issue at the infrastructure level.

## Slow Page Loading

If your storefront or admin dashboard loads slowly, the issue is usually related to large unoptimized images or a slow network connection. Compress product images before uploading. Mark8ly recommends images under two megabytes and suggests WebP format for the best compression-to-quality ratio.

For admin dashboard performance, check your browser's developer tools network tab to identify slow requests. If the marketplace API is consistently slow, it may indicate a temporary service issue. Check the Mark8ly status page or contact support for updates.

## Getting Further Help

If this guide does not resolve your issue, open a support ticket under **Support > Tickets** with a detailed description of the problem including steps to reproduce it, the browser and device you are using, and any error messages you see. Screenshots are helpful. Our support team typically responds within one business day.
