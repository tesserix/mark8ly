/**
 * /admin/stores/close-before-downgrade
 *
 * Server component stub — renders the interactive client component.
 * The page is reached after a plan-change attempt returns 409
 * plan_change_blocked_store_cap, with query params:
 *
 *   ?target=studio&billing=annual&over_by=2
 *
 * The page title is deliberately plain — editorial, not alarming.
 */
import type { Metadata } from 'next'
import { CloseBeforeDowngradeClient } from './CloseBeforeDowngradeClient'

export const metadata: Metadata = {
  title: 'Over-cap stores — Mark8ly',
}

export default function CloseBeforeDowngradePage() {
  return <CloseBeforeDowngradeClient />
}
