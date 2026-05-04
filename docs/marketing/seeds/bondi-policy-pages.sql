-- bondi-policy-pages.sql
-- Seeds the_bondi_store tenant with 6 standard CMS policy pages and wires
-- them into the storefront footer via store_branding.footer_sections.
--
-- Idempotent: pages use ON CONFLICT (store_id, slug) DO UPDATE; branding uses
-- a column-scoped UPDATE that won't touch unrelated fields.
--
-- Jurisdiction: Australia (Sydney/NSW). Australian Consumer Law + Privacy Act
-- 1988 (APPs) baseline. Ships worldwide.
--
-- Run against the marketplace-api database:
--   psql "$MARKETPLACE_DB_URL" -f docs/marketing/seeds/bondi-policy-pages.sql

\set ON_ERROR_STOP on

BEGIN;

-- ──────────────────────────────────────────────────────────────────────
-- Sanity: confirm exactly one store exists for the tenant. Abort if not.
-- ──────────────────────────────────────────────────────────────────────
DO $$
DECLARE
  store_count int;
BEGIN
  SELECT count(*) INTO store_count
  FROM stores
  WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e';
  IF store_count <> 1 THEN
    RAISE EXCEPTION
      'Expected exactly 1 store for tenant 8c302556-..., found %. Aborting seed.',
      store_count;
  END IF;
END $$;

-- Convenience: a temp table holding the resolved store_id for use below.
CREATE TEMP TABLE _bondi ON COMMIT DROP AS
SELECT id AS store_id, tenant_id
FROM stores
WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'
LIMIT 1;

-- ──────────────────────────────────────────────────────────────────────
-- 1. About — sort_order 10
-- ──────────────────────────────────────────────────────────────────────
INSERT INTO pages (tenant_id, store_id, slug, title, body, sort_order, published)
SELECT tenant_id, store_id, 'about', 'Our story',
$body$
The Bondi Store began with a simple observation: most of the things we wore on
the long days at the beach were either too precious to get sand on, or made so
poorly they didn't last a season. We wanted clothing that lived comfortably in
between — sun-bleached linen, weighty cotton, soft merino — pieces you could
swim in, sleep in, and wear home for dinner.

We're a small studio working out of Sydney's east. Every season we make a
short list, in considered quantities, with makers we trust. We don't run sales,
we don't chase trends, and we'd rather sell out of a colour than over-order
into discount. Our pieces are designed to be worn for years and to look better
the longer you have them.

We ship from Sydney to anywhere in the world. If something isn't right, we'll
make it right.

— Mahesh & the team
$body$, 10, true
FROM _bondi
ON CONFLICT (store_id, slug) DO UPDATE
  SET title = EXCLUDED.title, body = EXCLUDED.body,
      sort_order = EXCLUDED.sort_order, published = EXCLUDED.published,
      updated_at = NOW();

-- ──────────────────────────────────────────────────────────────────────
-- 2. Contact — sort_order 20
-- ──────────────────────────────────────────────────────────────────────
INSERT INTO pages (tenant_id, store_id, slug, title, body, sort_order, published)
SELECT tenant_id, store_id, 'contact', 'Contact us',
$body$
The fastest way to reach us is by email — we read everything ourselves and
reply within one business day, usually faster.

**General enquiries, orders, returns**
hello@the-bondi-store.com

**Wholesale, press, collaborations**
hello@the-bondi-store.com — please put **Wholesale** or **Press** in the
subject line so we can route it to the right inbox.

**Studio**
By appointment only. Email first.
Sydney, NSW, Australia

We work Monday to Friday, 9–5 AEST. Anything you send over a weekend will land
back in your inbox by Monday afternoon.
$body$, 20, true
FROM _bondi
ON CONFLICT (store_id, slug) DO UPDATE
  SET title = EXCLUDED.title, body = EXCLUDED.body,
      sort_order = EXCLUDED.sort_order, published = EXCLUDED.published,
      updated_at = NOW();

-- ──────────────────────────────────────────────────────────────────────
-- 3. Shipping — sort_order 30
-- ──────────────────────────────────────────────────────────────────────
INSERT INTO pages (tenant_id, store_id, slug, title, body, sort_order, published)
SELECT tenant_id, store_id, 'shipping', 'Shipping',
$body$
We ship every order from our Sydney studio. Most go out within one to three
business days of being placed.

