// Documents panel — invoice + receipt download links anchored to the
// order detail page's right column. Sits below the totals card so the
// merchant can pull official paperwork in one click.
//
// Receipts are only issued for paid orders; we render a disabled hint
// for unpaid orders so the affordance is discoverable but accurate.

import type { AdminOrder } from "@/lib/api/marketplace-api";
import { invoiceNumberFromOrder, receiptNumberFromOrder } from "@/lib/invoices/numbering";

interface Props {
  order: AdminOrder;
}

const PAID_STATUSES = new Set(["paid", "captured", "partially_refunded"]);

export function OrderDocumentsPanel({ order }: Props) {
  const invoiceNumber = invoiceNumberFromOrder(order.order_number);
  const receiptNumber = receiptNumberFromOrder(order.order_number);
  const receiptAvailable = PAID_STATUSES.has(order.payment_status);

  const baseHref = `/api/admin/stores/${order.store_id}/orders/${order.id}`;

  return (
    <section
      aria-labelledby="documents-heading"
      className="flex flex-col gap-4"
    >
      <h2
        id="documents-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-xl text-[color:var(--ink-900)]"
      >
        Documents
      </h2>

      <div className="flex flex-col gap-3 rounded-md border border-[color:var(--ink-900)]/10 bg-white px-5 py-4 shadow-sm">
        <DocumentRow
          label="Invoice"
          number={invoiceNumber}
          href={`${baseHref}/invoice`}
          enabled
        />
        <div className="h-px bg-[color:var(--ink-900)]/10" />
        <DocumentRow
          label="Receipt"
          number={receiptAvailable ? receiptNumber : "Available after payment"}
          href={`${baseHref}/receipt`}
          enabled={receiptAvailable}
          hint={receiptAvailable ? undefined : "Issued automatically once payment clears."}
        />
      </div>
    </section>
  );
}

interface RowProps {
  label: string;
  number: string;
  href: string;
  enabled: boolean;
  hint?: string;
}

function DocumentRow({ label, number, href, enabled, hint }: RowProps) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="flex flex-col">
        <span className="text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
          {label}
        </span>
        <span
          className={`text-sm ${
            enabled
              ? "text-[color:var(--ink-900)]"
              : "text-[color:var(--ink-900)] opacity-50"
          }`}
          style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
        >
          {number}
        </span>
        {hint && (
          <span className="mt-1 text-xs text-[color:var(--ink-900)] opacity-50">
            {hint}
          </span>
        )}
      </div>
      {enabled ? (
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex shrink-0 items-center gap-2 rounded-md border border-[color:var(--ink-900)] border-opacity-40 px-3 py-1.5 text-xs text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Download PDF
        </a>
      ) : (
        <span className="inline-flex shrink-0 items-center gap-2 rounded-md border border-[color:var(--ink-900)]/15 px-3 py-1.5 text-xs text-[color:var(--ink-900)]/40">
          Not yet available
        </span>
      )}
    </div>
  );
}
