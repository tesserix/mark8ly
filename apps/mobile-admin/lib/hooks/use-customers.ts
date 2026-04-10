import { useQuery } from "@tanstack/react-query";
import { createApiClient } from "@repo/mobile-shared/api/client";
import { createCustomersApi } from "@repo/mobile-shared/api/customers";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import type { Customer, CustomerDetail, PaginatedResponse } from "@repo/mobile-shared/api/types";

function useApiClient() {
  const { getToken } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
  return createApiClient({
    baseUrl: "https://api.mark8ly.com", // TODO: read from config
    getToken,
    getStoreId: () => activeStore?.id ?? null,
  });
}

export function useCustomers(search?: string) {
  const client = useApiClient();
  const customersApi = createCustomersApi(client);

  return useQuery<PaginatedResponse<Customer>>({
    queryKey: ["customers", search],
    queryFn: () =>
      customersApi.list({
        ...(search ? { search } : {}),
      }),
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
