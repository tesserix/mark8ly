-- 000138 — set price_tier from billing_currency for rows that never had it set.
--
-- price_tier took its 'developed' DEFAULT in 000040 and no code path ever
-- wrote it: there were zero assignments in Go and no UPDATE in any migration.
-- Meanwhile billing_currency has been recorded correctly since signup, so
-- rows read `inr` while every price lookup resolved the developed Price —
-- lookupKeyFor consults the currency ONLY on the PPP tier. The developed
-- Price carries currency_options for the seven developed-market currencies
-- and INR is not among them, so a PPP-country store would have been charged
-- the USD baseline: USD 19 rather than INR 999 on starter monthly.
--
-- Restricted to rows with no Stripe subscription. Changing the tier under a
-- live subscription would leave the row disagreeing with the Price object
-- Stripe is already billing against, which is worse than the bug: that has
-- to be a deliberate plan change through the planchange flow, not a silent
-- backfill. At the time of writing no store has a Stripe subscription, so
-- this covers every existing row.
UPDATE store_subscriptions
SET    price_tier = 'ppp'
WHERE  price_tier = 'developed'
  AND  stripe_subscription_id IS NULL
  AND  lower(billing_currency) IN ('inr', 'myr', 'thb', 'php', 'idr', 'vnd');
