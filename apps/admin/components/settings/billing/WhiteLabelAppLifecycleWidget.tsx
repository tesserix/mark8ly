/**
 * WhiteLabelAppLifecycleWidget — compact per-platform build/publish status.
 *
 * Shown inside WhiteLabelAppCard when the merchant has the white_label_app
 * add-on. Reads lifecycle_state from useCurrentPlan and renders one status
 * row per relevant platform (Apple / Google).
 *
 * Lifecycle states:
 *   pending_credentials  → "Awaiting credentials"  + Upload credentials CTA
 *   building             → "Building"               (no CTA)
 *   submitted_apple      → "Submitted for review"   (Apple only)
 *   submitted_google     → "Submitted for review"   (Google only)
 *   live_apple           → "Live"                   (Apple; moss accent)
 *   live_google          → "Live"                   (Google; moss accent)
 *   live_both            → "Live"                   (both; moss accent)
 *   update_needed        → "Update needed"          + Build new version CTA (disabled TODO)
 *   paused               → "Paused"                 (no CTA)
 *
 * Design rules (per impeccable.md):
 *   - No icons. Moss-700 hairline accent on "live". Ink-400 on pending.
 *   - Warning border on "update_needed". Hairline rules, no bordered cards.
 *   - Source Sans 3 for all body text.
 *
 * Returns null when:
 *   - Plan does not have the white_label_app add-on.
 *   - Data is still loading.
 */
'use client'

import Link from 'next/link'
import { useCurrentPlan } from '@/lib/api/subscription/hooks/useBilling'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { formatBillingDate } from '@/lib/format/date'
import type { WhiteLabelAppLifecycleState } from '@/lib/api/subscription/schemas/billing'

const copy = subscriptionCopy.appLifecycle

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface WhiteLabelAppLifecycleWidgetProps {
  storeId: string
}

type Platform = 'apple' | 'google'

interface PlatformRowProps {
  platform: Platform
  state: WhiteLabelAppLifecycleState | null
  updatedAt: string | null
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function isLive(state: WhiteLabelAppLifecycleState | null, platform: Platform): boolean {
  if (!state) return false
  if (state === 'live_both') return true
  return platform === 'apple' ? state === 'live_apple' : state === 'live_google'
}

function isAppleRelevant(state: WhiteLabelAppLifecycleState | null): boolean {
  if (!state) return true
  return state !== 'submitted_google' && state !== 'live_google'
}

function isGoogleRelevant(state: WhiteLabelAppLifecycleState | null): boolean {
  if (!state) return true
  return state !== 'submitted_apple' && state !== 'live_apple'
}

function resolveStateLabel(
  state: WhiteLabelAppLifecycleState | null,
  platform: Platform,
): string {
  if (!state) return copy.states['pending_credentials'] ?? 'Awaiting credentials'

  // For live_both, both rows show "Live"
  if (state === 'live_both') return copy.states['live_both'] ?? 'Live'

  // Platform-specific live states
  if (platform === 'apple' && state === 'live_apple') return copy.states['live_apple'] ?? 'Live'
  if (platform === 'google' && state === 'live_google') return copy.states['live_google'] ?? 'Live'

  return copy.states[state] ?? state
}

// ---------------------------------------------------------------------------
// PlatformRow sub-component
// ---------------------------------------------------------------------------

function PlatformRow({ platform, state, updatedAt }: PlatformRowProps) {
  const live = isLive(state, platform)
  const needsUpdate = state === 'update_needed'
  const label = resolveStateLabel(state, platform)
  const platformName =
    platform === 'apple' ? copy.platformApple : copy.platformGoogle

  return (
    <div
      className={[
        'flex items-start justify-between gap-4 py-3',
        needsUpdate
          ? 'border-l-2 border-[var(--warning,#A0622A)] pl-3'
          : '',
      ]
        .filter(Boolean)
        .join(' ')}
      data-testid={`lifecycle-row-${platform}`}
    >
      <div className="space-y-0.5">
        <p
          className="text-xs font-medium uppercase tracking-wide text-[var(--ink-500)]"
         
        >
          {platformName}
        </p>
        <p
          className={[
            'text-sm font-medium',
            live
              ? 'text-[var(--moss-700)]'
              : needsUpdate
              ? 'text-[var(--warning,#A0622A)]'
              : 'text-[var(--ink-400)]',
          ].join(' ')}
         
          data-testid={`lifecycle-state-${platform}`}
        >
          {label}
        </p>
        {updatedAt && (
          <p
            className="text-xs text-[var(--ink-400)]"
           
          >
            {copy.lastUpdatedLabel} {formatBillingDate(updatedAt)}
          </p>
        )}
      </div>

      <div className="flex-shrink-0">
        {state === 'pending_credentials' && (
          <Link
            href="/settings/billing/pro-app-purchase/credentials"
            className="inline-flex h-8 items-center rounded-md border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-3 text-xs font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
            data-testid={`lifecycle-cta-credentials-${platform}`}
          >
            {copy.ctaUploadCredentials}
          </Link>
        )}

        {needsUpdate && (
          /* TODO(P14): wire to actual build-new-version route once it exists */
          <button
            type="button"
            disabled
            aria-disabled="true"
            className="inline-flex h-8 cursor-not-allowed items-center rounded-md border border-[var(--hairline,var(--ink-100))] bg-[var(--paper-200)] px-3 text-xs font-medium text-[var(--ink-400)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
            data-testid={`lifecycle-cta-build-${platform}`}
          >
            {copy.ctaBuildNewVersion}
          </button>
        )}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function WhiteLabelAppLifecycleWidget({
  storeId,
}: WhiteLabelAppLifecycleWidgetProps) {
  const { data: plan, isLoading } = useCurrentPlan(storeId)

  if (isLoading || !plan) return null
  if (!plan.addOns.includes('white_label_app')) return null

  const lifecycleState = plan.whiteLabelApp?.lifecycleState ?? null
  const updatedAt = plan.whiteLabelApp?.updatedAt ?? null

  const showApple = isAppleRelevant(lifecycleState)
  const showGoogle = isGoogleRelevant(lifecycleState)

  return (
    <div
      className="mt-5 space-y-0 divide-y divide-[var(--hairline,var(--ink-100))]"
      aria-label={copy.heading}
      data-testid="wl-lifecycle-widget"
    >
      <p
        className="pb-2 text-xs font-medium uppercase tracking-wide text-[var(--ink-500)]"
       
      >
        {copy.heading}
      </p>

      {showApple && (
        <PlatformRow
          platform="apple"
          state={lifecycleState}
          updatedAt={updatedAt}
        />
      )}

      {showGoogle && (
        <PlatformRow
          platform="google"
          state={lifecycleState}
          updatedAt={updatedAt}
        />
      )}

      {lifecycleState === 'update_needed' && (
        <p
          className="pt-3 text-xs text-[var(--ink-700)]"
         
          data-testid="lifecycle-update-note"
        >
          {copy.updateNeededNote}
        </p>
      )}
    </div>
  )
}
