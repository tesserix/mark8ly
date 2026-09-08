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

/**
 * Where the marketing site lives, for the self-serve signup CTAs.
 *
 * Those CTAs cross a HOST boundary, which is why they cannot be relative.
 * This page is part of `apps/admin`, served at `admin.mark8ly.com` and
 * `{tenant}-admin.mark8ly.com`; signup lives in `apps/onboarding`, served at
 * `mark8ly.com`. A relative `/onboarding` resolves against the admin host and
 * 404s — which is the class of bug #849 was opened for.
 *
 * `||`, not `??`, and that distinction is load-bearing here. The Docker build
 * declares `ARG REUSABLE_PUBLIC_BUILD_ARG_6=""`, so an unset CI secret arrives
 * as the EMPTY STRING rather than as undefined. `??` only falls back on
 * null/undefined, so it would leave `MARKETING_URL` empty and every signup CTA
 * would render as a bare `/onboarding` — relative, on the wrong host, 404.
 * `SignInForm.tsx` uses `??` for the same variable and has that latent bug;
 * see #849 for the note.
 *
 * The fallback is the real production host rather than a localhost default,
 * because this string ends up in a PUBLIC, indexed page: if the build arg is
 * ever missing, a link to the live marketing site is a far better failure than
 * a link to someone's laptop.
 */
const MARKETING_URL = (
  process.env.NEXT_PUBLIC_MARKETING_URL || 'https://mark8ly.com'
).replace(/\/+$/, '')

/**
 * The self-serve signup entry, absolute.
 *
 * No `?plan=` query. The previous hrefs carried `?plan=starter` and
 * `?plan=studio`, and NOTHING reads them — `apps/onboarding` never looks at a
 * `plan` param, and by design cannot: the trial "starts with just an email and
 * doesn't ask for a card. You'll only be asked to choose a plan once the trial
 * is ending" (`apps/onboarding/app/help/page.tsx`). A parameter the receiving
 * app ignores is a URL making a promise the product does not keep, which is
 * the same failure mode as the dead links themselves.
 */
const SIGNUP_HREF = `${MARKETING_URL}/onboarding`

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
    conversationHref: '/settings/billing/pro-contact',
  },

  /** Pro+App add-on card. */
  proApp: {
    name: 'White-label App',
    tagline: 'Publish a branded iOS and Android app for your storefront.',
    requirement: 'Add-on — requires Studio plan or higher.',
    /** The $2,000 setup fee is a separate one-off charge, not part of the monthly price above. */
    setupFeeNote: 'Plus a $2,000 one-time setup.',
    cta: 'Add to plan',
    ctaHref: '/settings/billing/pro-app-purchase',
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
   * here.
   *
   * This page is public and indexed, and
   * `tests/unit/lib/copy/pricing-copy.test.ts` now pins these bullets
   * against `matrix.go` so a divergence fails the build.
   *
   * That sentence used to be here and was NOT true: the file it named did
   * not exist, and nothing on this side compared a bullet to the matrix.
   * It is recorded rather than quietly corrected because of what it cost —
   * three defects reached this public page while the comment told reviewers
   * CI was watching: #413 (prices billing would not charge), #564 (the
   * invented caps and SLA above) and #838 ("SSO (SAML / OIDC)" against a
   * route that answers 501). Each was found by a person reading the file.
   * If the guard is ever removed, delete this paragraph with it rather than
   * leaving the claim behind again.
   */
  plans: [
    {
      id: 'starter' as const,
      name: 'Starter',
      tagline: 'For merchants opening their first store.',
      cta: 'Start free trial',
      ctaHref: SIGNUP_HREF,
      features: [
        'Up to 2 storefronts',
        'Unlimited products & orders',
        '15,000 campaign emails / mo',
        'Full colour palette & announcement bar',
        'Your own domain',
        'Read-only API',
        '10 webhook endpoints',
      ],
    },
    {
      id: 'studio' as const,
      name: 'Studio',
      tagline: 'For stores gaining consistent monthly revenue.',
      cta: 'Start free trial',
      ctaHref: SIGNUP_HREF,
      features: [
        'Up to 5 storefronts',
        '50 images per product',
        '50,000 campaign emails / mo',
        'Custom CSS & fonts',
        'Read-only API',
        '25 webhook endpoints',
        '12-month audit log',
      ],
    },
    {
      id: 'pro' as const,
      name: 'Pro',
      tagline: 'Built for teams scaling past $10k orders a month. Start a conversation.',
      cta: 'Start a conversation',
      ctaHref: '/settings/billing/pro-contact',
      features: [
        'Up to 10 storefronts',
        'Unlimited images',
        'Full read/write API',
        '100 webhook endpoints',
        // SAML is not implemented — both SSO routes answer 501 for it and
        // there is no SP (mark8ly#820). Advertising it sold something
        // that cannot be delivered; OIDC is real and configurable at
        // Settings -> Single sign-on.
        'SSO (OpenID Connect)',
        'Priority support (4h response)',
        'Forever audit retention',
      ],
    },
  ] satisfies PlanCopyItem[],
} as const

export type PricingCopy = typeof pricingCopy
