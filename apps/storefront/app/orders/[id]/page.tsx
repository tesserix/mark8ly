// apps/storefront/app/orders/[id]/page.tsx
//
// Order confirmation / status page. Server component. Resolves the
// store from the host, fetches the order by ID, and renders a
// read-only summary: order number, status, items, shipping address,
// payment status, and totals breakdown.

import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { headers } from "next/headers";

import { slugFromHost } from "@/lib/slug";
import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { fetchOrder, type Order, type OrderItem } from "@/lib/api/checkout-api";
import { StorefrontNav } from "@/components/StorefrontNav";
import { PaymentPrompt } from "@/components/PaymentPrompt";
import { OrderTimeline } from "@/components/OrderTimeline";

export const dynamic = "force-dynamic";

interface PageProps {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ slug?: string }>;
}

async function resolveSlug(query: { slug?: string }): Promise<string> {
  const h = await headers();
  const host = h.get("host");
  return (
    query.slug ||
    slugFromHost(host) ||
    process.env.DEFAULT_STORE_SLUG ||
    ""
  );
}

export default async function OrderPage({ params, searchParams }: PageProps) {
  const [{ id }, query] = await Promise.all([params, searchParams]);
  const slug = await resolveSlug(query);

  if (!slug || !id) {
    notFound();
  }

  const [order, store] = await Promise.all([
    fetchOrder(slug, id),
    fetchStoreBySlug(slug).catch(() => null),
  ]);
  if (!order) {
    notFound();
  }

  return (
    <main id="main" className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <div className="mx-auto max-w-3xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store?.name ?? ""} />
        <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-accent,var(--moss-700))]">
          Thank you for your order
        </p>
        <h1 className="mt-2 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl text-[color:var(--storefront-text,var(--ink-900))]">
          {order.order_number}
        </h1>
        <p className="mt-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          We&apos;ve received your order and will send you a confirmation email shortly.
        </p>

        {/* Status badges */}
        <div className="mt-4 flex flex-wrap gap-3">
          <StatusBadge label="Order" value={order.status} />
          <StatusBadge label="Payment" value={order.payment_status} />
        </div>

        <PaymentPrompt
          orderId={order.id}
          paymentStatus={order.payment_status}
          storeName={store?.name ?? ""}
        />

        <OrderTimeline
          orderId={order.id}
          initialShipment={order.shipment ?? null}
          initialTimeline={order.timeline ?? []}
        />

        {/* Items */}
        <section aria-labelledby="items-heading" className="mt-10">
          <h2
            id="items-heading"
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60"
          >
            Items
          </h2>
          <ul className="mt-4 divide-y divide-[color:var(--storefront-text,var(--ink-900))]/10">
            {order.items.map((item, i) => (
              <OrderItemRow key={i} item={item} />
            ))}
          </ul>
        </section>

        {/* Shipping address */}
        {order.shipping_address && (
          <section aria-labelledby="shipping-heading" className="mt-10">
            <h2
              id="shipping-heading"
              className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60"
            >
              Shipping address
            </h2>
            <address className="mt-4 text-sm not-italic leading-relaxed text-[color:var(--storefront-text,var(--ink-900))]">
              {order.shipping_address.name}
              <br />
              {order.shipping_address.line1}
              {order.shipping_address.line2 && (
                <>
                  <br />
                  {order.shipping_address.line2}
                </>
              )}
              <br />
              {order.shipping_address.city}
              {order.shipping_address.region && `, ${order.shipping_address.region}`}
              {order.shipping_address.postal_code && ` ${order.shipping_address.postal_code}`}
              <br />
              {order.shipping_address.country_code}
            </address>
          </section>
        )}

        {/* Totals */}
        <section aria-labelledby="totals-heading" className="mt-10 border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-6">
          <h2 id="totals-heading" className="sr-only">
            Order totals
          </h2>
          <dl className="space-y-2 text-sm text-[color:var(--storefront-text,var(--ink-900))]">
            <div className="flex justify-between">
              <dt className="opacity-60">Subtotal</dt>
              <dd style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                {formatPrice(order.subtotal, order.currency_code)}
              </dd>
            </div>
            <div className="flex justify-between">
              <dt className="opacity-60">Shipping</dt>
              <dd style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                {formatPrice(order.shipping_total, order.currency_code)}
              </dd>
            </div>
            <div className="flex justify-between">
              <dt className="opacity-60">Tax</dt>
              <dd style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                {formatPrice(order.tax_total, order.currency_code)}
              </dd>
            </div>
            <div className="flex justify-between border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-2 font-medium">
              <dt>Total</dt>
              <dd
                className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg"
                style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
              >
                {formatPrice(order.grand_total, order.currency_code)}
              </dd>
            </div>
          </dl>
        </section>

        {/* CTA */}
        <div className="mt-10">
          <Link
            href="/products"
            className="inline-block rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] transition-all duration-150 hover:opacity-90 active:scale-[0.99] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
          >
            Continue shopping
          </Link>
        </div>
      </div>
    </main>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function StatusBadge({ label, value }: { label: string; value: string }) {
  const humanized = value
    .replace(/_/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-[color:var(--storefront-text,var(--ink-900))]/15 px-3 py-1 text-xs text-[color:var(--storefront-text,var(--ink-900))]">
      <span className="opacity-50">{label}:</span>
      <span className="font-medium">{humanized}</span>
    </span>
  );
}

function OrderItemRow({ item }: { item: OrderItem }) {
  return (
    <li className="flex gap-4 py-4">
      {item.image_url ? (
        <div className="relative h-16 w-16 shrink-0 overflow-hidden rounded-md bg-[color:var(--storefront-background,var(--paper-200))]">
          <Image
            src={item.image_url}
            alt={item.title_snapshot}
            fill
            sizes="64px"
            className="object-cover"
          />
        </div>
      ) : (
        <div className="h-16 w-16 shrink-0 rounded-md bg-[color:var(--storefront-accent,var(--ink-900))]/5" />
      )}
      <div className="flex flex-1 items-start justify-between">
        <div>
          <p className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
            {item.title_snapshot}
          </p>
          {item.option_summary && (
            <p className="mt-0.5 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
              {item.option_summary}
            </p>
          )}
          <p className="mt-0.5 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
            Qty {item.quantity}
          </p>
        </div>
        <p
          className="text-sm text-[color:var(--storefront-text,var(--ink-900))]"
          style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
        >
          {formatPrice(item.line_total, item.currency_code)}
        </p>
      </div>
    </li>
  );
}

function formatPrice(amount: string, currencyCode: string): string {
  const n = Number.parseFloat(amount);
  if (!Number.isFinite(n)) return `${currencyCode} ${amount}`;
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
    }).format(n);
  } catch {
    return `${currencyCode} ${amount}`;
  }
}
