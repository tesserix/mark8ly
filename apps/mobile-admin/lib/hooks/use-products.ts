import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { createProductsApi } from "@repo/mobile-shared/api/products";
import type { ProductDetail } from "@repo/mobile-shared/api/types";
import type { ProductListResponse } from "@repo/mobile-shared/api/schemas/products";
import { useApiClient } from "@/lib/api-client";

interface ProductListParams {
  status?: string;
  search?: string;
}

// The backend default (validation.go:329-336). This hook used to pin
// page_size=100 — the backend's max — as an interim ceiling with NO
// pagination behind it, so a store with more than 100 products silently lost
// everything past the first page. That ceiling is gone now that this walks
// every page via infinite scroll; PAGE_SIZE is just the per-request size,
// matching the majority of the other paginated list hooks (customers, orders,
// reviews all use 50).
const PAGE_SIZE = 50;

/**
 * The page param for the NEXT page, or undefined once the last page has been
 * reached (which stops infinite scroll). Exported so the off-by-one that
 * pagination hinges on can be pinned without a QueryClient — mirrors
 * `nextCustomerPage` in use-customers.ts, the hook this one is modelled on.
 */
export function nextProductPage(lastPage: ProductListResponse): number | undefined {
  const { page, total_pages } = lastPage.meta;
  return page < total_pages ? page + 1 : undefined;
}

/**
 * Paginated product list. Modelled on `useCustomers` — the closest of the
 * app's nine paginated list hooks in shape (search + status filter, the same
 * `paginated({data,meta})` wire envelope) — rather than invented fresh. Was a
 * single-page useQuery capped at the backend's page_size max; this walks
 * every page via infinite scroll so a store with more products than one page
 * is fully browsable. `getNextPageParam` returns undefined once the last
 * page is reached, which stops `fetchNextPage`.
 */
export function useProducts(params?: ProductListParams) {
  const client = useApiClient();
  const productsApi = createProductsApi(client);

  return useInfiniteQuery({
    queryKey: ["products", params?.status, params?.search],
    queryFn: ({ pageParam }) =>
      productsApi.list({
        ...(params?.status ? { status: params.status } : {}),
        ...(params?.search ? { search: params.search } : {}),
        page: String(pageParam),
        page_size: String(PAGE_SIZE),
      }),
    initialPageParam: 1,
    getNextPageParam: nextProductPage,
    refetchOnWindowFocus: true,
  });
}

export function useProduct(id: string) {
  const client = useApiClient();
  const productsApi = createProductsApi(client);

  return useQuery<ProductDetail>({
    queryKey: ["product", id],
    queryFn: () => productsApi.get(id),
    enabled: !!id,
  });
}
