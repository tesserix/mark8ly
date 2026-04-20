/**
 * DunningEmailPreview — CSM-gated internal tool.
 *
 * Renders the dunning email that will be sent to a merchant based on how
 * many days their payment has been overdue. Lets CSMs preview the exact
 * template without triggering a send.
 *
 * Gate: visible only to Tesserix platform super-admins (isPlatformOwner).
 * Returns null silently for merchant sessions — never expose this control
 * to the merchants it describes.
 *
 * Templates:
 *   T+1  (daysOverdue <= 1)  — "A note about your payment"
 *   T+3  (daysOverdue <= 3)  — "Your most recent payment"
 *   T+7  (daysOverdue <= 7)  — "Your account is on pause"
 *   T+14 (daysOverdue > 7)   — "Final notice before your account moves to read-only"
 */
'use client'

import { useIsPlatformOwner } from '@/lib/auth/useIsPlatformOwner'
import { subscriptionCopy } from '@/lib/copy/subscription'

const copy = subscriptionCopy.dunningPreview

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface DunningEmailPreviewProps {
  /** The store this email would be sent to — used for display context. */
  storeId: string
  /**
   * How many days past due the subscription is. Determines which template
   * tier is selected:
   *   1 → T+1, 2–3 → T+3, 4–7 → T+7, 8+ → T+14
   */
  daysOverdue: number
  /**
   * Pre-formatted retry date string shown in the email body.
   * Defaults to a placeholder when not provided.
   */
  retryDate?: string
}

// ---------------------------------------------------------------------------
// Template selection
// ---------------------------------------------------------------------------

type DunningTier = 't1' | 't3' | 't7' | 't14'

function selectTier(daysOverdue: number): DunningTier {
  if (daysOverdue <= 1) return 't1'
  if (daysOverdue <= 3) return 't3'
  if (daysOverdue <= 7) return 't7'
  return 't14'
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function DunningEmailPreview({
  storeId,
  daysOverdue,
  retryDate = '[retry date]',
}: DunningEmailPreviewProps) {
  const isPlatformOwner = useIsPlatformOwner()

  if (!isPlatformOwner) return null

  const tier = selectTier(daysOverdue)
  const template = copy[tier]
  const subject = template.subject
  const body = template.body(retryDate)

  return (
    <section
      aria-labelledby="dunning-preview-heading"
      className="space-y-4"
      data-testid="dunning-email-preview"
    >
      <h3
        id="dunning-preview-heading"
        className="font-serif text-base font-medium text-[var(--ink-900)]"
      >
        Email preview — {tier.toUpperCase()}
        <span className="ml-2 text-xs font-normal text-[var(--ink-500)]">
          store: {storeId}
        </span>
      </h3>

      {/* Subject line */}
      <div className="space-y-1">
        <span className="text-xs font-medium uppercase tracking-wide text-[var(--ink-500)]">
          {copy.subjectLabel}
        </span>
        <p
          className="text-sm font-medium text-[var(--ink-900)]"
          data-testid="dunning-subject"
        >
          {subject}
        </p>
      </div>

      {/* Body preview */}
      <div
        className="border border-[var(--hairline,var(--ink-100))] px-5 py-4"
        aria-label="Email body preview"
      >
        <p
          className="whitespace-pre-line text-sm leading-6 text-[var(--ink-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
          data-testid="dunning-body"
        >
          {body}
        </p>
      </div>

      <p className="text-xs text-[var(--ink-400)]">
        Internal preview. This email has not been sent.
      </p>
    </section>
  )
}
