import { cookies, headers } from "next/headers";
import Link from "next/link";
import { resolveStoreSlug } from "@/lib/slug";
import { decodeSession } from "@/lib/auth";

export const metadata = {
  title: "My Orders",
};

// The account orders list must re-query marketplace-api on every
// request — Next.js otherwise caches the first render per-tenant and
// new purchases never show up until a redeploy.
export const dynamic = "force-dynamic";
export const revalidate = 0;

interface OrderSummary {
  id: string;
  order_number: string;
  status: string;
  payment_status: string;
  grand_total: string;
  currency_code: string;
  placed_at: string;
}

interface OrdersResponse {
  data: OrderSummary[];
  meta?: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
}

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function statusLabel(status: string): string {
  const labels: Record<string, string> = {
    pending: "Pending",
    confirmed: "Confirmed",
    processing: "Processing",
    fulfilled: "Fulfilled",
    cancelled: "Cancelled",
    refunded: "Refunded",
  };
  return labels[status] ?? status;
}

import { StatusChip, type StatusTone } from "@/components/ui/StatusChip";

function statusTone(status: string): StatusTone {
  switch (status) {
    case "pending":
      return "warning";
    case "confirmed":
    case "processing":
      return "info";
    case "fulfilled":
      return "success";
    case "refunded":
      return "danger";
    case "cancelled":
    default:
      return "neutral";
  }
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
    });
  } catch {
    return iso;
  }
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

interface OrdersPageProps {
  searchParams: Promise<{ q?: string }>;
}

export default async function OrdersPage({ searchParams }: OrdersPageProps) {
  const { q: searchRaw } = await searchParams;
  const search = (searchRaw ?? "").trim().toLowerCase();
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value ?? "";
  const session = decodeSession(sessionCookie);

  if (!session) {
    return (
      <div className="space-y-2">
        <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          Orders
        </h1>
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Please sign in to view your orders.
        </p>
      </div>
    );
  }

  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    await resolveStoreSlug(host);

  let orders: OrderSummary[] = [];
  let fetchError = false;

  try {
    const apiHeaders: Record<string, string> = {
      Accept: "application/json",
      Cookie: `mp_customer_session=${sessionCookie}`,
    };
    if (STOREFRONT_KEY) {
      apiHeaders["X-Storefront-Key"] = STOREFRONT_KEY;
    }

    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/account/orders`,
      {
        headers: apiHeaders,
        cache: "no-store",
      },
    );

    if (res.ok) {
      const body = (await res.json()) as OrdersResponse;
      orders = body.data ?? [];
    } else {
      fetchError = true;
    }
  } catch {
    fetchError = true;
  }

  // Customers typically have a short list, so filter client-side on the
  // SSR render rather than adding a new backend search endpoint. Matches
  // against order number, status label, and payment status — the fields
  // a customer is likely to remember or type.
  const visible = search
    ? orders.filter((o) => {
        const hay = [
          o.order_number,
          statusLabel(o.status),
          o.payment_status,
        ]
          .filter(Boolean)
          .join(" ")
          .toLowerCase();
        return hay.includes(search);
      })
    : orders;

  return (
    <div className="space-y-6">
      <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
        Orders
      </h1>

      <form
        action="/account/orders"
        method="get"
        role="search"
        aria-label="Search orders"
        className="flex items-center gap-2"
      >
        <input
          type="search"
          name="q"
          defaultValue={searchRaw ?? ""}
          placeholder="Search by order #"
          className="w-full max-w-sm rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-transparent px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus:outline-2 focus:outline-offset-2 focus:outline-[color:var(--storefront-accent,var(--moss-700))]"
        />
        <button
          type="submit"
          className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/25 px-3 py-2 text-xs font-medium uppercase tracking-wider text-[color:var(--storefront-text,var(--ink-900))] hover:bg-[color:var(--storefront-text,var(--ink-900))]/5"
        >
          Search
        </button>
        {searchRaw && (
          <a
            href="/account/orders"
            className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50 hover:opacity-100"
          >
            Clear
          </a>
        )}
      </form>

      {fetchError && (
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Unable to load your orders right now. Please try again later.
        </p>
      )}

      {!fetchError && orders.length === 0 && (
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          You have not placed any orders yet.
        </p>
      )}

      {orders.length > 0 && visible.length === 0 && (
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          No orders match &ldquo;{searchRaw}&rdquo;.
        </p>
      )}

      {visible.length > 0 && (
        <ul className="divide-y divide-[color:var(--storefront-text,var(--ink-900))]/10 border-t border-[color:var(--storefront-text,var(--ink-900))]/10">
          {visible.map((order) => (
            <li key={order.id}>
              <Link
                href={`/account/orders/${order.id}`}
                className="group flex items-baseline justify-between gap-4 py-5 transition-opacity hover:opacity-75"
              >
                <div className="min-w-0 space-y-1">
                  <div className="flex items-center gap-3">
                    <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
                      #{order.order_number}
                    </span>
                    <StatusChip tone={statusTone(order.status)} size="md">
                      {statusLabel(order.status)}
                    </StatusChip>
                  </div>
                  <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
                    {formatDate(order.placed_at)}
                  </p>
                </div>

                <span className="shrink-0 font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
                  {formatCurrency(order.grand_total, order.currency_code)}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