## Australia
- **Standard** — AUD 12, free over AUD 150. Delivered in 3–6 business days
  via Australia Post.
- **Express** — AUD 18 (free over AUD 250). Delivered in 1–3 business days
  via Australia Post Express.
- A tracking link arrives by email the moment we ship.

## International
We ship worldwide via Australia Post International. Rates are calculated at
checkout based on weight and destination. Typical transit times after
dispatch:

- **New Zealand** — 5–10 business days
- **United States, Canada, UK, Europe** — 7–14 business days
- **Rest of world** — 10–21 business days

**Customs & duties.** International orders may attract import duties or taxes
once they arrive in your country. These are set by your local customs office,
not by us, and are the responsibility of the customer. We mark every parcel
honestly with its actual contents and value.

## Processing
We aim to ship within one to three business days. Pre-orders and made-to-order
items will show their dispatch window on the product page. Sales periods or
new releases may add a day.

## Tracking & lost parcels
Every order ships with tracking. If your parcel hasn't moved for more than
seven business days after dispatch, email us at hello@the-bondi-store.com
with your order number and we'll lodge an enquiry with the carrier and keep
you posted.

We don't accept liability for parcels marked "delivered" by the carrier where
the wrong delivery address was provided at checkout — please double-check
before paying.
$body$, 30, true
FROM _bondi
ON CONFLICT (store_id, slug) DO UPDATE
  SET title = EXCLUDED.title, body = EXCLUDED.body,
      sort_order = EXCLUDED.sort_order, published = EXCLUDED.published,
      updated_at = NOW();

-- ──────────────────────────────────────────────────────────────────────
-- 4. Refund / Returns — sort_order 40
-- ──────────────────────────────────────────────────────────────────────
INSERT INTO pages (tenant_id, store_id, slug, title, body, sort_order, published)
SELECT tenant_id, store_id, 'refund', 'Returns & refunds',
$body$
We want you to love what you ordered. If something isn't right — wrong size,
wrong colour, or just not what you expected — we'll exchange it or refund it.

## The window
**30 days** from the day your parcel was delivered to begin a return.

## What we accept
- Unworn, unwashed, in original condition
- Original tags and packaging attached
- Proof of purchase (your order number is enough)

## What we can't accept
- Sale or final-sale items (clearly marked at checkout)
- Items returned outside the 30-day window
- Items damaged through wear, washing, or alteration after delivery
- Underwear, swimwear, or earrings, for hygiene reasons — unless faulty

## How to start a return
Email **hello@the-bondi-store.com** with your order number and what you'd
like to return or exchange. We'll send a return address and instructions
within one business day.

## Who pays return shipping
- **Change of mind** — return shipping is at your cost. Use a tracked
  service; we can't refund parcels that don't arrive.
- **Faulty, damaged, or incorrectly sent** — we cover return shipping.
  Send us a photo when you get in touch and we'll arrange a prepaid label.

## When you'll see your money
Once we receive and inspect the return, we issue the refund to your original
payment method within 3 business days. Depending on your bank, the funds may
take a further 3–10 business days to appear.

## International returns
The same window and conditions apply. International return shipping for
change-of-mind returns is at your cost; for faulty items we'll reimburse
reasonable shipping on production of a receipt.

## Your rights under Australian Consumer Law
Our goods come with guarantees that cannot be excluded under the Australian
Consumer Law. You are entitled to a replacement or refund for a major failure
and to compensation for any other reasonably foreseeable loss or damage. You
are also entitled to have the goods repaired or replaced if the goods fail
to be of acceptable quality and the failure does not amount to a major
failure. Nothing in this policy limits those rights.
$body$, 40, true
FROM _bondi
ON CONFLICT (store_id, slug) DO UPDATE
  SET title = EXCLUDED.title, body = EXCLUDED.body,
      sort_order = EXCLUDED.sort_order, published = EXCLUDED.published,
      updated_at = NOW();

-- ──────────────────────────────────────────────────────────────────────
-- 5. Terms of service — sort_order 50
-- ──────────────────────────────────────────────────────────────────────
INSERT INTO pages (tenant_id, store_id, slug, title, body, sort_order, published)
SELECT tenant_id, store_id, 'terms', 'Terms of service',
$body$
These terms govern your use of the-bondi-store.mark8ly.com (the **site**)
and any orders you place with The Bondi Store (**we**, **us**, **our**).

By using the site or placing an order, you agree to these terms. If you don't,
please don't use the site.

