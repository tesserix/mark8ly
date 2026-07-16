import { useQuery } from "@tanstack/react-query";
import { createOrdersApi } from "@repo/mobile-shared/api/orders";
import type { OrderDetail } from "@repo/mobile-shared/api/types";
import type { OrderListResponse } from "@repo/mobile-shared/api/schemas/orders";
import { useApiClient } from "@/lib/api-client";

export function useOrders(status?: string, search?: string) {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);

  return useQuery<OrderListResponse>({
    queryKey: ["orders", status, search],
    queryFn: () =>
      ordersApi.list({
        ...(status ? { status } : {}),
        ...(search ? { search } : {}),
      }),
    refetchOnWindowFocus: true,
  });
}

export function useOrder(id: string) {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);

  return useQuery<OrderDetail>({
    queryKey: ["orders", id],
    queryFn: () => ordersApi.get(id),
    enabled: !!id,
  });
}
