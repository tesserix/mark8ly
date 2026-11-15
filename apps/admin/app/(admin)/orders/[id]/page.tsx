// app/orders/[id]/page.tsx
//
// Order detail page. Server component fetches the order via getOrder and
// composes the editorial detail layout: header (number + status badges) →
// items table → addresses → totals → actions bar (client component).

import { notFound } from "next/navigation";

import { Breadcrumbs } from "@/components/layout";
import { OrderActionsBar } from "@/components/orders/OrderActionsBar";
import { OrderArtworkPanel } from "@/components/orders/OrderArtworkPanel";
import { ShippingLabelPanel } from "@/components/orders/ShippingLabelPanel";
import { OrderAddressCard } from "@/components/orders/OrderAddressCard";
import { OrderDetailHeader } from "@/components/orders/OrderDetailHeader";
import { OrderDocumentsPanel } from "@/components/orders/OrderDocumentsPanel";
import { OrderItemsTable } from "@/components/orders/OrderItemsTable";
import { OrderLifecycleStepper } from "@/components/orders/OrderLifecycleStepper";
import { OrderTotalsCard } from "@/components/orders/OrderTotalsCard";
import { OutsideRefundForm } from "@/components/orders/OutsideRefundForm";
import { InStoreSaleCard, showsShipping } from "@/components/orders/InStoreSaleCard";
import { InStoreReceiptCard } from "@/components/orders/InStoreReceiptCard";
import { getOrder } from "@/lib/api/marketplace-api";
import { getOrderShipment } from "@/lib/api/shipping-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default async function OrderDetailPage({ params }: PageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId, role } = session;

  if (!currentStore) {
    notFound();
  }

  const order = await getOrder(currentStore.id, id, { userId, tenantId });
  if (!order) {
    notFound();
  }
  // Receipt availability is gated on shipment delivery, so we look it up
  // in parallel-friendly fashion. Failures (no shipment yet) yield null
  // and the documents panel renders the receipt as not-yet-available.
  const shipment = await getOrderShipment(currentStore.id, id, {
    userId,
    tenantId,
  }).catch(() => null);

  return (
    <main className="flex flex-col gap-8" aria-labelledby="order-heading">
      <Breadcrumbs
        items={[
          { label: "Orders", href: "/orders" },
          { label: order.order_number },
        ]}
      />

      {/* Masthead — serif order number + serif grand total on a single
          baseline. Meta row carries placed date, customer, item count. */}
      <OrderDetailHeader order={order} />

      {/* Lifecycle rail — the canonical status signal. Directly under
          the masthead so a merchant can read "where is this order" in
          one glance. No duplicated status badges above. */}
      <OrderLifecycleStepper order={order} shipmentStatus={shipment?.status ?? null} />

      <div className="grid grid-cols-1 gap-10 lg:grid-cols-[minmax(0,1fr)_360px]">
        {/* Main column — tighter rhythm between financial sections
            (items → totals), looser at the section boundary into
            addresses. Creates grouping without adding chrome. */}
        <div className="flex min-w-0 flex-col gap-6">
          <OrderItemsTable
            items={order.items}
            currencyCode={order.currency_code}
          />
          {/* Directly under the items, above the money: for a custom
              order this is the production brief, and a merchant opening
              the page to find out what to make should not have to scroll
              past totals and addresses to reach it. Renders nothing when
              no line carries buyer input (#968). */}
          <OrderArtworkPanel
            storeId={currentStore.id}
            orderId={order.id}
            items={order.items}
          />
          <div className="border-t border-border-subtle pt-6">
            <OrderTotalsCard order={order} />
          </div>
          {showsShipping(order) ? (
            <div className="mt-4 border-t border-border-subtle pt-8">
              <OrderAddressCard addresses={order.addresses} />
            </div>
          ) : null}
        </div>

        {/* Aside — sticky action workspace. All three sections use the
            hairline system; no elevated cards inside. */}
        <aside className="flex flex-col gap-8 lg:sticky lg:top-8 lg:self-start">
          <InStoreSaleCard order={order} />
          <OrderActionsBar order={order} shipmentStatus={shipment?.status ?? null} />
          {showsShipping(order) ? (
            <div className="border-t border-border-subtle pt-8">
              <ShippingLabelPanel
                storeId={currentStore.id}
                orderId={order.id}
                orderStatus={order.status}
                customerCarrier={order.shipping_carrier}
                customerService={order.shipping_service}
              />
            </div>
          ) : null}
          <InStoreReceiptCard storeId={currentStore.id} orderId={order.id} channel={order.channel} />
          {/* Refunds made outside mark8ly (#1110): in-store sales, owner and admin. */}
          {order.channel === "in_store" &&
          (role === "owner" || role === "admin") &&
          (order.payment_status === "paid" || order.payment_status === "partially_refunded") ? (
            <section aria-labelledby="outside-refund-heading" className="flex flex-col gap-4 border-t border-border-subtle pt-8">
              <h2
                id="outside-refund-heading"
                className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
              >
                Record a refund
              </h2>
              <OutsideRefundForm order={order} />
            </section>
          ) : null}
          <div className="border-t border-border-subtle pt-8">
            <OrderDocumentsPanel
              order={order}
              shipmentStatus={shipment?.status ?? null}
            />
          </div>
        </aside>
      </div>
    </main>
  );
}