## 1. About us
The Bondi Store is operated by Tesserix Pty Ltd, an Australian company based
in Sydney, New South Wales. Our contact email is hello@the-bondi-store.com.

## 2. Eligibility
You must be at least 18 years old, or have the consent of a parent or
guardian, to place an order. By ordering, you confirm you meet this
requirement.

## 3. Orders
- Each order is an offer by you to purchase the goods listed at the price
  shown, subject to acceptance by us.
- We may decline an order at our discretion — for example if a product is
  out of stock, the price is wrong because of an obvious typographical
  error, or we suspect fraud. In that case we'll refund any payment you've
  made in full.
- An order is accepted when we send a dispatch confirmation, not when we
  send the order receipt.

## 4. Pricing & payment
- Prices are shown in Australian dollars (AUD) and include GST where
  applicable for Australian customers.
- International orders are quoted in AUD; your bank will convert at the
  prevailing rate. Any conversion fees are between you and your bank.
- Payment is taken at checkout via our payment processor. We don't store
  your full card details.

## 5. Shipping & delivery
Shipping terms, costs, and timeframes are described on our **Shipping** page,
which is incorporated into these terms.

## 6. Returns
Our returns and refund process is described on our **Returns & refunds** page,
which is incorporated into these terms. Nothing in our returns policy limits
your rights under the Australian Consumer Law.

## 7. Intellectual property
All content on the site — text, photographs, illustrations, the brand name,
the marks, and the page design — is owned by us or licensed to us. You may
view and share it for personal, non-commercial purposes. You may not copy it
for resale, train AI models on it, or use it commercially without written
permission.

## 8. Acceptable use
You agree not to use the site to:
- attempt to circumvent security controls
- post defamatory, harassing, or unlawful content
- scrape or systematically harvest content
- impersonate another person or business

We may suspend access if we reasonably believe you've breached these terms.

## 9. Liability
To the fullest extent permitted by law, we're not liable for indirect,
incidental, or consequential loss arising out of your use of the site or
the goods. Our total liability for any claim is limited to the amount you
paid for the goods that gave rise to the claim. **Nothing in these terms
limits any rights you have under the Australian Consumer Law that cannot be
excluded.**

## 10. Privacy
We handle personal information in accordance with our **Privacy policy**.

## 11. Governing law
These terms are governed by the laws of New South Wales, Australia. Any
dispute that can't be resolved by good-faith discussion will be heard by the
courts of New South Wales.

## 12. Changes
We may update these terms from time to time. The current version always lives
at this URL. Material changes will be flagged on the homepage for at least
14 days.

**Last updated:** 4 May 2026
$body$, 50, true
FROM _bondi
ON CONFLICT (store_id, slug) DO UPDATE
  SET title = EXCLUDED.title, body = EXCLUDED.body,
      sort_order = EXCLUDED.sort_order, published = EXCLUDED.published,
      updated_at = NOW();

-- ──────────────────────────────────────────────────────────────────────
-- 6. Privacy policy — sort_order 60
-- ──────────────────────────────────────────────────────────────────────
INSERT INTO pages (tenant_id, store_id, slug, title, body, sort_order, published)
SELECT tenant_id, store_id, 'privacy', 'Privacy policy',
$body$
We take your privacy seriously and handle personal information in accordance
with the Australian **Privacy Act 1988 (Cth)** and the Australian Privacy
Principles (APPs). This page explains what we collect, why, and what you can
do about it.

## Who we are
The Bondi Store is operated by Tesserix Pty Ltd, an Australian company based
in Sydney, NSW. Our contact for privacy questions is
hello@the-bondi-store.com.

## What we collect
- **Account & order details** — your name, email, shipping and billing
  address, phone number (if provided), and order history.
- **Payment details** — handled by our payment processor (Stripe). We don't
  store your full card number on our servers; we receive a confirmation token.
- **Communications** — emails you send us and our replies.
- **Browse data** — pages you view, items you add to cart, the device and
  browser you used. Collected via cookies and similar technologies.
- **Marketing preferences** — whether you've opted in to email marketing.

## Why we collect it
- To process and ship your orders
- To respond to your questions
- To send transactional emails (order confirmations, dispatch notices,
  return updates)
- With your consent, to send marketing emails about new releases. You can
  unsubscribe at any time using the link in every marketing email.
- To detect and prevent fraud
- To comply with our legal obligations (tax, accounting, consumer law)
- To improve the site — anonymous, aggregated patterns only

