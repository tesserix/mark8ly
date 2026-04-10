import { useQuery } from "@tanstack/react-query";
import { createApiClient } from "@repo/mobile-shared/api/client";
import { createOrdersApi } from "@repo/mobile-shared/api/orders";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import type { Order, OrderDetail, PaginatedResponse } from "@repo/mobile-shared/api/types";

function useApiClient() {
  const { getToken } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
  return createApiClient({
    baseUrl: "https://api.mark8ly.com", // TODO: read from config
    getToken,
    getStoreId: () => activeStore?.id ?? null,
  });
}

export function useOrders(status?: string, search?: string) {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);

  return useQuery<PaginatedResponse<Order>>({
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
