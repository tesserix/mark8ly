// components/orders/OrderDetailHeader.tsx
//
// Top of the order detail page. Editorial: serif order number, customer
// muted underneath, three status badges in a row, "back to orders" link.

import type { AdminOrder } from "@/lib/api/marketplace-api";
import { formatDate } from "@/lib/format";

import {
  FulfillmentStatusBadge,
  OrderStatusBadge,
  PaymentStatusBadge,
} from "./OrderStatusBadges";

interface OrderDetailHeaderProps {
  order: AdminOrder;
}

export function OrderDetailHeader({ order }: OrderDetailHeaderProps) {
  return (
    <header className="flex flex-col gap-3">
      <div className="flex flex-col gap-2">
        <h1
          id="order-heading"
          className="font-serif text-2xl font-medium tracking-tight text-foreground sm:text-3xl"
        >
          {order.order_number}
        </h1>
        <p className="text-sm text-foreground-secondary">
          Placed {formatDate(order.placed_at)} · {order.customer_name ?? order.customer_email}
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-x-6 gap-y-2 pt-1 text-sm">
        <OrderStatusBadge status={order.status} />
        <PaymentStatusBadge status={order.payment_status} />
        <FulfillmentStatusBadge status={order.fulfillment_status} />
      </div>
    </header>
  );
}

function formatPlacedAt(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleDateString(undefined, {
      year: "numeric",
      month: "long",
      day: "numeric",
    });
  } catch {
    return iso;
  }
}
