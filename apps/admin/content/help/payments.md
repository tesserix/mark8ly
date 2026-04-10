---
title: "Setting Up Payments"
category: "Store Setup"
order: 3
---

## Supported Payment Gateways

Mark8ly supports Stripe and Razorpay as payment providers. Stripe is available for merchants in most countries and supports credit cards, debit cards, Apple Pay, and Google Pay. Razorpay is the primary option for merchants operating in India and supports UPI, netbanking, wallets, and card payments.

You can configure one or both gateways under **Settings > Payments**. Each gateway operates independently, so you can offer multiple payment options to your customers at checkout.

## Connecting Stripe

To connect Stripe, you need your API keys from the Stripe Dashboard. Navigate to **Settings > Payments**, select Stripe, and enter your publishable key and secret key. Use test mode keys during setup to verify the integration without processing real charges.

Mark8ly uses Stripe's Payment Intents API for secure, SCA-compliant payments. This means customers in regions that require Strong Customer Authentication will see the appropriate verification steps during checkout.

Once connected, toggle the gateway to **Active** to make it available at checkout. You can switch between test and live mode at any time, but remember that test mode transactions do not appear in your live Stripe dashboard.

## Connecting Razorpay

For Razorpay, retrieve your Key ID and Key Secret from the Razorpay Dashboard under **Settings > API Keys**. Enter these credentials in the Mark8ly payment settings and activate the gateway.

Razorpay handles the currency conversion and payment method selection based on the customer's location and preferences. UPI is automatically offered for Indian customers, while international cards work seamlessly for customers elsewhere.

## Managing Transactions

All payment activity is visible in the Orders section of your admin dashboard. Each order shows its payment status: pending, authorized, paid, failed, or refunded. You can issue full or partial refunds directly from the order detail page.

For disputed charges, manage them through your payment provider's dashboard. Mark8ly records the dispute status on the order but the resolution process happens between you, the customer, and the payment provider.

## Security and Compliance

Mark8ly never stores raw card numbers or sensitive payment credentials on its servers. All payment processing happens through your chosen gateway's secure infrastructure. Your API keys are encrypted at rest and transmitted over TLS.

Keep your API keys confidential. If you suspect a key has been compromised, rotate it immediately in your payment provider's dashboard and update the credentials in Mark8ly.
