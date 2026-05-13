---
title: Customer Support Knowledge Base
category: Customer Support
order: 1
---

This is the canonical knowledge base for the Otto AI support assistant on
every mark8ly storefront. The same content seeds the `kb_mark8ly` pgvector
namespace, so any edit you make here should be mirrored in
`tesserix-k8s/charts/apps/support-platform-kb-seed/values.yaml` under
`content.mark8ly` and pushed — the next CronJob run re-embeds and the SLM
picks it up.

Each entry follows the same shape so the SLM can quote it verbatim:

- **Issue** — what the customer literally says.
- **Cause** — most likely root cause, derived from the code.
- **Solution** — the 1–2 line answer the SLM should give.
- **Escalate** — the trigger that flips `needs_human = true`.

---

## Account & sign-in

### Session expired
- **Issue**: "Your session has expired" / redirect to /login.
- **Cause**: HttpOnly cookie reached its 24h expiry, cookies cleared, or customer is on a sibling store's subdomain.
- **Solution**: Clear cookies and sign back in on the **exact** store subdomain shown in the URL. Cookies are scoped per store.
- **Escalate**: Same-subdomain re-login still fails.

### Email verification link expired
- **Issue**: "This verification link is invalid or has expired."
- **Cause**: Token is single-use and expires after 7 days.
- **Solution**: Click "Request a new verification email" on signup/account page. Check spam.
- **Escalate**: Neither the first email nor the resend arrives within 5 minutes.

### Password reset link expired
- **Issue**: Reset link says "invalid or expired".
- **Cause**: Reset tokens expire 1 hour after issue.
- **Solution**: Request a fresh link from forgot-password and use it within an hour.
- **Escalate**: Customer clicks within minutes and still gets "expired", or no email arrives.

### MFA already enabled
- **Issue**: "MFA is already enabled — disable it first to re-enrol."
- **Cause**: Customer reset their device but never disabled MFA on the old one.
- **Solution**: Account Settings → Security → Disable MFA (needs the existing authenticator code), then re-enrol on the new device.
- **Escalate**: Lost access to the old authenticator and can't complete the disable flow — needs a staff reset.

### MFA invalid code
- **Issue**: "Verification code is incorrect."
- **Cause**: Phone clock skew, code already rotated (every 30s), or wrong account in the authenticator.
- **Solution**: Wait a few seconds for a fresh code; ensure phone time is Auto.
- **Escalate**: Clock correct, right account, still rejected on multiple tries.

---

## Browsing & search

### Product not found
- **Issue**: "I can't find the product I saw yesterday."
- **Cause**: Unpublished, out of stock, or wrong store's subdomain.
- **Solution**: Search by exact name/SKU. Confirm the URL's store slug matches the original. "In stock only" filters can hide unavailable items.
- **Escalate**: Exact SKU and recent purchase, but product is missing.

### Store not found
- **Issue**: "This store doesn't exist" / 404.
- **Cause**: Mistyped subdomain, store closed/suspended, subdomain renamed.
- **Solution**: Verify the URL with the store owner; stores can be temporarily closed.
- **Escalate**: Store admin says it's live with the same subdomain.

---

## Cart & checkout

### Currency mismatch at checkout
- **Issue**: "Item currency does not match store currency."
- **Cause**: Frontend cached an old price in the wrong currency.
- **Solution**: Refresh the page and rebuild cart.
- **Escalate**: Persists after a hard refresh.

### Too many saved addresses
- **Issue**: "Maximum of 5 saved addresses."
- **Cause**: v1 hard cap.
- **Solution**: Account → Saved Addresses → delete one, then add the new one.
- **Escalate**: Never — this is by design.

### Idempotency-key conflict
- **Issue**: "Idempotency key was previously used with a different payload."
- **Cause**: Customer edited the cart and retried with the same key.
- **Solution**: If an order exists already, don't retry. Otherwise start a fresh checkout so a new key is generated.
- **Escalate**: Two orders exist when the customer meant one, or payment charged twice.

