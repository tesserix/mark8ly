/**
 * Pricing page copy — single source of truth for all editorial strings.
 *
 * Voice: calm, editorial, confident. Never urgency. Never "Hey there!".
 * See plan lines 289–300 for the correct/wrong examples table.
 *
 * All strings live here so a reviewer can catch voice drift in one file.
 */

export interface PlanCopyItem {
  id: 'starter' | 'studio' | 'pro'
  name: string
  tagline: string
  cta: string
  ctaHref: string
  features: string[]
}

export const pricingCopy = {
  /** Page headline. Source Serif 4, large. */
  h1: 'Pricing that grows with you.',

  /** Lead paragraph under the headline. */
  intro:
    'Three plans. Clear limits. No surprise fees. Change plans any time — upgrades prorate, downgrades wait for the period to close.',

  /** Billing toggle labels. */
  toggle: {
    label: 'Billed',
    monthly: 'Monthly',
    annual: 'Annually',
  },

  /** Footnote shown on Pro card when monthly billing is selected. */
  proMonthlyPremiumNote: '+20% on monthly vs annual equivalent.',

  /** Pro card main pricing copy — used when currency is USD (default). */
  proAnnualLine:
    'From $1,188/yr ($99/mo equivalent), billed annually. Monthly available at $119/mo.',

  /** Pro card CTAs. */
  proCtas: {
    conversation: 'Start a conversation',
    brief: 'Download brief',
    conversationHref: '/admin/settings/billing/pro-contact',
    briefHref: '/pricing/mark8ly-pro-brief.pdf',
  },

  /** Pro+App add-on card. */
  proApp: {
    name: 'White-label App',
    tagline: 'Publish a branded iOS and Android app for your storefront.',
    requirement: 'Add-on — requires Studio plan or higher.',
    /** The $2,000 setup fee is a separate one-off charge, not part of the monthly price above. */
    setupFeeNote: 'Plus a $2,000 one-time setup.',
    cta: 'Add to plan',
    ctaHref: '/admin/settings/billing/pro-app-purchase',
  },

  /** Bottom disclosure footnote. Currency is interpolated at render time. */
  disclosureTemplate: (currency: string) =>
    `Prices shown in ${currency}. Annual billing bills upfront; monthly bills each month.`,

  /**
   * The three public plans. Backend uses starter / studio / pro.
   *
   * Every bullet must be something plangate actually grants on that plan.
   * #413 caught this page quoting prices the billing catalog would not
   * charge; #418 fixed the prices and left these feature bullets alone,
   * and they turned out to describe a different product entirely —
   * invented product/order/staff caps, wrong storefront counts in both
   * directions, and "dedicated infrastructure" / "SLA-backed uptime" on a
   * shared Cloud SQL micro instance (#564).
   *
   * Before adding a bullet: find its Feature constant in
   * services/marketplace-api/internal/plangate/matrix.go, confirm it is
   * enabled for that plan, and confirm a real handler enforces it. A
   * numeric limit must be a number the code enforces, not one invented
   * here. This page is public and indexed — pricing-copy.test.ts pins the
   * counts against the matrix so a divergence fails the build.
   */
  plans: [
    {
      id: 'starter' as const,
      name: 'Starter',
      tagline: 'For merchants opening their first store.',
      cta: 'Start free trial',
      ctaHref: '/signup?plan=starter',
      features: [
        'Up to 2 storefronts',
        'Unlimited products & orders',
        '15,000 campaign emails / mo',
        'Full colour palette & announcement bar',
        'Your own domain',
        'Read-only API',
      ],
    },
    {
      id: 'studio' as const,
      name: 'Studio',
      tagline: 'For stores gaining consistent monthly revenue.',
      cta: 'Start free trial',
      ctaHref: '/signup?plan=studio',
      features: [
        'Up to 5 storefronts',
        '50 images per product',
        '50,000 campaign emails / mo',
        'Custom CSS & fonts',
        'Read-only API',
        '12-month audit log',
      ],
    },
    {
      id: 'pro' as const,
      name: 'Pro',
      tagline: 'Built for teams scaling past $10k orders a month. Start a conversation.',
      cta: 'Start a conversation',
      ctaHref: '/admin/settings/billing/pro-contact',
      features: [
        'Up to 10 storefronts',
        'Unlimited images',
        'Full read/write API',
        'SSO (SAML / OIDC)',
        'Priority support (4h response)',
        'Forever audit retention',
      ],
    },
  ] satisfies PlanCopyItem[],
} as const

export type PricingCopy = typeof pricingCopy
