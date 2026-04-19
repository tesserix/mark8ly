/**
 * Pricing page copy — single source of truth for all editorial strings.
 *
 * Voice: calm, editorial, confident. Never urgency. Never "Hey there!".
 * See plan lines 289–300 for the correct/wrong examples table.
 *
 * All strings live here so a reviewer can catch voice drift in one file.
 */

export interface PlanCopyItem {
  id: 'starter' | 'growth' | 'scale' | 'pro'
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
    'Four plans. Clear limits. No surprise fees. Change plans any time — upgrades prorate, downgrades wait for the period to close.',

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
    requirement: 'Add-on — requires Growth plan or higher.',
    cta: 'Add to plan',
    ctaHref: '/admin/settings/billing/pro-app-purchase',
  },

  /** Bottom disclosure footnote. Currency is interpolated at render time. */
  disclosureTemplate: (currency: string) =>
    `Prices shown in ${currency}. Annual billing bills upfront; monthly bills each month.`,

  /** The four main plans. */
  plans: [
    {
      id: 'starter' as const,
      name: 'Starter',
      tagline: 'For merchants opening their first store.',
      cta: 'Start free trial',
      ctaHref: '/signup?plan=starter',
      features: [
        '1 storefront',
        'Up to 100 products',
        '500 orders / month',
        '2 staff accounts',
        'Standard analytics',
        'Community support',
      ],
    },
    {
      id: 'growth' as const,
      name: 'Growth',
      tagline: 'For stores gaining consistent monthly revenue.',
      cta: 'Start free trial',
      ctaHref: '/signup?plan=growth',
      features: [
        '3 storefronts',
        'Unlimited products',
        '5,000 orders / month',
        '10 staff accounts',
        'Advanced analytics',
        'Priority email support',
        'Custom domain',
        'Discount codes',
      ],
    },
    {
      id: 'scale' as const,
      name: 'Scale',
      tagline: 'For operations managing multiple channels at volume.',
      cta: 'Start free trial',
      ctaHref: '/signup?plan=scale',
      features: [
        '10 storefronts',
        'Unlimited products',
        '50,000 orders / month',
        'Unlimited staff',
        'Full analytics suite',
        'Dedicated onboarding',
        'API access',
        'Vendor management',
        'Multi-currency',
      ],
    },
    {
      id: 'pro' as const,
      name: 'Pro',
      tagline: 'Built for teams scaling past $10k orders a month. Start a conversation.',
      cta: 'Start a conversation',
      ctaHref: '/admin/settings/billing/pro-contact',
      features: [
        'Unlimited storefronts',
        'Unlimited everything',
        'Dedicated infrastructure',
        'SLA-backed uptime',
        'Custom contract',
        'Named account manager',
        'Enterprise SSO',
        'Audit log retention',
        'Advanced fraud tooling',
      ],
    },
  ] satisfies PlanCopyItem[],
} as const

export type PricingCopy = typeof pricingCopy
