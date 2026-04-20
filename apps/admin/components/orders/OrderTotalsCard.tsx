// components/orders/OrderTotalsCard.tsx
//
// Subtotal / shipping / tax / discount / refunded / grand total summary.
// Right-aligned column, hairline rule above the grand total which is set
// in serif for editorial weight.

import type { AdminOrder } from "@/lib/api/marketplace-api";
import { formatMoney } from "@/lib/format";

interface OrderTotalsCardProps {
  order: AdminOrder;
}

export function OrderTotalsCard({ order }: OrderTotalsCardProps) {
  const refunded = Number.parseFloat(order.refunded_amount);
  const showRefunded = Number.isFinite(refunded) && refunded > 0;
  return (
    <section
      aria-labelledby="order-totals-heading"
      className="flex flex-col gap-4"
    >
      <h2
        id="order-totals-heading"
        className="font-serif text-2xl font-medium text-foreground"
      >
        Totals
      </h2>
      <dl className="flex flex-col gap-2 text-sm text-foreground">
        <Row label="Subtotal" value={formatMoney(order.subtotal, order.currency_code)} />
        <Row label="Shipping" value={formatMoney(order.shipping_total, order.currency_code)} />
        <Row label="Tax" value={formatMoney(order.tax_total, order.currency_code)} />
        {Number.parseFloat(order.discount_total) > 0 && (
          <Row label="Discount" value={`− ${formatMoney(order.discount_total, order.currency_code)}`} />
        )}
        {showRefunded && (
          <Row
            label="Refunded"
            value={`− ${formatMoney(order.refunded_amount, order.currency_code)}`}
            tone="muted"
          />
        )}
        <div className="mt-2 flex items-baseline justify-between border-t border-[color:var(--ink-900)]/15 pt-3">
          <dt className="text-base text-foreground">Grand total</dt>
          <dd className="font-serif text-2xl tabular-nums text-foreground">
            {formatMoney(order.grand_total, order.currency_code)}
          </dd>
        </div>
      </dl>
    </section>
  );
}

interface RowProps {
  label: string;
  value: string;
  tone?: "default" | "muted";
}

function Row({ label, value, tone = "default" }: RowProps) {
  const color = tone === "muted" ? "text-foreground-secondary" : "text-foreground";
  return (
    <div className="flex items-baseline justify-between">
      <dt className={color}>{label}</dt>
      <dd className={`${color} tabular-nums`}>{value}</dd>
    </div>
  );
}