## Who we share it with
We share personal information only with parties who help us run the
business, under contract, and only to the extent they need it. These include:

- **Payment processor** — Stripe (details handled per Stripe's privacy
  policy)
- **Shipping carriers** — Australia Post and equivalent international
  partners, to deliver your order
- **Email & analytics** — providers that help us send transactional emails
  and understand site usage
- **Hosting** — Google Cloud Platform, primarily within Australian regions
- **Professional advisers** — accountants and lawyers under confidentiality

We do **not** sell your personal information.

## International transfers
Some of our service providers (e.g. Stripe, Google Cloud) may store data
outside Australia. Where this happens, we take reasonable steps to ensure
the receiving party handles it consistently with the APPs.

## How long we keep it
We keep order and accounting records for at least 7 years to meet
Australian tax law. Marketing data is kept for as long as you remain
subscribed plus 12 months. Other personal information is kept only as long
as we need it for the purposes above.

## Cookies
We use:
- **Essential cookies** — for cart, checkout, and login. Always on.
- **Analytics cookies** — to understand site usage. Anonymous and
  aggregated. Set only with your consent where required.

You can clear or block cookies in your browser; some site features may not
work without them.

## Your rights
Under the APPs you have the right to:
- ask what personal information we hold about you
- ask us to correct it if it's wrong
- ask us to delete it (subject to our legal record-keeping obligations)
- complain if you think we've mishandled it

To exercise any of these rights, email hello@the-bondi-store.com. We'll
respond within 30 days. If you're not happy with our response, you can lodge
a complaint with the **Office of the Australian Information Commissioner**
(OAIC) at oaic.gov.au.

## Children
Our site and service aren't directed at children under 18. We don't
knowingly collect personal information from children. If you believe we
have, please contact us and we'll delete it.

## Changes
We may update this policy from time to time. Material changes will be
flagged on the homepage for at least 14 days.

**Last updated:** 4 May 2026
$body$, 60, true
FROM _bondi
ON CONFLICT (store_id, slug) DO UPDATE
  SET title = EXCLUDED.title, body = EXCLUDED.body,
      sort_order = EXCLUDED.sort_order, published = EXCLUDED.published,
      updated_at = NOW();

-- ──────────────────────────────────────────────────────────────────────
-- Branding: footer sections, social, tagline.
--
-- footer_sections is constrained: max 6 sections, max 10 items/section,
-- slug regex `^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`, urls must start with
-- http(s):// or /.
-- ──────────────────────────────────────────────────────────────────────
UPDATE store_branding
SET
  footer_sections = $json$
[
  {
    "label": "Help",
    "items": [
      {"label": "Contact",            "kind": "page", "page_slug": "contact"},
      {"label": "Shipping",           "kind": "page", "page_slug": "shipping"},
      {"label": "Returns & refunds",  "kind": "page", "page_slug": "refund"}
    ]
  },
  {
    "label": "Legal",
    "items": [
      {"label": "Terms of service",   "kind": "page", "page_slug": "terms"},
      {"label": "Privacy policy",     "kind": "page", "page_slug": "privacy"}
    ]
  },
  {
    "label": "About",
    "items": [
      {"label": "Our story",          "kind": "page", "page_slug": "about"}
    ]
  }
]
$json$::jsonb,
  social_instagram = 'https://instagram.com/the_bondi_store',
  footer_tagline   = 'Sun-bleached linen, made for the long Bondi summer.',
  footer_copyright = '© 2026 The Bondi Store · Sydney',
  updated_at       = NOW()
WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e';

-- Sanity: confirm the branding update touched exactly one row.
DO $$
DECLARE
  updated_count int;
BEGIN
  SELECT count(*) INTO updated_count
  FROM store_branding
  WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e';
  IF updated_count <> 1 THEN
    RAISE EXCEPTION
      'Expected exactly 1 store_branding row for tenant 8c302556-..., found %.',
      updated_count;
  END IF;
END $$;

COMMIT;

-- ──────────────────────────────────────────────────────────────────────
-- After run: hard-refresh the storefront in a private window and confirm:
--   • Footer shows three sections: Help, Legal, About
--   • Each link opens /pages/<slug> with the rendered markdown
--   • Instagram icon row in footer links to @the_bondi_store
--   • Tagline + copyright render under the brand row
-- ──────────────────────────────────────────────────────────────────────
