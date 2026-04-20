// Order masthead. The page's editorial moment: order number and grand
// total sit on a single baseline, both in Source Serif 4, paired with
// a muted em-dash. Meta row below carries placed date, customer, and
// item count — all in body sans at tertiary weight.
//
// Status badges are intentionally NOT here — the lifecycle stepper
// directly below communicates the same state in a single visual
// language, so duplicating it as badges adds noise.

import type { AdminOrder } from "@/lib/api/marketplace-api";
import { formatDate, formatMoney } from "@/lib/format";

interface OrderDetailHeaderProps {
  order: AdminOrder;
}

export function OrderDetailHeader({ order }: OrderDetailHeaderProps) {
  const itemCount = order.items.reduce((sum, it) => sum + it.quantity, 0);
  const customer = order.customer_name?.trim() || order.customer_email;

  return (
    <header className="flex flex-col gap-3">
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <h1
          id="order-heading"
          className="font-serif text-3xl font-medium tracking-tight text-foreground tabular-nums sm:text-[2.25rem]"
        >
          {order.order_number}
        </h1>
        <span
          aria-hidden="true"
          className="font-serif text-2xl font-light text-foreground-tertiary sm:text-3xl"
        >
          —
        </span>
        <span
          aria-label={`Grand total ${order.grand_total} ${order.currency_code}`}
          className="font-serif text-2xl font-light text-foreground-secondary tabular-nums sm:text-3xl"
        >
          {formatMoney(order.grand_total, order.currency_code)}
        </span>
      </div>

      <p className="flex flex-wrap items-center gap-x-5 gap-y-1 text-sm text-foreground-secondary">
        <span>Placed {formatDate(order.placed_at)}</span>
        <span aria-hidden="true" className="text-foreground-tertiary">
          ·
        </span>
        <span className="text-foreground">{customer}</span>
        <span aria-hidden="true" className="text-foreground-tertiary">
          ·
        </span>
        <span>
          {itemCount} {itemCount === 1 ? "item" : "items"}
        </span>
      </p>
    </header>
  );
}
