import { AdminPage } from "@/components/layout";
import { OrdersList } from "@/components/orders/OrdersList";
import { OrdersListEmpty } from "@/components/orders/OrdersListEmpty";
import { OrdersListPagination } from "@/components/orders/OrdersListPagination";
import {
  listOrders,
  type ListOrdersQuery,
  type OrderStatus,
  type PaymentStatus,
} from "@/lib/api/marketplace-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

interface OrdersPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

const DESCRIPTION =
  "Every order placed in this store, with payment, fulfillment, and refund state at a glance.";

export default async function OrdersPage({ searchParams }: OrdersPageProps) {
  const session = await getServerSessionContext();
  const { currentStore, userId, tenantId } = session;
  const params = await searchParams;
  const query = parseSearchParams(params);

  if (!currentStore) {
    return (
      <AdminPage eyebrow="Operations" title="Orders" description={DESCRIPTION}>
        <OrdersListEmpty variant="no-orders" />
      </AdminPage>
    );
  }

  const response = await listOrders(currentStore.id, query, {
    userId,
    tenantId,
  });

  const orders = response?.data ?? [];
  const meta = response?.meta ?? {
    page: 1,
    page_size: query.pageSize ?? 50,
    total: 0,
    total_pages: 0,
  };
  const hasActiveFilters = !!query.status || !!query.paymentStatus || !!query.search;
  const isEmpty = orders.length === 0;

  const buildHref = (page: number) => {
    const p = new URLSearchParams();
    if (query.status) p.set("status", query.status);
    if (query.paymentStatus) p.set("payment_status", query.paymentStatus);
    if (query.search) p.set("search", query.search);
    if (query.pageSize) p.set("page_size", String(query.pageSize));
    if (page > 1) p.set("page", String(page));
    const qs = p.toString();
    return qs ? `/orders?${qs}` : "/orders";
  };

  return (
    <AdminPage eyebrow="Operations" title="Orders" description={DESCRIPTION}>
      <OrdersSearchBar defaultValue={query.search ?? ""} />
      {isEmpty ? (
        <OrdersListEmpty
          variant={hasActiveFilters ? "no-matches" : "no-orders"}
          clearFiltersHref={hasActiveFilters ? "/orders" : undefined}
        />
      ) : (
        <>
          <OrdersList orders={orders} />
          <OrdersListPagination
            currentPage={meta.page}
            totalPages={meta.total_pages}
            buildHref={buildHref}
          />
        </>
      )}
    </AdminPage>
  );
}

// Plain GET form so the search survives hard reload + deep-links. The
// preserved inputs carry existing status/payment_status through so
// searching inside a filtered view doesn't silently clear the filter.
function OrdersSearchBar({ defaultValue }: { defaultValue: string }) {
  return (
    <form
      action="/orders"
      method="get"
      className="mb-4 flex items-center gap-2"
      role="search"
      aria-label="Search orders"
    >
      <input
        type="search"
        name="search"
        defaultValue={defaultValue}
        placeholder="Search by order #, customer name or email"
        className="w-full max-w-md rounded-md border border-border-subtle bg-background px-3 py-2 text-sm placeholder:text-foreground-tertiary focus:border-foreground focus:outline-none"
      />
      <button
        type="submit"
        className="rounded-md bg-foreground px-4 py-2 text-sm font-medium text-background hover:opacity-90"
      >
        Search
      </button>
      {defaultValue && (
        <a
          href="/orders"
          className="text-xs text-foreground-tertiary hover:text-foreground"
        >
          Clear
        </a>
      )}
    </form>
  );
}

const VALID_ORDER_STATUS: readonly OrderStatus[] = [
  "pending",
  "confirmed",
  "fulfilled",
  "cancelled",
];
const VALID_PAYMENT_STATUS: readonly PaymentStatus[] = [
  "pending",
  "authorized",
  "paid",
  "failed",
  "refunded",
  "partially_refunded",
];

function parseSearchParams(
  raw: Record<string, string | string[] | undefined>,
): ListOrdersQuery {
  const status = typeof raw.status === "string" ? raw.status : undefined;
  const paymentStatus =
    typeof raw.payment_status === "string" ? raw.payment_status : undefined;
  const page =
    typeof raw.page === "string" ? Number.parseInt(raw.page, 10) : undefined;
  const pageSize =
    typeof raw.page_size === "string"
      ? Number.parseInt(raw.page_size, 10)
      : undefined;

  const validStatus = VALID_ORDER_STATUS.includes(status as OrderStatus)
    ? (status as OrderStatus)
    : undefined;
  const validPaymentStatus = VALID_PAYMENT_STATUS.includes(
    paymentStatus as PaymentStatus,
  )
    ? (paymentStatus as PaymentStatus)
    : undefined;

  const search = typeof raw.search === "string" ? raw.search.trim() : "";
  return {
    status: validStatus,
    paymentStatus: validPaymentStatus,
    search: search || undefined,
    page: page && page > 0 ? page : undefined,
    pageSize:
      pageSize && pageSize > 0 && pageSize <= 200 ? pageSize : undefined,
  };
}
