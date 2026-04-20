// app/orders/[id]/page.tsx
//
// Order detail page. Server component fetches the order via getOrder and
// composes the editorial detail layout: header (number + status badges) →
// items table → addresses → totals → actions bar (client component).

import { notFound } from "next/navigation";

import { Breadcrumbs } from "@/components/layout";
import { OrderActionsBar } from "@/components/orders/OrderActionsBar";
import { ShippingLabelPanel } from "@/components/orders/ShippingLabelPanel";
import { OrderAddressCard } from "@/components/orders/OrderAddressCard";
import { OrderDetailHeader } from "@/components/orders/OrderDetailHeader";
import { OrderDocumentsPanel } from "@/components/orders/OrderDocumentsPanel";
import { OrderItemsTable } from "@/components/orders/OrderItemsTable";
import { OrderLifecycleStepper } from "@/components/orders/OrderLifecycleStepper";
import { OrderTotalsCard } from "@/components/orders/OrderTotalsCard";
import { getOrder } from "@/lib/api/marketplace-api";
import { getOrderShipment } from "@/lib/api/shipping-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default async function OrderDetailPage({ params }: PageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;

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
    <main className="flex flex-col gap-10" aria-labelledby="order-heading">
      <Breadcrumbs
        items={[
          { label: "Orders", href: "/orders" },
          { label: order.order_number },
        ]}
      />

      <OrderDetailHeader order={order} />

      <OrderLifecycleStepper order={order} shipmentStatus={shipment?.status ?? null} />

      <div className="grid grid-cols-1 gap-10 lg:grid-cols-[minmax(0,1fr)_360px]">
        <div className="flex min-w-0 flex-col gap-10">
          <OrderItemsTable
            items={order.items}
            currencyCode={order.currency_code}
          />
          <div className="border-t border-border-subtle pt-10">
            <OrderTotalsCard order={order} />
          </div>
          <div className="border-t border-border-subtle pt-10">
            <OrderAddressCard addresses={order.addresses} />
          </div>
        </div>

        <aside className="flex flex-col gap-8 lg:sticky lg:top-8 lg:self-start">
          <OrderActionsBar order={order} />
          <div className="border-t border-border-subtle pt-8">
            <ShippingLabelPanel
              storeId={currentStore.id}
              orderId={order.id}
              orderStatus={order.status}
            />
          </div>
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
