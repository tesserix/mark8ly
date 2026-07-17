# Payment Testing Reference

Reference for testing payment integrations against a live test store. Test mode only —
never use these details with live-mode gateway keys.

## Test store: My God (India / Razorpay)

| Field | Value |
|---|---|
| Storefront | https://my-god.mark8ly.com |
| Admin | https://my-god-admin.mark8ly.com |
| Tenant ID | `0d7d8563-f155-4520-8238-45e646e4d8fa` |
| Store ID | `c597071b-632b-40b0-950c-70a0ce547553` |
| Slug | `my-god` |
| Country / Currency | IN / INR (timezone `Asia/Kolkata`) |
| Owner login | samyak.rout@gmail.com |

The store was moved from AU/AUD to IN/INR (2026-07-17) specifically for Razorpay testing.
Store country/currency lives in **both** `mark8ly_platform_api.stores` and
`mark8ly_marketplace_api.stores` — keep them in sync if changing again. Note the
marketplace_api copy has no `updated_at` column.

## Razorpay test mode

Requires test-mode API keys (`rzp_test_...`) configured in the store's payment settings
(`payment_gateway_configs` table, provider `razorpay`, mode `test`).

### Test cards

| Purpose | Network | Number | CVV | Expiry |
|---|---|---|---|---|
| Domestic (India) success | Visa | `4628 9499 7226 2986` | any 3 digits | any future date |
| International success | Mastercard | `5105 1051 0510 5100` | any 3 digits | any future date |

- Cardholder name/email/phone: anything (Razorpay docs use "Gaurav Kumar").
- The international Mastercard requires completing the address-verification form at checkout.

### OTP screen (test mode mock — no real SMS)

- Any **4–10 digit** number → payment succeeds
- Fewer than 4 digits → simulates payment failure

### Test UPI IDs

| VPA | Result |
|---|---|
| `success@razorpay` | Payment succeeds |
| `failure@razorpay` | Payment fails |

### Source

https://razorpay.com/docs/payments/payments/test-card-details/ — see the docs for
error-scenario cards (BAD_REQUEST_ERROR / GATEWAY_ERROR), subscription cards, and EMI cards.
