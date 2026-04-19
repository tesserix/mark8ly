/**
 * /settings/arbitrage — geo-pricing arbitrage appeal page.
 *
 * Server component: resolves storeId from session then renders the client
 * appeal form. The (admin) layout handles auth/tenant middleware — this page
 * never touches auth directly.
 *
 * Design: editorial. H1 in Source Serif 4. Intro paragraph in Source Sans 3.
 * Left-aligned 2/3 max-width column. Hairline rules between sections.
 */
import { AdminPage } from '@/components/layout'
import { getServerSessionContext } from '@/lib/auth/serverSession'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { AppealForm } from './AppealForm'

const copy = subscriptionCopy.arbitrage.appeal

export default async function ArbitragePage() {
  const { currentStore } = await getServerSessionContext()

  return (
    <AdminPage eyebrow="Billing" title={copy.heading}>
      <div className="max-w-2xl">
        <p
          className="mb-8 text-sm leading-relaxed text-[var(--ink-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {copy.intro}
        </p>

        {currentStore ? (
          <AppealForm storeId={currentStore.id} />
        ) : (
          <p className="text-sm text-[var(--danger)]">
            No store found. Please create a store before submitting an appeal.
          </p>
        )}
      </div>
    </AdminPage>
  )
}
