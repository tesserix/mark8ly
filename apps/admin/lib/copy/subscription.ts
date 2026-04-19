/**
 * Subscription billing copy — single source of truth for all billing
 * panel strings in the admin app.
 *
 * Voice: calm, editorial, confident. Never urgency language.
 * All strings centralised here so a reviewer can catch voice drift in one file.
 *
 * See plan §B (Editorial copy voice) for correct vs wrong examples.
 */

export const subscriptionCopy = {
  billing: {
    heading: 'Billing',
    description: 'Your plan, payment method, and invoices.',

    // ─── Plan card ──────────────────────────────────────────────────────
    planCardHeading: 'Current plan',
    planCardDescription: 'Your active subscription and billing period.',
    renewsOn: (date: string) => `Renews on ${date}`,
    trialEnds: (date: string) => `Trial ends ${date}`,
    cancelScheduled: 'Your subscription will end at the close of the current period.',
    changePlan: 'Change plan',
    cancelSubscription: 'Cancel subscription',
    contactSales: 'Contact sales',

    // ─── Invoices panel ─────────────────────────────────────────────────
    invoicesHeading: 'Invoices and receipts',
    invoicesDescription:
      'Open the Stripe billing portal to download invoices, update payment methods, or view receipts.',
    openPortalCta: 'Open billing portal',
    openingPortal: 'Opening portal\u2026',

    // ─── Payment method panel ────────────────────────────────────────────
    paymentMethodHeading: 'Payment method',
    paymentMethodDescription: 'The card charged at each renewal.',
    cardEnding: (brand: string, last4: string) =>
      `${brand.charAt(0).toUpperCase()}${brand.slice(1)} ending in \u2022\u2022\u2022\u2022 ${last4}`,
    renewsAutomatically: (date: string) =>
      `Renewed automatically on ${date}`,
    updateCard: 'Update card',
    noPaymentMethod: 'No payment method on file.',
    addPaymentMethod: 'Add payment method',

    // ─── White-label app panel ────────────────────────────────────────────
    whiteLabelAppHeading: 'White-label app',
    whiteLabelAppActive:
      'Your branded iOS and Android app is active.',
    whiteLabelAppCta: 'Manage app credentials',
    whiteLabelAppUpsell:
      'Add the white-label app to publish iOS and Android builds of your storefront.',
    whiteLabelAppAddCta: 'Add white-label app',

    // ─── Error / loading states ─────────────────────────────────────────
    loadingError: 'Something went wrong loading your plan.',
    retryLabel: 'Retry',
    inactiveRedirect:
      'Your subscription needs attention. Opening billing\u2026',
    inactiveBanner: (status: string) =>
      `This workspace is ${status}. Resolve billing to continue.`,

    // ─── Skeleton aria ───────────────────────────────────────────────────
    loadingAriaLabel: 'Loading billing information',
  },
} as const

export type SubscriptionCopy = typeof subscriptionCopy
