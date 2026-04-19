/**
 * /settings/billing/plan-change — server component stub.
 *
 * Resolves storeId + currentPlan from the session context, then renders
 * the PlanChangeClient (client component with React Query).
 */
import { AdminPage } from '@/components/layout'
import { getServerSessionContext } from '@/lib/auth/serverSession'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { PlanChangeClient } from './PlanChangeClient'

const copy = subscriptionCopy.planChange

export default async function PlanChangePage() {
  const { currentStore } = await getServerSessionContext()

  return (
    <AdminPage
      eyebrow="Billing"
      title={copy.heading}
      description={copy.intro}
    >
      {currentStore ? (
        <PlanChangeClient storeId={currentStore.id} />
      ) : (
        <p className="text-sm text-[var(--danger)]">
          No store found. Please create a store before changing your plan.
        </p>
      )}
    </AdminPage>
  )
}
