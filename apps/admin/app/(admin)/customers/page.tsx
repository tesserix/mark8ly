import { AdminPage } from "@/components/layout";
import { CustomersList } from "@/components/customers/CustomersList";
import { CustomersListEmpty } from "@/components/customers/CustomersListEmpty";
import { CustomersListFilters } from "@/components/customers/CustomersListFilters";
import { CustomersListPagination } from "@/components/customers/CustomersListPagination";
import {
  listCustomers,
  type CustomerStatus,
  type ListCustomersQuery,
} from "@/lib/api/marketplace-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

interface CustomersPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

const DESCRIPTION =
  "Everyone who has shopped at your store. View order history, manage tags, and handle account status.";

export default async function CustomersPage({
  searchParams,
}: CustomersPageProps) {
  const session = await getServerSessionContext();
  const { currentStore, userId, tenantId } = session;
  const params = await searchParams;
  const query = parseSearchParams(params);

  if (!currentStore) {
    return (
      <AdminPage
        eyebrow="Relationships"
        title="Customers"
        description={DESCRIPTION}
      >
        <CustomersListEmpty variant="no-customers" />
      </AdminPage>
    );
  }

  const response = await listCustomers(currentStore.id, query, {
    userId,
    tenantId,
  });

  const customers = response?.data ?? [];
  const meta = response?.meta ?? {
    page: 1,
    page_size: query.pageSize ?? 50,
    total: 0,
    total_pages: 0,
  };
  const hasActiveFilters = !!query.search || !!query.status || !!query.tag;
  const isEmpty = customers.length === 0;

  const buildHref = (page: number) => {
    const p = new URLSearchParams();
    if (query.search) p.set("search", query.search);
    if (query.status) p.set("status", query.status);
    if (query.tag) p.set("tag", query.tag);
    if (query.sortBy) p.set("sort_by", query.sortBy);
    if (query.sortDir) p.set("sort_dir", query.sortDir);
    if (query.pageSize) p.set("page_size", String(query.pageSize));
    if (page > 1) p.set("page", String(page));
    const qs = p.toString();
    return qs ? `/customers?${qs}` : "/customers";
  };

  return (
    <AdminPage
      eyebrow="Relationships"
      title="Customers"
      description={DESCRIPTION}
    >
      <CustomersListFilters />
      {isEmpty ? (
        <CustomersListEmpty
          variant={hasActiveFilters ? "no-matches" : "no-customers"}
          clearFiltersHref={hasActiveFilters ? "/customers" : undefined}
        />
      ) : (
        <>
          <CustomersList
            customers={customers}
            currency={currentStore.currency_code}
          />
          <CustomersListPagination
            currentPage={meta.page}
            totalPages={meta.total_pages}
            buildHref={buildHref}
          />
        </>
      )}
    </AdminPage>
  );
}

const VALID_CUSTOMER_STATUS: readonly CustomerStatus[] = ["active", "blocked"];

function parseSearchParams(
  raw: Record<string, string | string[] | undefined>,
): ListCustomersQuery {
  const search = typeof raw.search === "string" ? raw.search : undefined;
  const status = typeof raw.status === "string" ? raw.status : undefined;
  const tag = typeof raw.tag === "string" ? raw.tag : undefined;
  const sortBy = typeof raw.sort_by === "string" ? raw.sort_by : undefined;
  const sortDir = typeof raw.sort_dir === "string" ? raw.sort_dir : undefined;
  const page =
    typeof raw.page === "string" ? Number.parseInt(raw.page, 10) : undefined;
  const pageSize =
    typeof raw.page_size === "string"
      ? Number.parseInt(raw.page_size, 10)
      : undefined;

  const validStatus = VALID_CUSTOMER_STATUS.includes(status as CustomerStatus)
    ? (status as CustomerStatus)
    : undefined;

  return {
    search: search || undefined,
    status: validStatus,
    tag: tag || undefined,
    sortBy: sortBy || undefined,
    sortDir: sortDir || undefined,
    page: page && page > 0 ? page : undefined,
    pageSize:
      pageSize && pageSize > 0 && pageSize <= 200 ? pageSize : undefined,
  };
}
