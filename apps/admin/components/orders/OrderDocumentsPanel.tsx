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

import { useState, useTransition } from "react";

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
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-xl text-[color:var(--ink-900)]"
      >
        Documents
      </h2>

      <div className="flex flex-col gap-4 rounded-md border border-[color:var(--ink-900)]/10 bg-white px-5 py-4 shadow-sm">
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
  const [pending, startTransition] = useTransition();
  const [feedback, setFeedback] = useState<{ tone: "ok" | "err"; text: string } | null>(null);

  function resend() {
    setFeedback(null);
    startTransition(async () => {
      const r = await resendDocumentEmail(storeId, orderId, kind);
      if (r.ok) {
        setFeedback({ tone: "ok", text: `Sent to ${recipient}.` });
        // Auto-clear after a few seconds
        setTimeout(() => setFeedback(null), 4000);
      } else {
        setFeedback({ tone: "err", text: r.error?.message ?? "Send failed." });
      }
    });
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-start justify-between gap-4">
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
        <div className="flex shrink-0 flex-col items-end gap-1.5">
          {enabled ? (
            <>
              <a
                href={href}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)] border-opacity-40 px-3 py-1.5 text-xs text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              >
                Download PDF
              </a>
              <button
                type="button"
                onClick={resend}
                disabled={pending}
                className="text-xs text-[color:var(--ink-900)] opacity-70 underline-offset-4 hover:underline hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              >
                {pending ? "Sending…" : "Email to customer"}
              </button>
            </>
          ) : (
            <span className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)]/15 px-3 py-1.5 text-xs text-[color:var(--ink-900)]/40">
              Not yet available
            </span>
          )}
        </div>
      </div>
      {feedback && (
        <p
          role="status"
          className={`text-xs ${
            feedback.tone === "ok"
              ? "text-[color:var(--moss-700)]"
              : "text-[color:var(--danger,#5a1010)]"
          }`}
        >
          {feedback.text}
        </p>
      )}
    </div>
  );
}
