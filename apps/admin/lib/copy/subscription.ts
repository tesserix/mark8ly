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

    // ─── Setup / first-run ──────────────────────────────────────────────
    setupHeading: 'Set up billing',
    setupDescription:
      'Finish setting up your billing account to see your plan, payment method, and invoices.',
    setupCta: 'Set up billing',
    setupInProgress: 'Setting up\u2026',

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
      'Freezes the storefront and shows a closed page to visitors. Your store slot is preserved — reopen it at any time by upgrading your plan.',
    deleteLabel: 'Delete',
    deleteHelp:
      'Permanently removes the store and frees the slot on your plan. Your data is kept for 60 days — contact support to restore within that window.',
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

  /**
   * Dunning email preview — CSM-only internal tool.
   *
   * T+1 / T+3 / T+7 / T+14 templates. Voice: editorial, calm, factual.
   * Never urgency language, no exclamation marks, no emoji.
   */
  dunningPreview: {
    subjectLabel: 'Subject',
    t1: {
      subject: 'A note about your payment',
      body: (retryDate: string) =>
        `Your most recent payment did not go through. We will retry your card on ${retryDate}.\n\nIf you would prefer to use a different card, you can add a new one from your billing settings. No action is needed if you would like to wait for the retry.\n\nThank you for being a Mark8ly merchant.`,
    },
    t3: {
      subject: 'Your most recent payment',
      body: (retryDate: string) =>
        `We have been unable to collect payment for your subscription. Our next retry is scheduled for ${retryDate}.\n\nTo avoid any interruption to your store, we recommend adding a new card from your billing settings before that date.\n\nIf you have already updated your payment method, no further action is needed.`,
    },
    t7: {
      subject: 'Your account is on pause',
      body: (retryDate: string) =>
        `Your subscription is currently on hold following several unsuccessful payment attempts. Your store is not visible to customers at this time.\n\nWe will retry your card on ${retryDate}. Adding a new payment method before that date will restore access promptly.\n\nIf you believe this is an error, please contact our support team.`,
    },
    t14: {
      subject: 'Final notice before your account moves to read-only',
      body: (retryDate: string) =>
        `This is a final notice regarding your unpaid subscription balance.\n\nIf we are unable to collect payment on or before ${retryDate}, your account will move to read-only status. You will retain access to your data, but your storefront will remain closed to customers until the balance is settled.\n\nTo restore full access, please add a new payment method from your billing settings or contact us directly.`,
    },
  },

  /**
   * White-label app lifecycle widget copy.
   *
   * Voice: calm, editorial, factual. State labels and CTAs only.
   */
  appLifecycle: {
    heading: 'App build status',
    platformApple: 'App Store (iOS)',
    platformGoogle: 'Google Play (Android)',
    states: {
      pending_credentials: 'Awaiting credentials',
      building: 'Building',
      submitted_apple: 'Submitted for review',
      submitted_google: 'Submitted for review',
      live_apple: 'Live',
      live_google: 'Live',
      live_both: 'Live',
      update_needed: 'Update needed',
      paused: 'Paused',
    } as Record<string, string>,
    ctaUploadCredentials: 'Upload credentials',
    ctaBuildNewVersion: 'Build new version',
    lastUpdatedLabel: 'Last updated',
    noAddon: null,
    updateNeededNote:
      'A new build is required. Submit updated credentials to continue.',
  },

  /**
   * Pro contact-sales form — /admin/settings/billing/pro-contact
   *
   * Voice: calm, editorial, confident. Never urgency language.
   * Correct: "Built for teams scaling past $10k orders a month. Start a conversation."
   * Wrong:   "Unlock unlimited power! Book a demo!"
   *
   * See plan §B (Editorial copy voice) for the full correct/wrong examples table.
   */
  proContact: {
    heading: 'Start a Pro conversation',
    intro:
      'Pro is built for teams scaling past $10k orders a month. Tell us a bit about your store and we\'ll get back to you within 1 business day.',
    labels: {
      companyName: 'Company name',
      contactName: 'Your name',
      contactEmail: 'Work email',
      phone: 'Phone (optional)',
      monthlyOrders: 'Monthly orders',
      currentPlatform: 'Current platform (optional)',
      migrationTimeline: 'When are you looking to move?',
      notes: 'Anything else we should know? (optional)',
    },
    monthlyOrdersHelp:
      'Approximate orders across all stores in a typical month.',
    timelineOptions: [
      { value: 'this_quarter' as const, label: 'This quarter' },
      { value: 'next_quarter' as const, label: 'Next quarter' },
      { value: 'h2' as const, label: 'Second half of the year' },
      { value: 'exploring' as const, label: 'Still exploring' },
    ],
    submitCta: 'Start a conversation',
    submittingCta: 'Sending\u2026',
    toastSuccess:
      "Thanks \u2014 we'll get back to you within 1 business day.",
    toastMailtoFallback:
      "We'll open your email client so you can send us a message directly.",
    toastError:
      "Couldn't send right now. Please email pro-sales@mark8ly.com.",
    salesEmail: 'pro-sales@mark8ly.com',
  },

  /**
   * Pro+App purchase flow — /settings/billing/pro-app-purchase
   *
   * Voice: confident, editorial. The add-on is a positive step.
   * Explain the Apple 4.2.6 attestation factually — not alarmingly.
   */
  proApp: {
    purchase: {
      heading: 'Add the White-label App',
      intro:
        'Publish a branded iOS and Android app for your storefront. You\u2019ll provide Apple and Google credentials in the next step.',
      /**
       * Apple App Store guideline 4.2.6 acknowledgement label.
       * Shown as a checkbox the merchant must tick before submitting.
       */
      apple426Label:
        'I acknowledge that my app will be built on Mark8ly\u2019s native technology and listed under my own App Store Connect account, in accordance with Apple App Store guideline 4.2.6.',
      /** Attestation text sent verbatim to the backend `attestation_text` field. */
      apple426AttestationText:
        'Merchant acknowledged Apple App Store guideline 4.2.6 at purchase time.',
      prorationNote: (today: string, renewal: string) =>
        `${today} today, pro-rated to co-terminate with your annual Pro renewal on ${renewal}.`,
      nextRenewalNote: (amount: string, date: string) =>
        `Next full-year charge: ${amount} on ${date}.`,
      submitCta: 'Add to plan',
      toastPurchased: 'Welcome to Mark8ly White-label App.',
      /**
       * Shown when Stripe returns a hosted invoice URL for SCA/3DS.
       * The URL is opened in a new tab.
       */
      toastPaymentAction:
        'Complete payment authentication in your bank, then upload credentials.',
      toastError: "Couldn\u2019t complete the purchase. Please try again.",
      alreadyActiveHeading: 'White-label App is active on your plan',
      alreadyActiveBody:
        'Your add-on is live \u2014 upload or update your Apple and Google credentials to publish updates.',
      alreadyActiveCta: 'Manage credentials',
      needsProHeading: 'Upgrade to Pro to add the White-label App',
      needsProBody:
        'The White-label App is a Pro-plan add-on. Upgrade to Pro to continue.',
      needsProCta: 'View plans',
    },
  },

  /**
   * App credential upload — /settings/billing/pro-app-purchase/credentials
   *
   * Voice: editorial, calm, factual. Security information is stated
   * matter-of-factly — never alarming or defensive.
   */
  appCredentials: {
    heading: 'App Store credentials',
    intro:
      'Both sets of credentials are required before your app can be published. They\u2019re stored encrypted in Google Secret Manager and never leave your browser as plaintext after upload.',
    apple: {
      heading: 'Apple App Store Connect',
      help: "You\u2019ll need an App Store Connect API key. See the setup guide for step-by-step instructions.",
      keyIdLabel: 'Key ID',
      issuerIdLabel: 'Issuer ID',
      p8FileLabel: 'Private key (.p8 file)',
      submitCta: 'Save Apple credentials',
      uploadedTemplate: (at: string) => `Uploaded ${at}`,
      replaceCta: 'Replace',
    },
    google: {
      heading: 'Google Play Console',
      help: 'Upload your Google Cloud service account JSON with Play Console access.',
      serviceAccountLabel: 'Service account JSON file',
      submitCta: 'Save Google credentials',
      invalidJsonError: "That doesn\u2019t look like a valid service account JSON.",
    },
    toastUploaded: 'Credentials saved.',
    toastError: "Couldn\u2019t save credentials. Please check the file and try again.",
    supportHint: 'Having trouble?',
    supportCta: 'Contact support',
  },
} as const

export type SubscriptionCopy = typeof subscriptionCopy
