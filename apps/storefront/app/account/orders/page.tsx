import { cookies, headers } from "next/headers";
import Link from "next/link";
import { slugFromHost } from "@/lib/slug";
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

function statusClasses(status: string): string {
  switch (status) {
    case "pending":
      return "bg-amber-50 text-amber-800 border-amber-200";
    case "confirmed":
    case "processing":
      return "bg-blue-50 text-blue-800 border-blue-200";
    case "fulfilled":
      return "bg-emerald-50 text-emerald-800 border-emerald-200";
    case "cancelled":
      return "bg-neutral-100 text-neutral-500 border-neutral-200";
    case "refunded":
      return "bg-red-50 text-red-700 border-red-200";
    default:
      return "bg-neutral-100 text-neutral-600 border-neutral-200";
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

export default async function OrdersPage() {
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value ?? "";
  const session = decodeSession(sessionCookie);

  if (!session) {
    return (
      <div className="space-y-2">
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
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
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "default";

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

  return (
    <div className="space-y-6">
      <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
        Orders
      </h1>

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

      {orders.length > 0 && (
        <ul className="divide-y divide-[color:var(--storefront-text,var(--ink-900))]/10 border-t border-[color:var(--storefront-text,var(--ink-900))]/10">
          {orders.map((order) => (
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
                    <span
                      className={`inline-block rounded-sm border px-2 py-0.5 text-xs font-medium leading-tight ${statusClasses(order.status)}`}
                    >
                      {statusLabel(order.status)}
                    </span>
                  </div>
                  <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
                    {formatDate(order.placed_at)}
                  </p>
                </div>

                <span className="shrink-0 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
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
