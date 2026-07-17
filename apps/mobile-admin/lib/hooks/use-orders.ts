import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { createOrdersApi } from "@repo/mobile-shared/api/orders";
import type { OrderDetail } from "@repo/mobile-shared/api/types";
import type { OrderListResponse } from "@repo/mobile-shared/api/schemas/orders";
import { useApiClient } from "@/lib/api-client";

interface OrderListParams {
  status?: string;
  payment_status?: string;
  search?: string;
}

const PAGE_SIZE = 50;

/** Next page param, or undefined on the last page (stops infinite scroll). */
export function nextOrderPage(lastPage: OrderListResponse): number | undefined {
  const { page, total_pages } = lastPage.meta;
  return page < total_pages ? page + 1 : undefined;
}

export function useOrders(params?: OrderListParams) {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);

  return useInfiniteQuery({
    queryKey: ["orders", "list", params?.status, params?.payment_status, params?.search],
    queryFn: ({ pageParam }) =>
      ordersApi.list({
        ...(params?.status ? { status: params.status } : {}),
        ...(params?.payment_status ? { payment_status: params.payment_status } : {}),
        ...(params?.search ? { search: params.search } : {}),
        page: String(pageParam),
        page_size: String(PAGE_SIZE),
      }),
    initialPageParam: 1,
    getNextPageParam: nextOrderPage,
    refetchOnWindowFocus: true,
  });
}

export function useOrder(id: string) {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);

  // Detail key stays under the ["orders"] prefix so an order mutation's
  // invalidateQueries(["orders"]) refetches both this and the list.
  return useQuery<OrderDetail>({
    queryKey: ["orders", "detail", id],
    queryFn: () => ordersApi.get(id),
    enabled: !!id,
  });
}
