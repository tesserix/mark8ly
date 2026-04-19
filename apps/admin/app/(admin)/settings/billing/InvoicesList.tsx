'use client'

import { useToast } from '@/components/feedback/Toaster'
import { useOpenPortal } from '@/lib/api/subscription/hooks/useBilling'
import { subscriptionCopy } from '@/lib/copy/subscription'

interface InvoicesListProps {
  storeId: string
}

const copy = subscriptionCopy.billing

/**
 * InvoicesList — invoice management panel.
 *
 * There is no dedicated invoice list endpoint. Invoices are downloaded via
 * the Stripe Customer Portal (POST .../portal). A single editorial CTA
 * opens the portal session.
 *
 * Design: hairline at bottom only, Source Serif 4 H2, Source Sans 3 body.
 */
export function InvoicesList({ storeId }: InvoicesListProps) {
  const openPortal = useOpenPortal(storeId)
  const { toast } = useToast()

  function handleOpenPortal() {
    openPortal.mutate(undefined, {
      onError: (err) => {
        toast.error(err.message ?? copy.loadingError)
      },
    })
  }

  return (
    <section
      aria-labelledby="invoices-heading"
      className="border-b border-[var(--hairline,var(--ink-100))] pb-10"
    >
      <h2
        id="invoices-heading"
        className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-[var(--ink-900)]"
      >
        {copy.invoicesHeading}
      </h2>

      <p className="mt-3 max-w-xl text-sm leading-6 text-[var(--ink-700)]">
        {copy.invoicesDescription}
      </p>

      <div className="mt-6">
        <button
          type="button"
          onClick={handleOpenPortal}
          disabled={openPortal.isPending}
          className="inline-flex h-10 items-center rounded-[6px] border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-5 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
          aria-busy={openPortal.isPending}
        >
          {openPortal.isPending ? copy.openingPortal : copy.openPortalCta}
        </button>
      </div>
    </section>
  )
}
