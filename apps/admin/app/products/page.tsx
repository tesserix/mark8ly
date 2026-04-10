import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import {
  listProducts,
  listCategories,
  type ListProductsQuery,
  type AdminStore,
} from "@/lib/api/marketplace-api";

import { ProductsListHeader } from "@/components/products/ProductsListHeader";
import { ProductsListFilters } from "@/components/products/ProductsListFilters";
import { ProductsListSummary } from "@/components/products/ProductsListSummary";
import { ProductsList } from "@/components/products/ProductsList";
import { ProductsListPagination } from "@/components/products/ProductsListPagination";
import { ProductsListEmpty } from "@/components/products/ProductsListEmpty";

interface ProductsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function ProductsPage({
  searchParams,
}: ProductsPageProps) {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId, stores } = session;
  const params = await searchParams;

  const query = parseSearchParams(params);
  const canCreate = role === "owner" || role === "admin";

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="flex flex-col gap-6 px-8 py-6">
          <ProductsListHeader canCreate={false} />
          <ProductsListEmpty variant="no-products" />
        </main>
      </AdminShell>
    );
  }

  const [response, categories] = await Promise.all([
    listProducts(currentStore.id, query, { userId, tenantId }),
    listCategories(currentStore.id, { userId, tenantId }),
  ]);

  const products = response?.data ?? [];

  // Map platform-api Store[] to AdminStore[] for the copy dialog
  const adminStores: AdminStore[] = stores.map((s) => ({
    id: s.id,
    name: s.name,
    slug: s.slug,
    country_code: s.country_code,
    currency_code: s.currency_code,
    status: s.status,
  }));
  const meta = response?.meta ?? {
    page: 1,
    page_size: query.pageSize ?? 20,
    total: 0,
    total_pages: 0,
  };
  const hasActiveFilters = !!query.status || !!query.search;
  const isEmpty = products.length === 0;

  const buildHref = (page: number) => {
    const p = new URLSearchParams();
    if (query.status) p.set("status", query.status);
    if (query.search) p.set("search", query.search);
    if (query.pageSize) p.set("page_size", String(query.pageSize));
    if (page > 1) p.set("page", String(page));
    const qs = p.toString();
    return qs ? `/products?${qs}` : "/products";
  };

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main
        className="flex flex-col gap-6 px-8 py-6"
        aria-labelledby="products-heading"
      >
        <ProductsListHeader canCreate={canCreate} />
        <div className="flex flex-col gap-4">
          <ProductsListFilters />
          <ProductsListSummary meta={meta} statusFilter={query.status} />
        </div>

        {isEmpty ? (
          <ProductsListEmpty
            variant={hasActiveFilters ? "no-matches" : "no-products"}
            clearFiltersHref={hasActiveFilters ? "/products" : undefined}
          />
        ) : (
          <>
            <ProductsList
              products={products}
              storeId={currentStore.id}
              role={role}
              stores={adminStores}
              categories={categories}
            />
            <ProductsListPagination
              currentPage={meta.page}
              totalPages={meta.total_pages}
              buildHref={buildHref}
            />
          </>
        )}
      </main>
    </AdminShell>
  );
}

function parseSearchParams(
  raw: Record<string, string | string[] | undefined>,
): ListProductsQuery {
  const status = typeof raw.status === "string" ? raw.status : undefined;
  const search = typeof raw.search === "string" ? raw.search : undefined;
  const page =
    typeof raw.page === "string" ? Number.parseInt(raw.page, 10) : undefined;
  const pageSize =
    typeof raw.page_size === "string"
      ? Number.parseInt(raw.page_size, 10)
      : undefined;
  const validStatus =
    status === "draft" || status === "active" || status === "archived"
      ? status
      : undefined;
  return {
    status: validStatus,
    search: search || undefined,
    page: page && page > 0 ? page : undefined,
    pageSize: pageSize && pageSize > 0 && pageSize <= 100 ? pageSize : undefined,
  };
}
