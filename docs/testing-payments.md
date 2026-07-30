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
| Domestic (India) success | Visa credit | `4718 6091 0820 4366` | any 3 digits | any future date |
| International success | Mastercard credit | `5104 0155 5555 5558` | any 3 digits | any future date |
| International success | Mastercard debit | `5104 0600 0000 0008` | any 3 digits | any future date |
| Failure (incorrect OTP/verification) | Visa | `4100 2800 0000 0009` | any 3 digits | any future date |

- Cardholder name/email/phone: anything (Razorpay docs use "Gaurav Kumar").
- **Use the domestic Visa card** unless international payments are enabled on the
  Razorpay account — international cards fail with "International cards not allowed"
  by default. Cards like `4628 9499 7226 2986` / `5105 1051 0510 5100` are from the
  docs' international section and hit this error.

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

## Cashfree test mode

Same test store (`my-god`) — Cashfree is India-only and INR-only, so IN/INR is a
hard requirement, not a convenience. For webhook setup see
[`cashfree-webhook.md`](./cashfree-webhook.md).

### Setup

Requires test-mode credentials from the Cashfree dashboard's **Test** environment
(**Switch to Test** → Developers → API Keys), configured in the store's payment
settings (`payment_gateway_configs`, provider `cashfree`, mode `test`):

| Admin field | Cashfree name | Header |
|---|---|---|
| API key | App ID | `x-client-id` |
| Secret key | Secret Key | `x-client-secret` |

**`mode` must be `test`.** Unlike Razorpay — which serves both environments from
one host and distinguishes them by the `rzp_test_`/`rzp_live_` key prefix —
Cashfree selects the environment by **hostname** (`sandbox.cashfree.com` vs
`api.cashfree.com`). There is no prefix to catch a mistake: test keys sent to the
production host simply fail authentication, and the error says nothing about
mode. If a correctly-entered key returns 401, check this first.

Production activation being "in review" on the Cashfree dashboard does **not**
block test-mode work.

### What to expect at checkout

**Razorpay is the default**, so on an IN store it is pre-selected and badged
"Recommended", with PayPal and Cashfree below it as options. Cashfree is a
deliberate choice the buyer makes — to test it you must **select the Cashfree
radio** before placing the order. Forgetting that step is the likeliest reason a
"Cashfree test" ends up going through Razorpay.

If Cashfree does not appear in the list at all, it has no active gateway config
for that store (see section 2 of `cashfree-verify.sql`) — the storefront only
lists providers that are both allowlisted for the country and configured.

The order confirms via a server-side status poll, not a client signature:
Cashfree's SDK returns no signed receipt, so after the sheet closes the
storefront calls `confirm-payment` and the backend asks Cashfree what was
actually captured. This means **checkout works before the webhook is wired** —
useful for a first smoke test. Refund settlement does need the webhook.

A phone number is mandatory: Cashfree requires `customer_phone` to create an
order at all. The checkout form already enforces a valid 10-digit Indian mobile,
so this is satisfied by construction — but a direct API call without one fails
with an explicit `customer_phone is required` before any HTTP request is made.

### Test instruments

Not reproduced here deliberately. Cashfree's sandbox instrument list (test cards,
UPI VPAs for success/failure, netbanking simulators) is versioned per API
generation, and a stale card number here would look like an integration failure
rather than a wrong card — the exact debugging dead-end this file exists to
prevent. Take them from the dashboard's test-environment payment page or:

https://docs.cashfree.com/docs/test-data

Once you've completed a successful sandbox payment, record the instruments that
worked in a table here, matching the Razorpay section above.

### Verifying a test run

```bash
psql "$DATABASE_URL" -v store_slug=my-god \
  -f services/marketplace-api/scripts/sql/cashfree-verify.sql
```

Section 2 confirms credentials resolved, section 3 lists misconfigurations,
section 4 shows payment transactions, and section 6 shows whether webhooks are
arriving.
