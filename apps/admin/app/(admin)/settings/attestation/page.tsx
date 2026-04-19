/**
 * /settings/attestation — US/CA business-entity attestation page (§19.3.1).
 *
 * Server component: resolves storeId from session, then renders the
 * AttestationForm client component. The (admin) layout handles auth/tenant
 * middleware — this page never touches auth directly.
 *
 * Design: editorial. H1 in Source Serif 4. Intro in Source Sans 3.
 * Left-aligned max-w-2xl column.
 */
import { AdminPage } from '@/components/layout'
import { getServerSessionContext } from '@/lib/auth/serverSession'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { AttestationForm } from './AttestationForm'

const copy = subscriptionCopy.attestation

export default async function AttestationPage() {
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
          <AttestationForm storeId={currentStore.id} />
        ) : (
          <p
            className="text-sm text-[var(--danger)]"
            style={{ fontFamily: "'Source Sans 3', sans-serif" }}
          >
            No store found. Please create a store before completing the attestation.
          </p>
        )}
      </div>
    </AdminPage>
  )
}
