// /account/orders/[id] — customer-facing single order detail.
// Reuses fetchOrder + the same visual layout as the post-checkout
// /orders/[id] page but rendered inside the account shell and with
// "Back to orders" nav instead of the store header.

import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { cookies, headers } from "next/headers";

import { resolveStoreSlug } from "@/lib/slug";
import { decodeSession } from "@/lib/session";
import { fetchOrder, type Order, type OrderItem } from "@/lib/api/checkout-api";
import { OrderTimeline } from "@/components/OrderTimeline";
import { CancelOrderButton } from "@/components/account/CancelOrderButton";
import { invoiceNumberFromOrder, receiptNumberFromOrder } from "@/lib/invoices/numbering";

export const dynamic = "force-dynamic";
export const revalidate = 0;

interface PageProps {
  params: Promise<{ id: string }>;
}

export const metadata = { title: "Order" };

export default async function AccountOrderPage({ params }: PageProps) {
  const { id } = await params;

  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value ?? "";
  const session = decodeSession(sessionCookie);
  if (!session) {
    return (
      <div className="space-y-2">
        <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          Order
        </h1>
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Please sign in to view this order.
        </p>
      </div>
    );
  }

  const h = await headers();
  const host = h.get("host");
  const slug =
    await resolveStoreSlug(host);
  if (!slug || !id) notFound();

  const order = await fetchOrder(slug, id);
  if (!order) notFound();

  return (
    <div className="space-y-6">
      <div>
        <Link
          href="/account/orders"
          className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60 transition-opacity hover:opacity-100"
        >
          ← All orders
        </Link>
        <h1 className="mt-2 font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          {order.order_number}
        </h1>
        <p className="mt-1 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Placed {new Date(order.placed_at).toLocaleDateString("en-US", {
            year: "numeric", month: "short", day: "numeric",
          })}
        </p>
      </div>

      <div className="flex flex-wrap gap-3">
        <StatusBadge label="Order" value={order.status} />
        <StatusBadge label="Payment" value={order.payment_status} />
      </div>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          Items
        </h2>
        <ul className="mt-4 divide-y divide-[color:var(--storefront-text,var(--ink-900))]/10 border-t border-[color:var(--storefront-text,var(--ink-900))]/10">
          {order.items.map((item, i) => (
            <OrderItemRow key={i} item={item} />
          ))}
        </ul>
      </section>

      {order.shipping_address && (
        <section>
          <h2 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
            Shipping to
          </h2>
          <address className="mt-3 text-sm not-italic leading-relaxed text-[color:var(--storefront-text,var(--ink-900))]">
            {order.shipping_address.name}
            <br />
            {order.shipping_address.line1}
            {order.shipping_address.line2 && <><br />{order.shipping_address.line2}</>}
            <br />
            {order.shipping_address.city}
            {order.shipping_address.region ? `, ${order.shipping_address.region}` : ""}
            {order.shipping_address.postal_code ? ` ${order.shipping_address.postal_code}` : ""}
            <br />
            {order.shipping_address.country_code}
          </address>
        </section>
      )}

      <OrderTimeline
        orderId={order.id}
        initialShipment={order.shipment ?? null}
        initialTimeline={order.timeline ?? []}
      />

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          Totals
        </h2>
        <dl className="mt-3 space-y-1.5 text-sm text-[color:var(--storefront-text,var(--ink-900))]">
          <TotalRow label="Subtotal" value={order.subtotal} ccy={order.currency_code} />
          <TotalRow label="Shipping" value={order.shipping_total} ccy={order.currency_code} />
          <TotalRow label="Tax" value={order.tax_total} ccy={order.currency_code} />
          <div className="mt-2 flex justify-between border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-2 text-base font-medium">
            <dt>Total</dt>
            <dd style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
              {formatCurrency(order.grand_total, order.currency_code)}
            </dd>
          </div>
          {order.refunded_amount && Number(order.refunded_amount) > 0 && (
            <div className="mt-1 flex justify-between text-sm text-[color:var(--storefront-accent,var(--moss-700))]">
              <dt>Refunded</dt>
              <dd style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                −{formatCurrency(order.refunded_amount, order.currency_code)}
              </dd>
            </div>
          )}
        </dl>
      </section>

      <DocumentsSection
        orderId={order.id}
        orderNumber={order.order_number}
        shipmentStatus={order.shipment?.status ?? null}
      />

      <CancelOrderButton
        orderId={order.id}
        orderStatus={order.status}
        shipmentStatus={order.shipment?.status ?? null}
      />
    </div>
  );
}

