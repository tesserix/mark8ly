---
title: "Setting Up Payments"
category: "Store Setup"
order: 3
---

Mark8ly handles the checkout flow and securely processes transactions through your chosen provider. You can configure one or both gateways under **Settings → Payments** — they operate independently, so customers can pick the option that suits them.

## Supported gateways

| Gateway | Available in | Methods |
| --- | --- | --- |
| **Stripe** | Most countries | Credit and debit cards, Apple Pay, Google Pay |
| **Razorpay** | India | UPI, netbanking, wallets, cards |

## Connecting Stripe

You'll need your API keys from the Stripe Dashboard.

1. Open **Settings → Payments** in Mark8ly and select **Stripe**.
2. Enter your **publishable key** and **secret key**.
3. Use **test mode** keys first to verify the integration without real charges.
4. Once tested, swap to live keys and toggle the gateway to **Active**.

Mark8ly uses Stripe's Payment Intents API, so customers in regions that require Strong Customer Authentication (SCA) automatically see the correct verification steps during checkout.

> Test-mode transactions never appear in your live Stripe dashboard. If a test charge seems to have vanished, check that you're viewing test mode in Stripe.

## Connecting Razorpay

1. In the Razorpay Dashboard, go to **Settings → API Keys** and copy your **Key ID** and **Key Secret**.
2. Paste them into the Razorpay section of Mark8ly's payment settings.
3. Activate the gateway.

Razorpay handles currency conversion and payment-method selection automatically based on the customer's location. UPI is offered for Indian customers, and international cards work seamlessly for everyone else.

## Managing transactions

Every order shows a payment status in the **Orders** view. The lifecycle is:

- **pending** — checkout started, waiting for the gateway.
- **authorized** — funds reserved on the customer's card.
- **paid** — funds captured and settled.
- **failed** — the gateway declined the charge.
- **refunded** — full or partial refund issued.

Issue refunds directly from the order detail page — no need to leave Mark8ly.

For chargebacks and disputes, manage them through your gateway's dashboard. Mark8ly records the dispute status on the order, but resolution happens between you, the customer, and the gateway.

## Security and compliance

Mark8ly never stores raw card numbers or sensitive payment credentials. All processing happens through your gateway's secure infrastructure.

- API keys are encrypted at rest.
- All traffic uses TLS in transit.
- Webhooks from Stripe and Razorpay are verified by signature.

> If you suspect a key has been compromised, **rotate it immediately** in the gateway's dashboard, then update the credentials in Mark8ly.
