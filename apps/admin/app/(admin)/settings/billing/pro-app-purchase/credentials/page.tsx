/**
 * /settings/billing/pro-app-purchase/credentials — App credential upload.
 *
 * Server component: resolves storeId from the session context then
 * delegates to CredentialUploadForm (client component with React Query).
 */
import { AdminPage } from '@/components/layout'
import { getServerSessionContext } from '@/lib/auth/serverSession'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { CredentialUploadForm } from '../CredentialUploadForm'

const copy = subscriptionCopy.appCredentials

export default async function CredentialsPage() {
  const { currentStore } = await getServerSessionContext()

  return (
    <AdminPage
      eyebrow="Account"
      title={copy.heading}
      description={copy.intro}
    >
      {currentStore ? (
        <CredentialUploadForm storeId={currentStore.id} />
      ) : (
        <p className="text-sm text-[var(--danger)]">
          No store found. Please create a store before uploading credentials.
        </p>
      )}
    </AdminPage>
  )
}