function DocumentsSection({
  orderId,
  orderNumber,
  shipmentStatus,
}: {
  orderId: string;
  orderNumber: string;
  shipmentStatus: string | null;
}) {
  const invoiceNumber = invoiceNumberFromOrder(orderNumber);
  const receiptNumber = receiptNumberFromOrder(orderNumber);
  // Receipts are issued post-delivery, not on payment.
  const receiptAvailable = shipmentStatus === "delivered";

  return (
    <section>
      <h2 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)] opacity-60">
        Documents
      </h2>
      <ul className="mt-3 divide-y divide-[color:var(--ink-900)]/10 border-t border-[color:var(--ink-900)]/10">
        <li className="flex items-center justify-between gap-4 py-3">
          <div className="flex flex-col">
            <span className="text-sm font-medium text-[color:var(--ink-900)]">Invoice</span>
            <span
              className="text-xs text-[color:var(--ink-900)] opacity-60"
              style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
            >
              {invoiceNumber}
            </span>
          </div>
          <a
            href={`/api/orders/${encodeURIComponent(orderId)}/invoice`}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)]/30 px-3 py-1.5 text-xs font-medium text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)]"
          >
            Download PDF
          </a>
        </li>
        <li className="flex items-center justify-between gap-4 py-3">
          <div className="flex flex-col">
            <span
              className={`text-sm font-medium ${
                receiptAvailable
                  ? "text-[color:var(--ink-900)]"
                  : "text-[color:var(--ink-900)] opacity-50"
              }`}
            >
              Receipt
            </span>
            <span
              className="text-xs text-[color:var(--ink-900)] opacity-60"
              style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
            >
              {receiptAvailable ? receiptNumber : "Available after delivery"}
            </span>
          </div>
          {receiptAvailable ? (
            <a
              href={`/api/orders/${encodeURIComponent(orderId)}/receipt`}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)]/30 px-3 py-1.5 text-xs font-medium text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)]"
            >
              Download PDF
            </a>
          ) : (
            <span className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)]/15 px-3 py-1.5 text-xs text-[color:var(--ink-900)]/40">
              Not yet available
            </span>
          )}
        </li>
      </ul>
    </section>
  );
}

function StatusBadge({ label, value }: { label: string; value: string }) {
  const tone =
    value === "paid" || value === "captured" || value === "confirmed" || value === "fulfilled"
      ? "bg-[color:var(--storefront-accent,var(--moss-700))]/10 text-[color:var(--storefront-accent,var(--moss-700))]"
      : value === "pending"
        ? "bg-amber-50 text-amber-700"
        : "bg-[color:var(--storefront-accent,var(--ink-900))]/5 text-[color:var(--storefront-text,var(--ink-900))]/70";
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${tone}`}>
      <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden />
      {label}: {value}
    </span>
  );
}

function OrderItemRow({ item }: { item: OrderItem }) {
  return (
    <li className="flex items-start gap-4 py-4">
      <div className="relative h-16 w-16 shrink-0 overflow-hidden rounded-md bg-[color:var(--storefront-background,var(--paper-200))]">
        {item.image_url ? (
          <Image
            src={item.image_url}
            alt={item.title_snapshot}
            fill
            sizes="64px"
            className="object-cover"
          />
        ) : null}
      </div>
      <div className="flex-1 text-sm">
        <p className="font-medium text-[color:var(--storefront-text,var(--ink-900))]">{item.title_snapshot}</p>
        {item.option_summary && (
          <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">{item.option_summary}</p>
        )}
        <p className="mt-1 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Qty {item.quantity} · {formatCurrency(item.unit_price, item.currency_code)}
        </p>
      </div>
      <span
        className="text-sm text-[color:var(--storefront-text,var(--ink-900))]"
        style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
      >
        {formatCurrency(item.line_total, item.currency_code)}
      </span>
    </li>
  );
}

function TotalRow({ label, value, ccy }: { label: string; value: string; ccy: string }) {
  return (
    <div className="flex justify-between">
      <dt className="opacity-70">{label}</dt>
      <dd style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
        {formatCurrency(value, ccy)}
      </dd>
    </div>
  );
}

function formatCurrency(amount: string, currencyCode: string): string {
  try {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: currencyCode,
    }).format(Number(amount));
  } catch {
    return `${currencyCode} ${amount}`;
  }
}
