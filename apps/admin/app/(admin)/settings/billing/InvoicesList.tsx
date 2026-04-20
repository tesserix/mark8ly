'use client'

import { Skeleton } from '@tesserix/web'
import { useToast } from '@/components/feedback/Toaster'
import {
  useInvoices,
  useOpenPortal,
} from '@/lib/api/subscription/hooks/useBilling'
import type { Invoice } from '@/lib/api/subscription/schemas/billing'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { formatBillingDate } from '@/lib/format/date'

interface InvoicesListProps {
  storeId: string
}

const copy = subscriptionCopy.billing

const ZERO_DECIMAL = new Set([
  'bif', 'clp', 'djf', 'gnf', 'jpy', 'kmf', 'krw', 'mga', 'pyg',
  'rwf', 'ugx', 'vnd', 'vuv', 'xaf', 'xof', 'xpf',
])

function formatAmount(minor: number, currency: string): string {
  const code = currency.toUpperCase()
  const lower = currency.toLowerCase()
  const major = ZERO_DECIMAL.has(lower) ? minor : minor / 100
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: code,
      minimumFractionDigits: ZERO_DECIMAL.has(lower) ? 0 : 2,
    }).format(major)
  } catch {
    return `${code} ${major.toFixed(ZERO_DECIMAL.has(lower) ? 0 : 2)}`
  }
}

function statusLabel(status: string): { label: string; tone: string } {
  switch (status) {
    case 'paid':
      return { label: 'Paid', tone: 'var(--moss-700)' }
    case 'open':
      return { label: 'Open', tone: 'var(--ink-700)' }
    case 'void':
      return { label: 'Void', tone: 'var(--ink-500)' }
    case 'draft':
      return { label: 'Draft', tone: 'var(--ink-500)' }
    case 'uncollectible':
      return { label: 'Uncollectible', tone: 'var(--danger,#7a1a1a)' }
    default:
      return { label: status, tone: 'var(--ink-700)' }
  }
}

function InvoiceRow({ invoice }: { invoice: Invoice }) {
  const amount =
    invoice.status === 'paid' ? invoice.amount_paid : invoice.amount_due
  const { label, tone } = statusLabel(invoice.status)

  return (
    <tr className="border-b border-[var(--hairline,var(--ink-100))]">
      <td className="py-4 text-sm text-[var(--ink-900)]">
        {formatBillingDate(invoice.created_at)}
      </td>
      <td className="py-4 text-sm text-[var(--ink-700)]">
        {invoice.number || '—'}
      </td>
      <td className="py-4 text-sm text-[var(--ink-900)]">
        {formatAmount(amount, invoice.currency)}
      </td>
      <td className="py-4 text-sm" style={{ color: tone }}>
        {label}
      </td>
      <td className="py-4 text-right text-sm">
        {invoice.invoice_pdf ? (
          <a
            href={invoice.invoice_pdf}
            target="_blank"
            rel="noopener noreferrer"
            className="text-[var(--moss-700)] underline-offset-2 hover:underline focus-visible:outline-none focus-visible:underline"
          >
            {copy.invoiceDownloadPdf}
          </a>
        ) : invoice.hosted_invoice_url ? (
          <a
            href={invoice.hosted_invoice_url}
            target="_blank"
            rel="noopener noreferrer"
            className="text-[var(--moss-700)] underline-offset-2 hover:underline"
          >
            {copy.invoiceView}
          </a>
        ) : (
          <span className="text-[var(--ink-400)]">—</span>
        )}
      </td>
    </tr>
  )
}

function InvoicesTableSkeleton() {
  return (
    <div className="mt-6 space-y-3" aria-label={copy.loadingAriaLabel}>
      <Skeleton className="h-4 w-full rounded-md" />
      <Skeleton className="h-4 w-full rounded-md" />
      <Skeleton className="h-4 w-3/4 rounded-md" />
    </div>
  )
}

/**
 * InvoicesList — invoice history panel.
 *
 * Fetches up to 25 most-recent invoices from Stripe (via the Go admin
 * endpoint) and renders a hairline-row table. PDF links go straight to
 * Stripe's signed URLs; a secondary "Manage in portal" link covers
 * operations not yet in the UI (update card, cancel, etc.).
 */
export function InvoicesList({ storeId }: InvoicesListProps) {
  const { data: invoices, isLoading, error, refetch } = useInvoices(storeId)
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
      <div className="flex flex-wrap items-baseline justify-between gap-4">
        <h2
          id="invoices-heading"
          className="font-serif text-2xl font-medium tracking-tight text-[var(--ink-900)]"
        >
          {copy.invoicesHeading}
        </h2>
        <button
          type="button"
          onClick={handleOpenPortal}
          disabled={openPortal.isPending}
          className="text-sm text-[var(--ink-700)] underline-offset-2 hover:underline focus-visible:outline-none focus-visible:underline disabled:cursor-not-allowed disabled:opacity-60"
        >
          {openPortal.isPending ? copy.openingPortal : copy.manageInPortal}
        </button>
      </div>

      {isLoading && <InvoicesTableSkeleton />}

      {error && !isLoading && (
        <div className="mt-6 space-y-4" role="alert">
          <p className="text-sm text-[var(--ink-700)]">{copy.invoicesError}</p>
          <button
            type="button"
            onClick={() => void refetch()}
            className="text-sm text-[var(--moss-700)] underline-offset-2 hover:underline"
          >
            {copy.retryLabel}
          </button>
        </div>
      )}

      {!isLoading && !error && invoices && invoices.length === 0 && (
        <p className="mt-6 text-sm text-[var(--ink-700)]">
          {copy.invoicesEmpty}
        </p>
      )}

      {!isLoading && !error && invoices && invoices.length > 0 && (
        <div className="mt-6 overflow-x-auto">
          <table className="w-full min-w-[640px] border-collapse text-left">
            <thead>
              <tr className="border-b border-[var(--hairline,var(--ink-100))]">
                <th className="pb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-500)]">
                  {copy.invoiceColDate}
                </th>
                <th className="pb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-500)]">
                  {copy.invoiceColNumber}
                </th>
                <th className="pb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-500)]">
                  {copy.invoiceColAmount}
                </th>
                <th className="pb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-500)]">
                  {copy.invoiceColStatus}
                </th>
                <th className="pb-3 text-right text-xs font-medium uppercase tracking-wide text-[var(--ink-500)]">
                  {copy.invoiceColActions}
                </th>
              </tr>
            </thead>
            <tbody>
              {invoices.map((inv) => (
                <InvoiceRow key={inv.id} invoice={inv} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
