// Rendered inside /account/orders/[id] near the Totals section.
//
// The auto-refund fired on self-cancel of a paid order (see
// marketplace-api order_detail.go's post-cancel refund call) is
// best-effort and async: the cancel response returns immediately while
// the payment gateway settles the refund, and a gateway blip defers it
// to the background sweeper entirely. Until refunded_amount catches up
// to grand_total, the customer sees "Cancelled" with no refund line and
// no explanation — this note closes that gap with clear, transitional
// copy. Same theme-token + aria-live pattern as ReturnStatusBanner.

interface Props {
  orderStatus: string;
  paymentStatus: string;
  grandTotal: string;
  refundedAmount?: string;
}

// Payment states that imply money was actually captured and could still
// be owed back to the customer. "refunded" (fully settled) and states
// that never captured payment (pending/authorized/failed) are excluded.
const REFUND_ELIGIBLE_PAYMENT_STATUSES = new Set(["paid", "partially_refunded"]);

export function RefundProcessingBanner({
  orderStatus,
  paymentStatus,
  grandTotal,
  refundedAmount,
}: Props) {
  if (orderStatus !== "cancelled") return null;
  if (!REFUND_ELIGIBLE_PAYMENT_STATUSES.has(paymentStatus)) return null;

  const total = Number(grandTotal);
  const refunded = Number(refundedAmount ?? "0");
  if (!Number.isFinite(total) || total <= 0) return null;
  if (Number.isFinite(refunded) && refunded >= total) return null;

  return (
    <section
      role="status"
      aria-live="polite"
      className="rounded-md border px-4 py-3 text-sm text-[color:var(--storefront-text,var(--ink-900))]"
      style={{
        borderColor: "var(--storefront-neutral-border)",
        backgroundColor: "var(--storefront-neutral-bg)",
      }}
    >
      <p className="text-[13px] font-semibold">Refund in progress</p>
      <p className="mt-1 text-[13px] leading-relaxed text-[color:var(--storefront-text,var(--ink-900))]/80">
        Your refund is being processed and will appear on your original
        payment method within 5–10 business days.
      </p>
    </section>
  );
}
