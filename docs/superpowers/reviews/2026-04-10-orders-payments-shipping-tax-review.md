# Feature Review: Orders, Payments, Shipping, Tax

**Date:** 2026-04-10
**Reviewers:** Architect, Go Developer, Tech Lead, Security Reviewer, UX Specialist
**Status:** Most issues fixed — C1, C3, M1, M2 deferred (infrastructure changes needed)

## Summary

Five specialist agents reviewed the shipped orders, payment, shipping, and tax features. Found 6 critical issues, 12 high, 8 medium. The two most urgent: API keys stored plaintext (C1) and client-supplied payment amounts not verified server-side (C2).

## Critical (P0)

| # | Feature | Issue | Status |
|---|---------|-------|--------|
| C1 | Payment | API keys stored plaintext despite `_encrypted` column names — DB breach exposes all merchant secrets | **DEFERRED** — needs KMS envelope encryption infrastructure |
| C2 | Payment | Client-supplied subtotal/discount used as payment amount — attacker can pay $0.01 for $100 order | **FIXED** — server-side subtotal recomputation from unit_price × quantity in checkout_ext.go |
| C3 | Payment | Webhook config lookup not scoped to tenant/store — cross-tenant key confusion | **DEFERRED** — needs webhook handler refactoring |
| C4 | Payment | PayPal webhook signature extraction wrong — passes single header instead of 5-header JSON blob | **ALREADY FIXED** — current code correctly expects JSON-encoded paypalSignatureHeaders with all 5 fields + WebhookID |
| C5 | Tax | India GST seller region never set in checkout — all India orders get IGST instead of CGST/SGST | **FIXED** — SellerAddress.Region populated from carrier config warehouse_region in checkout_ext.go |
| C6 | Orders | Sequence number allocation not atomic with order creation — crash between = burned numbers | **FIXED** — Service.Create allocates seq inside its own tx; callers pass OrderNumberSeq=0 |

## High (P1)

| # | Feature | Issue | Status |
|---|---------|-------|--------|
| H1 | Payment | Stripe webhook missing timestamp replay check (>300s staleness) | **FIXED** — verifyStripeSignature now rejects timestamps older than 5 minutes |
| H2 | Payment | PayPal refund hardcodes USD — non-USD refunds fail | **FIXED** — added CurrencyCode to RefundInput, PayPal uses it instead of hardcoded "USD" |
| H3 | Payment | Uncapped client-supplied discount_total — attacker can zero out order | **FIXED** — checkout_ext.go rejects discount_total > subtotal |
| H4 | Shipping | NinjaVan re-authenticates every call — no token cache, O(n) OAuth round trips | **FIXED** — added sync.Mutex token cache with ~24h expiry |
| H5 | Shipping | Schema mismatch between repository.go and shipping_rates.go column names | **REVIEWED** — column names align with migration schema; no mismatch found in current code |
| H6 | Shipping | Free-shipping threshold computed but never applied (dead code) | **FIXED** — ShippingService.GetShippingRates now accepts orderSubtotal and zeros price when threshold met |
| H7 | Orders | Admin handlers don't verify tenant_id — cross-tenant mutation possible | **FIXED** — verifyOrderOwnership checks store_id + tenant_id before Confirm/Fulfill/Cancel/Refund |
| H8 | Orders | List endpoint N+1 queries (51 queries per page of 25) | **FIXED** — batch-loads items + addresses in 2 queries via LoadChildrenBatch |
| H9 | Tax | No fallback when TaxJar unconfigured — silent zero-tax checkout for US stores | **FIXED** — checkout_ext.go returns error when TaxJar strategy has no API key configured |
| H10 | Tax | Checkout calls calculator directly, bypasses Service.validateBreakdown | **FIXED** — checkout_ext.go now routes through tax.Service.CalculateOrderTax |
| H11 | Tax | HSN code + GST rate not propagated from product data to India calculator | **FIXED** — added HSNCode, GSTRate, TaxCode fields to CheckoutItemRequest; propagated to TaxableItem |
| H12 | Tax | TaxJar JSON tags swapped on special_district fields | **FIXED** — SpecialTaxRate→"special_tax_rate", SpecialTaxCollectable→"special_district_tax_collected" |

## Medium (P2)

| # | Feature | Issue | Status |
|---|---------|-------|--------|
| M1 | Payment | Razorpay secret key dual-used for API auth + webhook signing | **DEFERRED** — needs separate webhook_secret config column |
| M2 | Payment | Provider error bodies (potential PII) logged verbatim | **DEFERRED** — needs structured error sanitization policy across all providers |
| M3 | Shipping | NinjaVan tracking/cancel hardcoded to /sg/ country prefix | **FIXED** — NinjaVanCarrier stores country from constructor; all paths use c.country |
| M4 | Shipping | Delhivery rate struct copy-paste error on o_pin JSON tag | **FIXED** — corrected all dlRateRequest JSON tags to match Delhivery API fields |
| M5 | Shipping | UpsertCarrierConfig uses FirstOrCreate — silently skips updates | **FIXED** — replaced with explicit lookup + create-or-update pattern |
| M6 | Tax | SaveTaxLines delete+insert not wrapped in transaction | **FIXED** — wrapped in db.Transaction (runs as savepoint when caller passes tx) |
| M7 | Tax | Service rounding tolerance too loose for multi-item orders | **FIXED** — tolerance now scales with number of tax lines (0.01 per line) |
| M8 | Orders | Extended checkout silently swallows shipping/tax failures | **FIXED** — checkout_ext.go now returns errors to client instead of falling back to zero |

## Deferred Items (Require Infrastructure Changes)

### C1: Envelope encryption for API keys
- Needs AES-256-GCM encryption with KMS-managed key
- Requires migration to encrypt existing plaintext values
- Config columns already named `_encrypted` — just need the actual encryption

### C3: Webhook scoping
- Webhook endpoint needs store_id in URL path or payload
- Config lookup must scope by (store_id, provider) not just provider

### M1: Razorpay webhook secret separation
- Add `webhook_secret` column to payment_gateway_configs
- Update Razorpay gateway to use separate key for webhook verification

### M2: PII sanitization in logs
- Define structured error types for provider API failures
- Strip response bodies before logging; preserve only status code + error code