### Address validation failed
- **Issue**: "Shipping address is invalid."
- **Cause**: Postal-code format wrong for the country, missing required field, or unknown address.
- **Solution**: Confirm postal-code format, fill every required field, try a slightly different spelling.
- **Escalate**: Address verifiably correct but validation keeps rejecting.

---

## Payment

### Gateway not configured
- **Issue**: "Gateway not configured" / "Payment service unavailable."
- **Cause**: Store hasn't activated Stripe/Razorpay or secrets are missing.
- **Solution**: Contact the store owner — checkout won't work until they finish payment setup.
- **Escalate**: Store says they configured the gateway but the error persists.

### Razorpay signature mismatch
- **Issue**: "Signature mismatch" / "Payment verification failed."
- **Cause**: Stale Razorpay config in browser cache after the store rotated its webhook secret.
- **Solution**: Hard refresh (Cmd-Shift-R) and retry. Customer wasn't debited.
- **Escalate**: Hard refresh doesn't help, or many customers report the same error at once.

---

## Orders & shipping

### Cannot cancel order
- **Issue**: "This order is not in a state where it can be cancelled."
- **Cause**: Order has shipped, or is still pending payment.
- **Solution**: Cancel only works between `paid` and `shipped`. If shipped, file a return after delivery instead.
- **Escalate**: Order is clearly not shipped but cancel is greyed.

### No shipping rates available
- **Issue**: "Unable to calculate shipping rates."
- **Cause**: PIN/postal code outside the store's service area, missing phone (India), or invalid address.
- **Solution**: Confirm PIN format. Fill phone for India. Check the store's shipping FAQ.
- **Escalate**: A known-good PIN suddenly returns no rates.

### Late delivery
- **Issue**: "My order is late, where is it?"
- **Cause**: Carrier delay; tracking can lag the physical location by 24–48h.
- **Solution**: Share the tracking link from the order page. Delivery estimates aren't guarantees; 2–5 day delays are common during peak.
- **Escalate**: Marked "delivered" but never received, OR tracking stuck >7 days.

### Missing or damaged item
- **Issue**: "Item didn't arrive" / "Arrived broken."
- **Cause**: Missing from shipment, transit damage, or wrong item picked.
- **Solution**: Open a return from the order page with photos. The store responds within 5 business days.
- **Escalate**: Return rejected and customer disputes the assessment.

---

## Returns & refunds

### Can't return order
- **Issue**: "This order can't be returned in its current state."
- **Cause**: Returns only fire on shipped/delivered orders.
- **Solution**: If unshipped, use Cancel. If shipped, Return becomes available after delivery.
- **Escalate**: Shipped >50 days ago and customer just noticed.

### Return already open
- **Issue**: "A return request already exists for this order."
- **Cause**: v1 allows one open return per order.
- **Solution**: Wait for the existing return to be resolved before filing another.
- **Escalate**: Customer needs to amend an open return (add items).

### Return rejected
- **Issue**: "Your return was rejected."
- **Cause**: Outside policy (custom/final-sale), window expired, or item appears used.
- **Solution**: Read the rejection reason. Customer can reply with photos for a re-review.
- **Escalate**: Reason is vague, or refund was promised in writing.

### Refund not received
- **Issue**: "I returned the item but no refund yet."
- **Cause**: Pending approval, just approved within 10 business days, or refunded to wallet instead of card.
- **Solution**: Refunds go to the original payment method in 5–10 business days. Bank settlement adds 2–3 more days.
- **Escalate**: Approved >10 business days ago with nothing on the card.

---

## Vendor / multi-tenant

