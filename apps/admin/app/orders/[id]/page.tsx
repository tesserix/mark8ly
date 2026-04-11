// app/orders/[id]/page.tsx
//
// Order detail page. Server component fetches the order via getOrder and
// composes the editorial detail layout: header (number + status badges) →
// items table → addresses → totals → actions bar (client component).

import { notFound } from "next/navigation";

import { AdminShell } from "@/components/shell/AdminShell";
import { OrderActionsBar } from "@/components/orders/OrderActionsBar";
import { OrderAddressCard } from "@/components/orders/OrderAddressCard";
import { OrderDetailHeader } from "@/components/orders/OrderDetailHeader";
import { OrderItemsTable } from "@/components/orders/OrderItemsTable";
import { OrderTotalsCard } from "@/components/orders/OrderTotalsCard";
import { getOrder } from "@/lib/api/marketplace-api";
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

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main
        className="mx-auto flex w-full max-w-5xl flex-col gap-10"
        aria-labelledby="order-heading"
      >
        <OrderDetailHeader order={order} />

        <div className="grid grid-cols-1 gap-10 lg:grid-cols-[2fr_1fr]">
          <div className="flex flex-col gap-10">
            <OrderItemsTable
              items={order.items}
              currencyCode={order.currency_code}
            />
            <OrderAddressCard addresses={order.addresses} />
          </div>
          <div className="flex flex-col gap-10 lg:sticky lg:top-8 lg:self-start">
            <OrderTotalsCard order={order} />
            <OrderActionsBar order={order} />
          </div>
        </div>
      </main>
    </AdminShell>
  );
}
