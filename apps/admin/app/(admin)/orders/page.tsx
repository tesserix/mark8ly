import Link from "next/link";

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
      <OrdersFilterBar
        search={query.search ?? ""}
        status={query.status}
        paymentStatus={query.paymentStatus}
        total={meta.total}
        clearable={hasActiveFilters}
      />
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

// Editorial filter bar — one hairline-underlined search input paired
// with an underline chip row for status + payment filters. Each chip
// is a plain Link that preserves the other query params, so filters
// compose cleanly and the URL stays deep-linkable.
interface OrdersFilterBarProps {
  search: string;
  status?: OrderStatus;
  paymentStatus?: PaymentStatus;
  total: number;
  clearable: boolean;
}

function OrdersFilterBar({
  search,
  status,
  paymentStatus,
  total,
  clearable,
}: OrdersFilterBarProps) {
  // Preserve the non-status params when a status chip is clicked so
  // filters compose rather than replace each other.
  const buildStatusHref = (next?: OrderStatus): string => {
    const p = new URLSearchParams();
    if (next) p.set("status", next);
    if (paymentStatus) p.set("payment_status", paymentStatus);
    if (search) p.set("search", search);
    const qs = p.toString();
    return qs ? `/orders?${qs}` : "/orders";
  };
  const buildPaymentHref = (next?: PaymentStatus): string => {
    const p = new URLSearchParams();
    if (status) p.set("status", status);
    if (next) p.set("payment_status", next);
    if (search) p.set("search", search);
    const qs = p.toString();
    return qs ? `/orders?${qs}` : "/orders";
  };

  return (
    <div className="flex flex-col gap-6">
      <form
        action="/orders"
        method="get"
        className="flex items-center gap-3 border-b border-border-subtle pb-2"
        role="search"
        aria-label="Search orders"
      >
        {status && <input type="hidden" name="status" value={status} />}
        {paymentStatus && (
          <input type="hidden" name="payment_status" value={paymentStatus} />
        )}
        <input
          type="search"
          name="search"
          defaultValue={search}
          placeholder="Search by order #, customer name or email"
          className="w-full border-none bg-transparent py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:outline-none"
        />
        <button
          type="submit"
          className="text-xs font-semibold uppercase tracking-[0.14em] text-foreground-secondary transition-colors hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Search
        </button>
        {search && (
          <Link
            href={`/orders${status || paymentStatus ? `?${new URLSearchParams({
              ...(status ? { status } : {}),
              ...(paymentStatus ? { payment_status: paymentStatus } : {}),
            }).toString()}` : ""}`}
            className="text-xs text-foreground-tertiary underline-offset-4 hover:text-foreground hover:underline"
          >
            Clear
          </Link>
        )}
      </form>

      {/* Status + Payment chips sit on the same row divided by a short
          vertical hairline. On narrow viewports the row wraps so each
          axis gets its own line without clipping. */}
      <div className="flex flex-wrap items-baseline gap-x-5 gap-y-2">
        <FilterChipRow
          legend="Status"
          current={status}
          options={[
            { value: undefined, label: "All" },
            { value: "pending", label: "Pending" },
            { value: "confirmed", label: "Confirmed" },
            { value: "fulfilled", label: "Fulfilled" },
            { value: "cancelled", label: "Cancelled" },
          ]}
          buildHref={buildStatusHref as (v: string | undefined) => string}
        />
        <span
          aria-hidden="true"
          className="h-4 w-px shrink-0 self-center bg-border-subtle"
        />
        <FilterChipRow
          legend="Payment"
          current={paymentStatus}
          options={[
            { value: undefined, label: "All" },
            { value: "pending", label: "Pending" },
            { value: "authorized", label: "Authorized" },
            { value: "paid", label: "Paid" },
            { value: "failed", label: "Failed" },
            { value: "partially_refunded", label: "Partial refund" },
            { value: "refunded", label: "Refunded" },
          ]}
          buildHref={buildPaymentHref as (v: string | undefined) => string}
        />
        {clearable && (
          <span className="ml-auto flex items-center gap-3 text-xs text-foreground-tertiary">
            <span className="tabular-nums">
              {total} {total === 1 ? "result" : "results"}
            </span>
            <span aria-hidden="true">·</span>
            <Link
              href="/orders"
              className="text-[color:var(--moss-700)] underline-offset-4 hover:underline"
            >
              Clear all
            </Link>
          </span>
        )}
      </div>
    </div>
  );
}

interface FilterChipOption {
  value: string | undefined;
  label: string;
}

function FilterChipRow({
  legend,
  current,
  options,
  buildHref,
}: {
  legend: string;
  current: string | undefined;
  options: FilterChipOption[];
  buildHref: (v: string | undefined) => string;
}) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
      <span className="shrink-0 text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
        {legend}
      </span>
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        {options.map((opt) => {
          const isCurrent = current === opt.value;
          return (
            <Link
              key={opt.label}
              href={buildHref(opt.value)}
              aria-current={isCurrent ? "true" : undefined}
              className={
                "text-xs transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] " +
                (isCurrent
                  ? "text-foreground underline underline-offset-[6px] decoration-[1.5px]"
                  : "text-foreground-tertiary hover:text-foreground")
              }
            >
              {opt.label}
            </Link>
          );
        })}
      </div>
    </div>
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
