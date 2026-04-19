/**
 * ClockPauseBadge — inline badge shown alongside the Tax-ID form heading
 * when the 90-day trial clock is paused for one of two reasons:
 *
 *   registry_unavailable — external registry unreachable; auto-retry queued.
 *   sea_review           — submission in the manual SEA review queue.
 *
 * Design: info tone, never warning or danger. The pause is a protective
 * feature, not a problem state. Paper background, moss text, hairline border.
 *
 * Usage:
 *   <ClockPauseBadge reason="registry_unavailable" />
 *   <ClockPauseBadge reason="sea_review" />
 */
import type { ClockPauseReason } from '@/lib/api/subscription/schemas/taxId'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ClockPauseBadgeProps {
  reason: ClockPauseReason
  className?: string
}

// ---------------------------------------------------------------------------
// Label map — editorial, descriptive, no urgency
// ---------------------------------------------------------------------------

const BADGE_LABELS: Record<ClockPauseReason, string> = {
  registry_unavailable: 'Clock paused: registry',
  sea_review: 'Clock paused: review queue',
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ClockPauseBadge({ reason, className }: ClockPauseBadgeProps) {
  const label = BADGE_LABELS[reason]

  return (
    <span
      role="status"
      aria-label={label}
      className={[
        // Layout
        'inline-flex items-center gap-1.5',
        // Sizing + shape
        'rounded-[4px] px-2.5 py-1',
        // Info tone — moss text on paper background, hairline border
        'border border-[var(--moss-700)] bg-[var(--paper-200)]',
        'text-xs font-medium text-[var(--moss-700)]',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
      style={{ fontFamily: "'Source Sans 3', sans-serif" }}
    >
      {/* Pause indicator — small circle, moss fill, info tone */}
      <span
        aria-hidden="true"
        className="inline-block h-1.5 w-1.5 rounded-full bg-[var(--moss-700)]"
      />
      {label}
    </span>
  )
}
