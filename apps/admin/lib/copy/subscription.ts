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
  /**
   * Admin shell banners — top-of-page notifications for billing states.
   *
   * Voice: calm, editorial, factual. Never urgency language, no exclamation
   * marks, no emoji.
   *
   * See plan §B (Editorial copy voice) for correct vs wrong examples.
   */
  banners: {
    failedPayment: {
      heading: "Payment didn't go through",
      bodyTemplate: (retryDate: string) =>
        `Your most recent payment did not go through. We'll retry on ${retryDate}.`,
      cta: 'Add a new card',
    },
    paymentActionRequired: {
      heading: 'Your bank needs to confirm this payment',
      bodyTemplate: (daysLeft: number) =>
        `Review the invoice to complete authentication. You have ${daysLeft} days before this affects your subscription.`,
      // T-14 / T-7 / T-1 tone escalation — still calm, just more specific.
      t14Body:
        'Your bank needs to confirm this payment. You have 14 days to complete authentication.',
      t7Body:
        'Your bank needs to confirm this payment. You have 7 days to complete authentication.',
      t1Body:
        'Your bank needs to confirm this payment. You have 1 day to complete authentication.',
      cta: 'Review the invoice',
    },
  },

  /**
   * Plan-change page — /settings/billing/plan-change
   *
   * Voice: calm, editorial, confident. Never urgency language.
   * Note: upgrade copy describes an immediate charge (factual);
   * downgrade copy describes a deferred switch (also factual).
   */
  planChange: {
    heading: 'Change plan',
    intro:
      'Upgrades take effect immediately and charge a prorated amount. Downgrades wait until your current period ends.',

    // Shown below the picker when an upgrade is selected.
    // `amount` is a pre-formatted currency string, e.g. "$12.00".
    upgradeProrationNote: (amount: string) =>
      `You'll be charged ${amount} now for the remainder of this period.`,

    // Shown below the picker when a downgrade is selected.
    // `effectiveAt` is a pre-formatted date string, e.g. "1 May 2026".
    downgradeScheduledNote: (effectiveAt: string) =>
      `You'll move to the new plan on ${effectiveAt}.`,

    // Shown when Pro monthly is selected — editorial note about pricing.
    proMonthlyPremiumNote:
      'Monthly Pro carries a 20% premium vs annual equivalent.',

    applyCta: 'Apply change',
    cancelCta: 'Cancel',

    toastSuccess: 'Plan changed.',
    toastScheduled: 'Plan change scheduled.',
    toastError: "Couldn't change plan. Please try again.",

    // Blocking decision messages
    blockReadOnly:
      'Your subscription is not currently eligible for a plan change.',
    blockCurrency:
      'The selected currency does not match your billing currency.',
    blockStoreCount: (count: number, limit: number) =>
      `You have ${count} active stores but the target plan allows ${limit}. Archive stores to proceed.`,

    // Period toggle labels
    periodMonthly: 'Monthly',
    periodAnnual: 'Annual',

    // Plan option labels
    currentPlanLabel: 'Current plan',

    // Period switch copy
    periodSwitchNote:
      'Switching to annual takes effect immediately and adjusts your next invoice.',

    // Loading state
    loadingPreflight: 'Calculating\u2026',
  },

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

  arbitrage: {
    banner: {
      heading: "We've noted a discrepancy",
      body: "Your registered tax jurisdiction and your recent customer addresses don't match. Review and respond to keep your store's pricing tier.",
      cta: 'Resolve',
    },
    appeal: {
      heading: 'Resolve arbitrage flag',
      intro:
        "Tell us your primary billing jurisdiction. We review appeals within 5 business days. You keep full access while the review is open.",
      jurisdictionLabel: 'Primary billing jurisdiction',
      jurisdictionHelp:
        'The country where your business is registered for tax purposes.',
      justificationLabel: 'Additional context (optional)',
      justificationHelp: 'Explain anything unusual about your customer distribution.',
      uploadLabel: 'Supporting document (optional)',
      uploadHelp:
        'A registration certificate, tax filing, or similar. PDF or PNG, up to 5 MB.',
      submitCta: 'Submit appeal',
      toastSubmitted: "Appeal submitted. We'll review it within 5 business days.",
      toastError: "Couldn't submit appeal. Please try again.",
      status: {
        pending:
          "Appeal under review \u2014 we'll get back to you within 5 business days.",
        resolved: 'Appeal resolved. Your jurisdiction has been updated.',
        rejected:
          'Appeal declined. A support message with details has been sent.',
      },
      errorNoOpenFlag:
        'No open arbitrage flag found \u2014 this store may already be clear.',
      uploadFileTooLarge: 'File must be under 5 MB.',
      uploadInvalidType: 'Only PDF or PNG files are accepted.',
    },
  },
} as const

export type SubscriptionCopy = typeof subscriptionCopy
