/**
 * /settings/billing/pro-contact — Pro contact-sales entry point.
 *
 * Server component: resolves storeId from the session context then
 * delegates to ProContactForm (client component with RHF + React Query).
 *
 * The (admin) route group layout handles auth/tenant — this page never
 * touches auth directly.
 */
import { AdminPage } from '@/components/layout'
import { getServerSessionContext } from '@/lib/auth/serverSession'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { ProContactForm } from './ProContactForm'

const copy = subscriptionCopy.proContact

export default async function ProContactPage() {
  const { currentStore } = await getServerSessionContext()

  return (
    <AdminPage
      eyebrow="Account"
      title={copy.heading}
      description={copy.intro}
    >
      {currentStore ? (
        <ProContactForm storeId={currentStore.id} />
      ) : (
        <p className="text-sm text-[var(--danger)]">
          No store found. Please create a store before contacting sales.
        </p>
      )}
    </AdminPage>
  )
}
