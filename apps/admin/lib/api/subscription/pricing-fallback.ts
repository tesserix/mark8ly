/**
 * Static pricing catalogue fallback (spec §2, §3.2, §4.1, §4.1.1).
 *
 * Used when the backend /api/v1/public/pricing endpoint is unavailable (404
 * or network error). Prices are in minor units (cents / paise).
 *
 * TODO: sync exact per-currency amounts with the backend once the public
 * pricing endpoint ships. USD and INR are spec-exact. Other developed-market
 * currencies follow the §4.1 table. PPP currencies (MYR, THB, PHP, IDR, VND)
 * are derived from spec §4.1.1.
 *
 * Features per plan are sourced from services/marketplace-api/internal/plangate/matrix.go.
 */
import type { Currency, PricingCatalogue } from './schemas/plan'

// ---------------------------------------------------------------------------
// White-label app add-on (USD-denominated globally; no PPP — §4.1.2)
// ---------------------------------------------------------------------------

const WHITE_LABEL_ADDON_USD_MONTHLY_MINOR = 19900 // $199.00/mo

// ---------------------------------------------------------------------------
// Starter features (matrix: 2 stores, 25 img, 90d audit, 15k campaign emails)
// ---------------------------------------------------------------------------

const STARTER_FEATURES = [
  { label: '2 stores', included: true },
  { label: '25 images per product', included: true },
  { label: 'Custom domain', included: true },
  { label: 'Full colour palette + announcement bar', included: true },
  { label: '15,000 campaign emails / month', included: true },
  { label: 'Shipping labels, returns, reviews, gift cards', included: true },
  { label: 'Read/write API access', included: false },
  { label: 'Custom CSS', included: false },
  { label: 'SSO (SAML/OIDC)', included: false },
]

// ---------------------------------------------------------------------------
// Studio features (matrix: 5 stores, 50 img, 365d audit, 50k emails, read API)
// ---------------------------------------------------------------------------

const STUDIO_FEATURES = [
  { label: '5 stores', included: true, highlight: true },
  { label: '50 images per product', included: true },
  { label: 'Custom domain', included: true },
  { label: 'Full colour palette + announcement bar', included: true },
  { label: '50,000 campaign emails / month', included: true, highlight: true },
  { label: 'Custom CSS', included: true, highlight: true },
  { label: '12-month audit log retention', included: true },
  { label: 'Read-only API access', included: true },
  { label: 'SSO (SAML/OIDC)', included: false },
]

// ---------------------------------------------------------------------------
// Pro features (matrix: 10 stores, unlimited img, forever audit, full R/W API, SSO)
// ---------------------------------------------------------------------------

const PRO_FEATURES = [
  { label: '10 stores', included: true, highlight: true },
  { label: 'Unlimited images per product', included: true, highlight: true },
  { label: 'Full read/write API access', included: true, highlight: true },
  { label: 'SSO (SAML/OIDC)', included: true, highlight: true },
  { label: 'Custom code injection', included: true },
  { label: 'Unlimited audit log retention', included: true },
  { label: 'Priority email support (4-hour response)', included: true },
]

// ---------------------------------------------------------------------------
// Catalogue builders
// ---------------------------------------------------------------------------

function makeWhiteLabelAddon(currency: Currency, priceMonthlyMinor: number) {
  return {
    id: 'white_label_app' as const,
    name: 'White-label Mobile App',
    priceMonthlyMinor,
    currency,
  }
}

function makeCatalogue(
  currency: Currency,
  starterMonthly: number,
  starterAnnual: number,
  studioMonthly: number,
  studioAnnual: number,
  proAnnual: number,
  proMonthlyPremium: number,
  whiteLabelAddonMonthly: number,
): PricingCatalogue {
  return {
    currency,
    plans: [
      {
        id: 'starter',
        name: 'Starter',
        tagline: 'For merchants launching their first store.',
        pricing: {
          monthly: { amount: starterMonthly, currency, per: 'month' },
          annual: {
            amount: starterAnnual,
            currency,
            per: 'year',
            monthlyEquivalent: Math.round(starterAnnual / 12),
          },
        },
        features: STARTER_FEATURES,
      },
      {
        id: 'studio',
        name: 'Studio',
        tagline: 'For growing brands managing multiple stores.',
        featured: true,
        pricing: {
          monthly: { amount: studioMonthly, currency, per: 'month' },
          annual: {
            amount: studioAnnual,
            currency,
            per: 'year',
            monthlyEquivalent: Math.round(studioAnnual / 12),
          },
        },
        features: STUDIO_FEATURES,
      },
      {
        id: 'pro',
        name: 'Pro',
        tagline: 'Built for teams scaling past serious order volumes.',
        pricing: {
          // Monthly Pro carries a +20% premium (spec §3.2, Council finding)
          monthly: { amount: proMonthlyPremium, currency, per: 'month' },
          annual: {
            amount: proAnnual,
            currency,
            per: 'year',
            monthlyEquivalent: Math.round(proAnnual / 12),
          },
        },
        addOns: [makeWhiteLabelAddon(currency, whiteLabelAddonMonthly)],
        features: PRO_FEATURES,
      },
    ],
  }
}

// ---------------------------------------------------------------------------
// Per-currency catalogues (spec §4.1 + §4.1.1, amounts in minor units)
// ---------------------------------------------------------------------------

