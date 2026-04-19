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
    cancelScheduledNote: (periodEnd: string) =>
      `Cancellation scheduled \u2014 full access continues until ${periodEnd}.`,
    restoreCta: 'Restore subscription',
    toastRestored: 'Subscription restored.',
    toastRestoreError: "Couldn't restore. Please try again.",
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

  /**
   * Trial banners + email ramp visualizer (§5, §5.3).
   *
   * Voice: calm, editorial. Trials are a positive state — never urgency.
   * No "!", no "Hurry", no "Don't lose access".
   * Correct: "Your trial ends 18 April. Add a payment method to continue."
   * Wrong:   "Trial ending soon! Don't lose access!"
   */
  trial: {
    banner: {
      day60Heading: '30 days left in your trial',
      day60BodyTemplate: (endsAt: string) =>
        `Your trial ends ${endsAt}. Add a payment method to keep your store running without interruption.`,
      day75Heading: '2 weeks left in your trial',
      day75BodyTemplate: (endsAt: string) =>
        `Your trial ends ${endsAt}. Add a payment method to continue without interruption.`,
      day85Heading: '5 days left in your trial',
      day85BodyTemplate: (endsAt: string) =>
        `Your trial ends ${endsAt}. Add a payment method today to avoid losing access.`,
      cta: 'Add a payment method',
    },
    /**
     * Email ramp timeline visualizer (§5.3).
     *
     * Spec §5.3 defines the operational ramp (Days 1–3: 500/day, Days 4–7:
     * 2,000/day, Days 8+: full allowance). The visualizer surfaces this as
     * four UI milestones grouped by meaningful trial phases, using broader
     * daysFromSignup thresholds (0, 14, 30, 60) for clear visual progression.
     * The limit text references the spec-defined caps.
     *
     * daysFromSignup marks the inclusive start of each window and is used by
     * EmailRampVisualizer to determine which milestone is current.
     */
    emailRamp: {
      heading: 'Email sending limits during your trial',
      intro:
        'We raise your sending limit as you progress through the trial. This protects your domain reputation and our deliverability.',
      milestones: [
        { daysFromSignup: 0, label: 'Days 1\u201314', limit: '100 outbound/day' },
        { daysFromSignup: 14, label: 'Days 15\u201330', limit: '500 outbound/day' },
        { daysFromSignup: 30, label: 'Days 31\u201360', limit: '2,000 outbound/day' },
        {
          daysFromSignup: 60,
          label: 'Days 61\u201390',
          limit: 'Unlimited (subject to abuse review)',
        },
      ] as const,
      currentBadge: 'Currently',
      footnote:
        'After signup, sending limits take effect within 24 hours of each milestone. Upgraded plans lift limits immediately.',
    },
  },

  /**
   * Store-close-before-downgrade flow — /admin/stores/close-before-downgrade
   *
   * Voice: editorial, calm, matter-of-fact. Frame as a choice, not an alarm.
   * Never: "URGENT", "WARNING", exclamation marks, or urgency language.
   */
  storeDowngrade: {
    heading: 'Choose what happens to your over-cap stores',
    introTemplate: (target: string, limit: number, overBy: number) =>
      `The ${target} plan includes ${limit} stores — you have ${overBy} over your cap. Choose what happens to each. Your plan won't change until you resolve every over-cap store.`,
    closeLabel: 'Close',
    closeHelp:
      'Freezes the storefront behind a closed page. Keeps your slot — reopen anytime by upgrading.',
    deleteLabel: 'Delete',
    deleteHelp:
      'Removes the store and frees the slot. 60-day soft-delete grace — restore from support during that window.',
    inFlightOrdersTemplate: (n: number) =>
      n === 0
        ? 'No in-flight orders.'
        : `${n} order${n === 1 ? '' : 's'} in flight — export before closing or deleting.`,
    exportCsvCta: 'Export orders CSV',
    confirmHeading: 'Confirm your choices',
    confirmCta: 'Apply changes and downgrade',
    toastSuccess: 'Stores updated. Downgrade scheduled.',
    toastError: "Couldn't apply your choices. Try each store individually.",
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

  taxId: {
    heading: 'Tax registration',
    intro:
      'We verify your tax ID with the registry of your business country. Verified stores get proper tax treatment on invoices.',
    businessNameLabel: 'Legal business name',
    countryLabel: 'Country of registration',
    taxIdLabel: 'Tax ID',
    taxIdHelp:
      'Format depends on your country \u2014 for the UK this is your VRN; for India it\u2019s your GSTIN.',
    billingAddressLabel: 'Billing address (optional)',
    billingAddressHelp:
      'Some registries require an address match. Leave blank if unsure.',
    submitCta: 'Submit for verification',
    updateCta: 'Update tax ID',
    toastSubmitted: 'Tax ID submitted for verification.',
    toastError: "Couldn\u2019t submit. Check the format and try again.",
    status: {
      validatedHeading: 'Tax ID verified',
      validatedBody:
        'Your tax ID is verified and will appear on customer invoices.',
      pausedRegistryHeading: 'Verification paused \u2014 registry unavailable',
      pausedRegistryBody:
        "The registry for your country isn\u2019t responding right now. We\u2019ll retry automatically; your 90-day clock is paused during this time.",
      pausedSeaHeading: 'Under manual review',
      pausedSeaBody:
        'Your submission is in our manual review queue. Most cases clear within 2 business days. Your clock is paused during review.',
      rejectedHeading: 'Verification failed',
      rejectedBodyTemplate: (code: string) =>
        `We couldn\u2019t verify your tax ID (${code}). Check the format and resubmit.`,
    },
  },

  attestation: {
    heading: 'Business entity attestation',
    intro:
      'Sole proprietors and individuals selling in the US or Canada must attest that their store is operated by a registered business entity before we can process customer payments for tangible goods over certain thresholds.',
    countryLabel: 'Jurisdiction',
    checkboxLabel:
      'I attest that this store is operated by a registered business entity in my selected jurisdiction.',
    /**
     * checkbox_version token sent to the backend. Increment when the
     * checkboxLabel copy changes so audit rows can be traced to the exact
     * wording the merchant acknowledged.
     */
    checkboxVersion: 'v1' as const,
    submitCta: 'Sign attestation',
    toastSigned: 'Attestation signed.',
    toastError: "Couldn\u2019t save your attestation. Try again.",
    signedHeading: 'Attestation on file',
    signedBodyTemplate: (acceptedAt: string) =>
      `You signed this attestation on ${acceptedAt}. Contact support to update or revoke.`,
    supportHint:
      'To update or revoke this attestation, contact our support team.',
  },
} as const

export type SubscriptionCopy = typeof subscriptionCopy
