import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { createCustomersApi } from "@repo/mobile-shared/api/customers";
import type { CustomerDetail } from "@repo/mobile-shared/api/types";
import type { CustomerListResponse } from "@repo/mobile-shared/api/schemas/customers";
import { useApiClient } from "@/lib/api-client";

interface CustomerListParams {
  search?: string;
  /** "active" | "blocked"; omitted means all. */
  status?: string;
}

const PAGE_SIZE = 50;

/**
 * The page param for the NEXT page, or undefined when the last page has been
 * reached (which stops infinite scroll). Exported so the off-by-one that
 * pagination hinges on can be pinned without a QueryClient.
 */
export function nextCustomerPage(lastPage: CustomerListResponse): number | undefined {
  const { page, total_pages } = lastPage.meta;
  return page < total_pages ? page + 1 : undefined;
}

/**
 * Paginated customer list. Was a single-page useQuery that only ever showed
 * the backend's first page; this walks every page via infinite scroll so a
 * store with many customers is fully browsable. `getNextPageParam` returns
 * undefined once the last page is reached, which stops fetchNextPage.
 */
export function useCustomers(params?: CustomerListParams) {
  const client = useApiClient();
  const customersApi = createCustomersApi(client);

  return useInfiniteQuery({
    queryKey: ["customers", params?.search, params?.status],
    queryFn: ({ pageParam }) =>
      customersApi.list({
        ...(params?.search ? { search: params.search } : {}),
        ...(params?.status ? { status: params.status } : {}),
        page: String(pageParam),
        page_size: String(PAGE_SIZE),
      }),
    initialPageParam: 1,
    getNextPageParam: nextCustomerPage,
    refetchOnWindowFocus: true,
  });
}

export function useCustomer(id: string) {
  const client = useApiClient();
  const customersApi = createCustomersApi(client);

  return useQuery<CustomerDetail>({
    queryKey: ["customer", id],
    queryFn: () => customersApi.get(id),
    enabled: !!id,
  });
}
