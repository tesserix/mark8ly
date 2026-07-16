import { useQuery } from "@tanstack/react-query";
import { createCustomersApi } from "@repo/mobile-shared/api/customers";
import type { CustomerDetail } from "@repo/mobile-shared/api/types";
import type { CustomerListResponse } from "@repo/mobile-shared/api/schemas/customers";
import { useApiClient } from "@/lib/api-client";

export function useCustomers(search?: string) {
  const client = useApiClient();
  const customersApi = createCustomersApi(client);

  return useQuery<CustomerListResponse>({
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
