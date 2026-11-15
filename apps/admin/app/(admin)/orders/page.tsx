import Link from "next/link";

import { AdminPage } from "@/components/layout";
import { OrdersList } from "@/components/orders/OrdersList";
import { OrdersListEmpty } from "@/components/orders/OrdersListEmpty";
import { OrdersListPagination } from "@/components/orders/OrdersListPagination";
import { listOrders, type ListOrdersQuery } from "@/lib/api/marketplace-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { ordersHref, parseOrdersQuery } from "@/lib/orders/list-query";

interface OrdersPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

const DESCRIPTION =
  "Every order placed in this store, with payment, fulfillment, and refund state at a glance.";

export default async function OrdersPage({ searchParams }: OrdersPageProps) {
  const session = await getServerSessionContext();
  const { currentStore, userId, tenantId } = session;
  const params = await searchParams;
  const query = parseOrdersQuery(params);

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
  const hasActiveFilters = !!query.status || !!query.paymentStatus || !!query.channel || !!query.search;
  const isEmpty = orders.length === 0;

  return (
    <AdminPage eyebrow="Operations" title="Orders" description={DESCRIPTION}>
      <OrdersFilterBar query={query} total={meta.total} clearable={hasActiveFilters} />
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
            buildHref={(page) => ordersHref(query, { page })}
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
  query: ListOrdersQuery;
  total: number;
  clearable: boolean;
}

function OrdersFilterBar({ query, total, clearable }: OrdersFilterBarProps) {
  const { search = "", status, paymentStatus, channel } = query;
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
        {channel && <input type="hidden" name="channel" value={channel} />}
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
            href={ordersHref(query, { search: undefined })}
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
          buildHref={(v) => ordersHref(query, { status: v as ListOrdersQuery["status"] })}
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
          buildHref={(v) => ordersHref(query, { paymentStatus: v as ListOrdersQuery["paymentStatus"] })}
        />
        <span
          aria-hidden="true"
          className="h-4 w-px shrink-0 self-center bg-border-subtle"
        />
        <FilterChipRow
          legend="Channel"
          current={channel}
          options={[
            { value: undefined, label: "All" },
            { value: "online", label: "Online" },
            { value: "in_store", label: "In-store" },
          ]}
          buildHref={(v) => ordersHref(query, { channel: v as ListOrdersQuery["channel"] })}
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
