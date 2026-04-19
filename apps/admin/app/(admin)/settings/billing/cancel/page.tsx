/**
 * /settings/billing/cancel — cancellation wizard entry point.
 *
 * Server component: resolves storeId from session, then delegates to the
 * client-side CancelWizard for the 4-step state machine.
 */
import { AdminPage } from '@/components/layout'
import { getServerSessionContext } from '@/lib/auth/serverSession'
import { CancelWizard } from './CancelWizard'

export default async function CancelPage() {
  const { currentStore } = await getServerSessionContext()

  if (!currentStore) {
    return (
      <AdminPage eyebrow="Account" title="Billing">
        <p className="text-sm text-[var(--danger)]">
          No store found. Please create a store before managing billing.
        </p>
      </AdminPage>
    )
  }

  return (
    <AdminPage eyebrow="Account" title="Billing">
      <CancelWizard storeId={currentStore.id} />
    </AdminPage>
  )
}
