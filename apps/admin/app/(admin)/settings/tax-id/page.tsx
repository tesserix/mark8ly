/**
 * /settings/tax-id — Tax-ID management page.
 *
 * Server component: resolves storeId from session, then renders the
 * TaxIdForm client component. The (admin) layout handles auth/tenant
 * middleware — this page never touches auth directly.
 *
 * Design: editorial. H1 in Source Serif 4. Intro in Source Sans 3.
 * Left-aligned max-w-2xl column. Hairline rules between sections.
 */
import { AdminPage } from '@/components/layout'
import { getServerSessionContext } from '@/lib/auth/serverSession'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { TaxIdForm } from './TaxIdForm'

const copy = subscriptionCopy.taxId

export default async function TaxIdPage() {
  const { currentStore } = await getServerSessionContext()

  return (
    <AdminPage eyebrow="Billing" title={copy.heading}>
      <div className="max-w-2xl">
        <p
          className="mb-8 text-sm leading-relaxed text-[var(--ink-700)]"
         
        >
          {copy.intro}
        </p>

        {currentStore ? (
          <TaxIdForm storeId={currentStore.id} />
        ) : (
          <p
            className="text-sm text-[var(--danger)]"
           
          >
            No store found. Please create a store before submitting tax details.
          </p>
        )}
      </div>
    </AdminPage>
  )
}
