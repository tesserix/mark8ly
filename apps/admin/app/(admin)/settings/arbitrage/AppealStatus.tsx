/**
 * AppealStatus — stub for Task 21.
 *
 * Reads `latest_arbitrage_audit.appeal_status` from the subscription response
 * when present and renders the appropriate status message.
 *
 * For Task 19/20: shows the pending copy when the store is flagged.
 * Full status tracking (resolved / rejected) is wired in Task 21.
 */
'use client'

import { subscriptionCopy } from '@/lib/copy/subscription'
import { useArbitrageFlag } from '@/lib/api/subscription/hooks/useArbitrage'

const copy = subscriptionCopy.arbitrage.appeal

interface AppealStatusProps {
  storeId: string
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function AppealStatus({ storeId }: AppealStatusProps) {
  const { flagged, isLoading } = useArbitrageFlag(storeId)

  if (isLoading) return null

  if (!flagged) return null

  // TODO(task-21): read `latestAudit.appeal_status` once CurrentPlan carries
  // the full audit shape. Map 'pending' → status.pending, 'resolved' →
  // status.resolved, 'rejected' → status.rejected. For now show pending copy.
  return (
    <p
      role="status"
      aria-live="polite"
      className="text-sm text-[var(--ink-700)]"
      style={{ fontFamily: "'Source Sans 3', sans-serif" }}
    >
      {copy.status.pending}
    </p>
  )
}
