---
title: "Troubleshooting Guide"
category: "Troubleshooting"
order: 14
---

Most issues fall into a few familiar categories: payment errors, storefront display problems, inventory discrepancies, missing emails, and slow loads. This guide covers the most common ones and how to fix them.

> Before diving in: **clear your browser cache, try a different browser, and check your internet connection**. A surprising number of "platform issues" are local browser or network problems.

## Payment gateway errors

**Symptom:** customers report that checkout is failing.

What to check, in order:

1. **API credentials.** Open **Settings → Payments** and confirm your keys haven't been rotated or expired. This is by far the most common cause.
2. **Test mode.** Run a test charge using your gateway's test credentials to isolate whether the problem is in Mark8ly or in the gateway.
3. **Gateway dashboard.** Open Stripe or Razorpay's dashboard and look at the failed payment for error details.

Common gateway errors:

- **Insufficient funds** — customer issue, retry with a different card.
- **Card declined by fraud detection** — customer should contact their bank.
- **Regional restrictions** — some card types don't work for certain countries.

## Products not showing on the storefront

If a product isn't visible to customers:

1. Check the **status** in the admin. Only **Active** products appear publicly. Drafts and archived products are hidden.
2. Confirm at least one variant has a **price greater than zero**.
3. Check that the product belongs to a **published category**. Products without a category are still accessible by direct URL but won't appear in storefront navigation.

## Inventory discrepancies

If counts don't match what's on the shelf:

- Update the variant's quantity directly on the product detail page.
- Enable **low-stock alerts** on the dashboard so you catch discrepancies before they affect customers.
- Perform periodic physical counts and reconcile with Mark8ly's records.

> Inventory drift usually comes from manual adjustments that didn't get recorded, or two team members editing stock at the same time. Funnel inventory updates through one person or one workflow whenever you can.

## Email notifications not sending

Mark8ly uses SendGrid for transactional emails (order confirmations, shipping, refunds).

If a customer reports they didn't get an email:

1. Ask them to check their **spam / promotions folder** first.
2. Confirm the email address on the order is correct.
3. Open **notification settings** and check that the relevant event types are enabled.
4. If emails are consistently undeliverable, contact support — there may be an infrastructure-level deliverability issue.

## Slow page loading

If your storefront or admin loads slowly:

- **Compress product images.** Aim for under **2 MB** per image, and prefer **WebP** for the best compression-to-quality ratio.
- **Check your network.** Open your browser's developer-tools network tab and look for slow requests.
- **Check the Mark8ly status page.** A persistently slow API may be a temporary platform issue.

> Image weight is the single biggest cause of slow storefronts. A typical 5 MB camera JPEG can be reduced to under 300 KB at the same visual quality.

## Getting further help

If this guide doesn't resolve your issue, open a support ticket under **Support → Tickets** with:

- A detailed description of the problem
- Steps to reproduce it
- The browser and device you're using
- Any error messages you see (screenshots are gold)

Our support team typically responds within **one business day**.
