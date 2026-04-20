// Documents panel — invoice + receipt download links anchored to the
// order detail page's right column. Sits below the totals card so the
// merchant can pull official paperwork in one click.
//
// Two documents, two lifecycles:
//   • Invoice — emailed on order acceptance; downloadable from now on
//   • Receipt — emailed on delivery; only downloadable once delivered
//
// Each row also exposes a "Resend email" affordance so the merchant can
// re-deliver the document on customer request without leaving the page.

"use client";

import { useTransition } from "react";

import { useToast } from "@/components/feedback/Toaster";
import type { AdminOrder } from "@/lib/api/marketplace-api";
import {
  invoiceNumberFromOrder,
  receiptNumberFromOrder,
} from "@/lib/invoices/numbering";
import { resendDocumentEmail } from "@/app/(admin)/orders/[id]/document-actions";

interface Props {
  order: AdminOrder;
  shipmentStatus?: string | null;
}

export function OrderDocumentsPanel({ order, shipmentStatus }: Props) {
  const invoiceNumber = invoiceNumberFromOrder(order.order_number);
  const receiptNumber = receiptNumberFromOrder(order.order_number);
  const receiptAvailable = shipmentStatus === "delivered";

  const baseHref = `/api/admin/stores/${order.store_id}/orders/${order.id}`;

  return (
    <section
      aria-labelledby="documents-heading"
      className="flex flex-col gap-4"
    >
      <h2
        id="documents-heading"
        className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
      >
        Documents
      </h2>

      <div className="flex flex-col gap-4">
        <DocumentRow
          kind="invoice"
          storeId={order.store_id}
          orderId={order.id}
          recipient={order.customer_email}
          label="Invoice"
          number={invoiceNumber}
          href={`${baseHref}/invoice`}
          enabled
          hint="Emailed automatically when the order is accepted."
        />
        <div className="h-px bg-[color:var(--ink-900)]/10" />
        <DocumentRow
          kind="receipt"
          storeId={order.store_id}
          orderId={order.id}
          recipient={order.customer_email}
          label="Receipt"
          number={receiptAvailable ? receiptNumber : "Available after delivery"}
          href={`${baseHref}/receipt`}
          enabled={receiptAvailable}
          hint={
            receiptAvailable
              ? "Emailed automatically when the order is delivered."
              : "Issued automatically once the shipment is marked delivered."
          }
        />
      </div>
    </section>
  );
}

interface RowProps {
  kind: "invoice" | "receipt";
  storeId: string;
  orderId: string;
  recipient: string;
  label: string;
  number: string;
  href: string;
  enabled: boolean;
  hint?: string;
}

function DocumentRow({
  kind,
  storeId,
  orderId,
  recipient,
  label,
  number,
  href,
  enabled,
  hint,
}: RowProps) {
  const { toast } = useToast();
  const [pending, startTransition] = useTransition();

  function resend() {
    startTransition(async () => {
      const r = await resendDocumentEmail(storeId, orderId, kind);
      if (r.ok) {
        toast.success(`${label} emailed`, `Delivered to ${recipient}.`);
      } else {
        toast.error(`Couldn't send ${label.toLowerCase()}`, r.error?.message ?? "Please try again.");
      }
    });
  }

  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex flex-col">
        <span className="text-xs uppercase tracking-wider text-foreground-tertiary">
          {label}
        </span>
        <span
          className={`text-sm tabular-nums ${
            enabled ? "text-foreground" : "text-foreground-tertiary"
          }`}
        >
          {number}
        </span>
        {hint && (
          <span className="mt-1 text-xs text-foreground-tertiary">{hint}</span>
        )}
      </div>
      <div className="flex shrink-0 flex-col items-end gap-1.5">
        {enabled ? (
          <>
            <a
              href={href}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)]/40 px-3 py-1.5 text-xs text-foreground transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              Download PDF
            </a>
            <button
              type="button"
              onClick={resend}
              disabled={pending}
              className="text-xs text-foreground-secondary underline-offset-4 transition-colors hover:text-[color:var(--moss-700)] hover:underline disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              {pending ? "Sending…" : "Email to customer"}
            </button>
          </>
        ) : (
          <span className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)]/15 px-3 py-1.5 text-xs text-foreground-tertiary">
            Not yet available
          </span>
        )}
      </div>
    </div>
  );
}
