/**
 * Cancellation flow copy — single source of truth for all strings in the
 * cancel wizard (confirm → survey → save-offer → final).
 *
 * Voice: calm, editorial, confident. Never guilt-tripping. Never urgency.
 * No emoji. No exclamation marks as hooks.
 *
 * Save-offer copy (saveOfferStep) is verbatim from spec §15.1 / plan Task 10
 * Step 1 — do not paraphrase.
 *
 * Eligibility rule (§15.1): show the save offer only when
 *   reason ∈ { 'too_expensive', 'missing_features' }
 * Skip it for 'going_out_of_business' and 'switching_to'.
 * For all other reasons, show the offer.
 * See SAVE_OFFER_ELIGIBLE_REASONS below.
 */

export const SAVE_OFFER_ELIGIBLE_REASONS = new Set([
  'too_expensive',
  'missing_features',
  'technical_issues',
  'other',
])

export const cancellationCopy = {
  confirmStep: {
    title: 'Cancel your subscription',
    body: "Your store will stay live until the end of your current billing period. You'll keep full access until then.",
    continueCta: 'Continue',
    keepCta: 'Keep my subscription',
  },

  surveyStep: {
    title: 'Before you go',
    body: "Tell us what didn't work. Two questions, then we'll get you on your way.",
    reasons: [
      { code: 'too_expensive', label: "It's too expensive for me right now" },
      { code: 'missing_features', label: "It's missing features I need" },
      { code: 'switching_to', label: "I'm switching to another platform" },
      { code: 'going_out_of_business', label: "I'm closing my business" },
      { code: 'technical_issues', label: 'I had technical problems' },
      { code: 'other', label: 'Something else' },
    ],
    feedbackLabel: "Anything you want us to hear? (optional)",
    feedbackPlaceholder: 'Anything you want us to hear? (optional)',
    submitCta: 'Submit',
  },

  // §15.1 VERBATIM — do not edit (plan Task 10 Step 1 canonical copy).
  saveOfferStep: {
    title: 'Before you cancel',
    body: `We get it \u2014 Mark8ly isn\u2019t for everyone. Before you go, would one of these make a difference?`,
    acceptCta: 'Take this offer',
    declineCta: 'No thanks, cancel my plan',
  },

  finalStep: {
    title: 'Your subscription is scheduled to end',
    body: (date: string) =>
      `Your plan ends on ${date}. You'll keep full access until then. If you change your mind, come back any time.`,
    revertCta: 'Keep my subscription after all',
    doneCta: 'Back to billing',
  },

  // Toast messages
  toastCancelled: 'Subscription cancelled.',
  toastReverted: 'Subscription restored.',
  toastError: "Couldn't process cancellation. Please try again.",

  // Loading / processing
  processing: 'Processing\u2026',
  retryLabel: 'Try again',
} as const

export type CancellationCopy = typeof cancellationCopy
