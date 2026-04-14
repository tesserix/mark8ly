// /account/orders/[id] — customer-facing single order detail.
// Reuses fetchOrder + the same visual layout as the post-checkout
// /orders/[id] page but rendered inside the account shell and with
// "Back to orders" nav instead of the store header.

import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { cookies, headers } from "next/headers";

import { slugFromHost } from "@/lib/slug";
import { decodeSession } from "@/lib/session";
import { fetchOrder, type Order, type OrderItem } from "@/lib/api/checkout-api";

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
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
          Order
        </h1>
        <p className="text-sm text-[color:var(--ink-900)] opacity-50">
          Please sign in to view this order.
        </p>
      </div>
    );
  }

  const h = await headers();
  const host = h.get("host");
  const slug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "";
  if (!slug || !id) notFound();

  const order = await fetchOrder(slug, id);
  if (!order) notFound();

  return (
    <div className="space-y-6">
      <div>
        <Link
          href="/account/orders"
          className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--ink-900)] opacity-60 transition-opacity hover:opacity-100"
        >
          ← All orders
        </Link>
        <h1 className="mt-2 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
          {order.order_number}
        </h1>
        <p className="mt-1 text-xs text-[color:var(--ink-900)] opacity-50">
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
        <h2 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)] opacity-60">
          Items
        </h2>
        <ul className="mt-4 divide-y divide-[color:var(--ink-900)]/10 border-t border-[color:var(--ink-900)]/10">
          {order.items.map((item, i) => (
            <OrderItemRow key={i} item={item} />
          ))}
        </ul>
      </section>

      {order.shipping_address && (
        <section>
          <h2 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)] opacity-60">
            Shipping to
          </h2>
          <address className="mt-3 text-sm not-italic leading-relaxed text-[color:var(--ink-900)]">
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

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)] opacity-60">
          Totals
        </h2>
        <dl className="mt-3 space-y-1.5 text-sm text-[color:var(--ink-900)]">
          <TotalRow label="Subtotal" value={order.subtotal} ccy={order.currency_code} />
          <TotalRow label="Shipping" value={order.shipping_total} ccy={order.currency_code} />
          <TotalRow label="Tax" value={order.tax_total} ccy={order.currency_code} />
          <div className="mt-2 flex justify-between border-t border-[color:var(--ink-900)]/10 pt-2 text-base font-medium">
            <dt>Total</dt>
            <dd style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
              {formatCurrency(order.grand_total, order.currency_code)}
            </dd>
          </div>
        </dl>
      </section>
    </div>
  );
}

function StatusBadge({ label, value }: { label: string; value: string }) {
  const tone =
    value === "paid" || value === "captured" || value === "confirmed" || value === "fulfilled"
      ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]"
      : value === "pending"
        ? "bg-amber-50 text-amber-700"
        : "bg-[color:var(--ink-900)]/5 text-[color:var(--ink-900)]/70";
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
      <div className="relative h-16 w-16 shrink-0 overflow-hidden rounded-md bg-[color:var(--paper-200)]">
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
        <p className="font-medium text-[color:var(--ink-900)]">{item.title_snapshot}</p>
        {item.option_summary && (
          <p className="text-xs text-[color:var(--ink-900)] opacity-50">{item.option_summary}</p>
        )}
        <p className="mt-1 text-xs text-[color:var(--ink-900)] opacity-50">
          Qty {item.quantity} · {formatCurrency(item.unit_price, item.currency_code)}
        </p>
      </div>
      <span
        className="text-sm text-[color:var(--ink-900)]"
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
