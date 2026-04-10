# Feature Review: Orders, Payments, Shipping, Tax

**Date:** 2026-04-10
**Reviewers:** Architect, Go Developer, Tech Lead, Security Reviewer, UX Specialist
**Status:** Findings documented — fixes pending

## Summary

Five specialist agents reviewed the shipped orders, payment, shipping, and tax features. Found 6 critical issues, 12 high, 8 medium. The two most urgent: API keys stored plaintext (C1) and client-supplied payment amounts not verified server-side (C2).

## Critical (P0)

| # | Feature | Issue | Fix |
|---|---------|-------|-----|
| C1 | Payment | API keys stored plaintext despite `_encrypted` column names — DB breach exposes all merchant secrets | Add AES-256-GCM envelope encryption with KMS key |
| C2 | Payment | Client-supplied subtotal/discount used as payment amount — attacker can pay $0.01 for $100 order | Recompute subtotal server-side from `unit_price * quantity`, reject on mismatch |
| C3 | Payment | Webhook config lookup not scoped to tenant/store — cross-tenant key confusion | Add store_id to webhook URL + scoped query |
| C4 | Payment | PayPal webhook signature extraction wrong — passes single header instead of 5-header JSON blob | Build JSON from all 5 PayPal headers + add WebhookID to config |
| C5 | Tax | India GST seller region never set in checkout — all India orders get IGST instead of CGST/SGST | Populate SellerAddress.Region from merchant's registered state |
| C6 | Orders | Sequence number allocation not atomic with order creation — crash between = burned numbers | Move allocation inside Service.Create transaction |

## High (P1)

| # | Feature | Issue |
|---|---------|-------|
| H1 | Payment | Stripe webhook missing timestamp replay check (>300s staleness) |
| H2 | Payment | PayPal refund hardcodes USD — non-USD refunds fail |
| H3 | Payment | Uncapped client-supplied discount_total — attacker can zero out order |
| H4 | Shipping | NinjaVan re-authenticates every call — no token cache, O(n) OAuth round trips |
| H5 | Shipping | Schema mismatch between repository.go and shipping_rates.go column names |
| H6 | Shipping | Free-shipping threshold computed but never applied (dead code) |
| H7 | Orders | Admin handlers don't verify tenant_id — cross-tenant mutation possible |
| H8 | Orders | List endpoint N+1 queries (51 queries per page of 25) |
| H9 | Tax | No fallback when TaxJar unconfigured — silent zero-tax checkout for US stores |
| H10 | Tax | Checkout calls calculator directly, bypasses Service.validateBreakdown |
| H11 | Tax | HSN code + GST rate not propagated from product data to India calculator |
| H12 | Tax | TaxJar JSON tags swapped on special_district fields |

## Medium (P2)

| # | Feature | Issue |
|---|---------|-------|
| M1 | Payment | Razorpay secret key dual-used for API auth + webhook signing |
| M2 | Payment | Provider error bodies (potential PII) logged verbatim |
| M3 | Shipping | NinjaVan tracking/cancel hardcoded to /sg/ country prefix |
| M4 | Shipping | Delhivery rate struct copy-paste error on o_pin JSON tag |
| M5 | Shipping | UpsertCarrierConfig uses FirstOrCreate — silently skips updates |
| M6 | Tax | SaveTaxLines delete+insert not wrapped in transaction |
| M7 | Tax | Service rounding tolerance too loose for multi-item orders |
| M8 | Orders | Extended checkout silently swallows shipping/tax failures |

## Recommended Fix Order

### Session 1: Critical security (C1, C2, C3, C4)
- Add envelope encryption for API keys
- Server-side subtotal recomputation
- Scope webhook lookup to store_id
- Fix PayPal webhook signature extraction

### Session 2: Critical correctness (C5, C6)
- Populate India GST seller region from merchant profile
- Move sequence allocation into order create transaction

### Session 3: High priority (H1-H12)
- Stripe timestamp replay check
- PayPal refund currency fix
- Discount cap validation
- NinjaVan token caching
- Schema mismatch fix
- Free-shipping threshold implementation
- Tenant_id verification on admin handlers
- Orders N+1 batch loading
- TaxJar fallback behavior
- Checkout→service routing for tax
- HSN/GST rate propagation
- TaxJar JSON tag fix

### Session 4: Medium (M1-M8)
- Remaining cleanup items