// USD (spec §4.1 exact)
// Starter: $19/mo, $182/yr — Studio: $49/mo, $470/yr — Pro: $1,188/yr, $119/mo (+20%)
const USD = makeCatalogue(
  'USD',
  1900, 18200,
  4900, 47000,
  118800, 11900,
  WHITE_LABEL_ADDON_USD_MONTHLY_MINOR,
)

// GBP (spec §4.1 exact: £15/mo, £144/yr — £39/mo, £375/yr — £948/yr)
// Pro monthly +20%: £79 × 1.2 ≈ £95/mo
const GBP = makeCatalogue(
  'GBP',
  1500, 14400,
  3900, 37500,
  94800, 9500,
  19900, // add-on still USD-priced per spec §4.1.2; TODO: confirm GBP pricing with backend
)

// EUR (spec §4.1 exact: €17/mo, €163/yr — €45/mo, €432/yr — €1,068/yr)
// Pro monthly +20%: €89 × 1.2 ≈ €107/mo
const EUR = makeCatalogue(
  'EUR',
  1700, 16300,
  4500, 43200,
  106800, 10700,
  19900,
)

// CAD (spec §4.1 exact: C$25/mo, C$239/yr — C$65/mo, C$625/yr — C$1,619/yr)
// Pro monthly +20%: C$135 × 1.2 = C$162/mo
const CAD = makeCatalogue(
  'CAD',
  2500, 23900,
  6500, 62500,
  161900, 16200,
  19900,
)

// AUD (spec §4.1: A$29+GST/mo, A$278+GST/yr — A$75+GST/mo, A$719+GST/yr — A$1,788+GST/yr)
// Prices are GST-exclusive per spec §19.4. Pro monthly +20%: ~A$149+GST/mo
const AUD = makeCatalogue(
  'AUD',
  2900, 27800,
  7500, 71900,
  178800, 14900,
  19900,
)

// SGD (spec §4.1: S$25/mo, S$239/yr — S$65/mo, S$623/yr — S$1,548/yr)
// Pro monthly +20%: S$129 × 1.2 ≈ S$155/mo
const SGD = makeCatalogue(
  'SGD',
  2500, 23900,
  6500, 62300,
  154800, 15500,
  19900,
)

// NZD (spec §4.1: NZ$29/mo, NZ$278/yr — NZ$75/mo, NZ$719/yr — NZ$1,788/yr)
// Pro monthly +20%: ~NZ$149/mo
const NZD = makeCatalogue(
  'NZD',
  2900, 27800,
  7500, 71900,
  178800, 14900,
  19900,
)

// INR (spec §4.1.1 exact: ₹999/mo, ₹9,599/yr — ₹2,499/mo, ₹23,999/yr — ₹65,999/yr, ₹5,499/mo eq)
// Pro monthly +20%: ₹5,499 × 1.2 ≈ ₹6,599/mo
const INR = makeCatalogue(
  'INR',
  99900, 959900,
  249900, 2399900,
  6599900, 659900,
  19900, // USD-denominated per spec §4.1.2; TODO: confirm if backend bills in INR for this add-on
)

// MYR (spec §4.1.1: RM 59/mo, RM 569/yr — RM 149/mo, RM 1,429/yr — RM 3,588/yr)
// Pro monthly +20%: RM 299 × 1.2 ≈ RM 359/mo
const MYR = makeCatalogue(
  'MYR',
  5900, 56900,
  14900, 142900,
  358800, 35900,
  19900,
)

// THB (spec §4.1.1: ฿499/mo, ฿4,799/yr — ฿1,199/mo, ฿11,519/yr — ฿28,788/yr)
// Pro monthly +20%: ฿2,399 × 1.2 ≈ ฿2,879/mo
const THB = makeCatalogue(
  'THB',
  49900, 479900,
  119900, 1151900,
  2878800, 287900,
  19900,
)

// PHP (spec §4.1.1: ₱749/mo, ₱7,199/yr — ₱1,899/mo, ₱18,239/yr — ₱45,588/yr)
// Pro monthly +20%: ₱3,799 × 1.2 ≈ ₱4,559/mo
const PHP = makeCatalogue(
  'PHP',
  74900, 719900,
  189900, 1823900,
  4558800, 455900,
  19900,
)

// IDR (spec §4.1.1: Rp 199,000/mo, Rp 1,919,000/yr — Rp 499,000/mo, Rp 4,799,000/yr — Rp 11,988,000/yr)
// Pro monthly +20%: Rp 999,000 × 1.2 ≈ Rp 1,199,000/mo
const IDR = makeCatalogue(
  'IDR',
  19900000, 191900000,
  49900000, 479900000,
  1198800000, 119900000,
  19900,
)

// VND (spec §4.1.1: ₫329,000/mo, ₫3,169,000/yr — ₫799,000/mo, ₫7,699,000/yr — ₫19,788,000/yr)
// Pro monthly +20%: ₫1,649,000 × 1.2 ≈ ₫1,979,000/mo
const VND = makeCatalogue(
  'VND',
  32900000, 316900000,
  79900000, 769900000,
  1978800000, 197900000,
  19900,
)

// ---------------------------------------------------------------------------
// Exported map: Currency → PricingCatalogue
// ---------------------------------------------------------------------------

export const LOCAL_PRICING_FALLBACK: Record<Currency, PricingCatalogue> = {
  USD,
  GBP,
  EUR,
  INR,
  CAD,
  AUD,
  SGD,
  NZD,
  MYR,
  THB,
  PHP,
  IDR,
  VND,
}
