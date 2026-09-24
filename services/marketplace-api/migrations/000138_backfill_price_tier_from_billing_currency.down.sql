-- Revert the backfill: return PPP-currency rows without a Stripe
-- subscription to the 'developed' default they held before 000138.
UPDATE store_subscriptions
SET    price_tier = 'developed'
WHERE  price_tier = 'ppp'
  AND  stripe_subscription_id IS NULL
  AND  lower(billing_currency) IN ('inr', 'myr', 'thb', 'php', 'idr', 'vnd');