### Wrong store showing orders
- **Issue**: "I placed an order on Store A but it shows on Store B."
- **Cause**: Customer has accounts on both stores and signed in to the wrong one. Cookies are scoped per store.
- **Solution**: Sign out, clear cookies, sign back in on the same subdomain where the order was placed.
- **Escalate**: Single account but orders genuinely appear cross-store.

### Store closed / unreachable
- **Issue**: "This store doesn't exist" / "Store temporarily unavailable."
- **Cause**: Store owner paused/closed it, subscription expired, or subdomain renamed.
- **Solution**: Contact the store owner. The mark8ly platform doesn't force stores open.
- **Escalate**: Store owner says they didn't close it.

---

## Coupons

### Coupon not found
- **Issue**: "Coupon `<CODE>` not found."
- **Cause**: Typo or wrong case — codes are case-sensitive.
- **Solution**: Copy-paste the code from email/ad to avoid typos.
- **Escalate**: Customer has the exact code and store confirms it should be valid.

### Coupon expired
- **Issue**: "Coupon has expired."
- **Cause**: Past the end date or store ended the campaign.
- **Solution**: Ask the store for a replacement code if the original email promised a longer window.
- **Escalate**: Email explicitly says "valid until <future date>" but system says expired.

### Coupon usage-limit reached
- **Issue**: "Coupon has reached its usage limit."
- **Cause**: Max total uses or per-customer cap hit.
- **Solution**: This specific code is no longer redeemable.
- **Escalate**: Never — caps work as intended.

### Coupon minimum-purchase not met
- **Issue**: "Coupon requires a minimum purchase of X."
- **Cause**: Cart subtotal is below the minimum spend.
- **Solution**: Add items until subtotal hits the threshold; discount applies automatically.
- **Escalate**: Never — this is by design.

---

## Gift cards & loyalty

### Gift card not found
- **Issue**: "Gift card not found."
- **Cause**: Wrong number or card revoked.
- **Solution**: Copy the number directly from email/receipt; confirm with the store.
- **Escalate**: Customer has the receipt and store confirms issuance.

### Gift card expired
- **Issue**: "Gift card has expired."
- **Cause**: Past the expiry set when the card was issued.
- **Solution**: Promotional cards may be reissued at store discretion.
- **Escalate**: Expired recently and no expiry-warning email was sent.

### Insufficient gift-card balance
- **Issue**: "Gift card balance is too low."
- **Cause**: Prior redemptions reduced balance below the cart total.
- **Solution**: Check live balance; pay the difference with another method or combine multiple cards.
- **Escalate**: Balance lower than the customer says it should be.

### Loyalty points not credited
- **Issue**: "My loyalty points didn't increase after a purchase."
- **Cause**: Not enrolled, batch credit pending (24–48h), or order was refunded.
- **Solution**: Enrol in Account → Loyalty. Points credit on `confirmed`, not `paid`. Check Points History.
- **Escalate**: Points missing >48h after order confirmed.

### Insufficient loyalty points
- **Issue**: "You don't have enough points to redeem this."
- **Cause**: Below threshold or points expired.
- **Solution**: Show current balance from Account → Loyalty.
- **Escalate**: Never.

---

## Reviews

### Already reviewed
- **Issue**: "You have already reviewed this product."
- **Cause**: One review per customer per product.
- **Solution**: Find the existing review on the product page or My Reviews; edit/delete within the edit window.
- **Escalate**: Customer insists they never reviewed it.

### Review validation failed
- **Issue**: "Posting review blocked."
- **Cause**: Body >5000 chars, title >300 chars, empty content, or rating outside 1–5.
- **Solution**: Trim and use a 1–5 star rating, then resubmit.
- **Escalate**: Customer says body was under the limits but still rejected (profanity-filter false positive).

### Review not appearing
- **Issue**: "I posted a review but I can't see it."
- **Cause**: In moderation (24–48h) or held by spam filter.
- **Solution**: My Reviews shows the approval status.
- **Escalate**: Pending >5 days, or auto-rejected and customer disputes.
