// components/orders/OrderDetailHeader.tsx
//
// Top of the order detail page. Editorial: serif order number, customer
// muted underneath, three status badges in a row, "back to orders" link.

import Link from "next/link";
import { ChevronLeft } from "lucide-react";

import type { AdminOrder } from "@/lib/api/marketplace-api";

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
    <header className="flex flex-col gap-4">
      <Link
        href="/orders"
        className="inline-flex items-center gap-1 text-sm text-[color:var(--ink-900)] opacity-70 hover:opacity-100 hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        <ChevronLeft className="h-4 w-4" />
        All orders
      </Link>

      <div className="flex flex-col gap-2">
        <h1
          id="order-heading"
          className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-4xl text-[color:var(--ink-900)]"
        >
          {order.order_number}
        </h1>
        <p className="text-sm text-[color:var(--ink-900)] opacity-70">
          Placed {formatPlacedAt(order.placed_at)} · {order.customer_name ?? order.customer_email}
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
