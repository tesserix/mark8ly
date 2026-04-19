/**
 * EmailRampVisualizer — horizontal timeline showing campaign email sending
 * limits during the 90-day trial (spec §5.3).
 *
 * Milestones (from subscriptionCopy.trial.emailRamp.milestones):
 *   Days 1–3:   500 campaign emails/day
 *   Days 4–7:   2,000 campaign emails/day
 *   Days 8–59:  Full plan allowance
 *   Days 60–90: Full plan allowance (no ramp cap)
 *
 * Current milestone: the last milestone whose daysFromSignup <= daysSinceSignup.
 *
 * Design (Paper · Ink · Moss):
 *   - Semantic <ol role="list"> with one <li> per milestone.
 *   - Current milestone: moss-700 left border (3px hairline) + "Currently" badge.
 *   - Past milestones: ink-400 text, no accent.
 *   - Future milestones: ink-700 text, dashed hairline top border.
 *   - No progress bar animation — hairline timeline only.
 *   - Source Serif 4 for milestone labels; Source Sans 3 for limits + prose.
 *   - No emoji. No urgency copy.
 *
 * Accessibility:
 *   - <ol role="list"> (removes implicit list semantics only when needed by CSS reset).
 *   - aria-current="step" on the active milestone <li>.
 *   - Intro paragraph + footnote are plain prose, no role needed.
 */
'use client'

import * as React from 'react'
import { subscriptionCopy } from '@/lib/copy/subscription'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface EmailRampVisualizerProps {
  daysSinceSignup: number
  className?: string
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const { emailRamp } = subscriptionCopy.trial

type Milestone = (typeof emailRamp.milestones)[number]

/**
 * Returns the index of the current milestone — the last milestone whose
 * daysFromSignup is <= daysSinceSignup.
 */
function resolveCurrentMilestoneIndex(daysSinceSignup: number): number {
  const milestones = emailRamp.milestones
  let current = 0
  for (let i = 0; i < milestones.length; i++) {
    if (milestones[i].daysFromSignup <= daysSinceSignup) {
      current = i
    }
  }
  return current
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface MilestoneItemProps {
  milestone: Milestone
  index: number
  currentIndex: number
}

function MilestoneItem({ milestone, index, currentIndex }: MilestoneItemProps) {
  const isCurrent = index === currentIndex
  const isPast = index < currentIndex
  // future = index > currentIndex

  return (
    <li
      aria-current={isCurrent ? 'step' : undefined}
      className={[
        'relative flex flex-col gap-1 pl-4 pb-6 last:pb-0',
        isCurrent
          ? 'border-l-[3px] border-l-[var(--moss-700)]'
          : 'border-l border-l-[var(--hairline)]',
      ].join(' ')}
    >
      {/* Future milestone: dashed top rule */}
      {!isCurrent && !isPast && (
        <span
          aria-hidden="true"
          className="absolute -top-px left-0 w-full border-t border-dashed border-[var(--hairline)]"
        />
      )}

      {/* Label row */}
      <div className="flex items-center gap-2">
        <span
          className={[
            'text-sm font-semibold leading-snug',
            isPast
              ? 'text-[var(--ink-400)]'
              : isCurrent
                ? 'text-[var(--ink-900)]'
                : 'text-[var(--ink-700)]',
          ].join(' ')}
          style={{ fontFamily: "'Source Serif 4', serif" }}
        >
          {milestone.label}
        </span>

        {isCurrent && (
          <span
            className="inline-flex items-center rounded-sm bg-[var(--moss-700)] px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-white"
            style={{ fontFamily: "'Source Sans 3', sans-serif" }}
          >
            {emailRamp.currentBadge}
          </span>
        )}
      </div>

      {/* Limit */}
      <span
        className={[
          'text-sm leading-snug',
          isPast ? 'text-[var(--ink-400)]' : 'text-[var(--ink-700)]',
        ].join(' ')}
        style={{ fontFamily: "'Source Sans 3', sans-serif" }}
      >
        {milestone.limit}
      </span>
    </li>
  )
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function EmailRampVisualizer({
  daysSinceSignup,
  className,
}: EmailRampVisualizerProps) {
  const currentIndex = resolveCurrentMilestoneIndex(daysSinceSignup)

  return (
    <section
      className={['space-y-4', className].filter(Boolean).join(' ')}
      aria-label={emailRamp.heading}
    >
      {/* Heading */}
      <h3
        className="text-sm font-semibold text-[var(--ink-900)]"
        style={{ fontFamily: "'Source Serif 4', serif" }}
      >
        {emailRamp.heading}
      </h3>

      {/* Intro */}
      <p
        className="text-sm leading-relaxed text-[var(--ink-700)]"
        style={{ fontFamily: "'Source Sans 3', sans-serif" }}
      >
        {emailRamp.intro}
      </p>

      {/* Timeline */}
      <ol role="list" className="mt-4 space-y-0">
        {emailRamp.milestones.map((milestone, index) => (
          <MilestoneItem
            key={milestone.daysFromSignup}
            milestone={milestone}
            index={index}
            currentIndex={currentIndex}
          />
        ))}
      </ol>

      {/* Footnote */}
      <p
        className="border-t border-[var(--hairline)] pt-3 text-xs leading-relaxed text-[var(--ink-400)]"
        style={{ fontFamily: "'Source Sans 3', sans-serif" }}
      >
        {emailRamp.footnote}
      </p>
    </section>
  )
}
