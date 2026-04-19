/**
 * /settings/billing/pro-app-purchase — Pro+App add-on purchase entry point.
 *
 * Server component: resolves storeId from the session context then
 * delegates to ProAppPurchaseClient (client component with React Query).
 */
import { AdminPage } from '@/components/layout'
import { getServerSessionContext } from '@/lib/auth/serverSession'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { ProAppPurchaseClient } from './ProAppPurchaseClient'

const copy = subscriptionCopy.proApp.purchase

export default async function ProAppPurchasePage() {
  const { currentStore } = await getServerSessionContext()

  return (
    <AdminPage
      eyebrow="Account"
      title={copy.heading}
      description={copy.intro}
    >
      {currentStore ? (
        <ProAppPurchaseClient storeId={currentStore.id} />
      ) : (
        <p className="text-sm text-[var(--danger)]">
          No store found. Please create a store before adding plan add-ons.
        </p>
      )}
    </AdminPage>
  )
}
