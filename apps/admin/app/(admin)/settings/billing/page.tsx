import { AdminPage } from '@/components/layout'
import { getServerSessionContext } from '@/lib/auth/serverSession'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { BillingClient } from './BillingClient'

const copy = subscriptionCopy.billing

/**
 * /settings/billing — billing dashboard.
 *
 * Server component: resolves storeId from the session context then
 * delegates all data fetching + rendering to BillingClient (client
 * component with React Query).
 *
 * The (admin) route group layout handles auth/tenant middleware — this
 * page never touches auth directly.
 */
export default async function BillingPage() {
  const { currentStore } = await getServerSessionContext()

  return (
    <AdminPage
      eyebrow="Account"
      title={copy.heading}
      description={copy.description}
    >
      {currentStore ? (
        <BillingClient storeId={currentStore.id} />
      ) : (
        <p className="text-sm text-[var(--danger)]">
          No store found. Please create a store before managing billing.
        </p>
      )}
    </AdminPage>
  )
}
