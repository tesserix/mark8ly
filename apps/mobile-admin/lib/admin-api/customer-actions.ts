import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createApiClient } from "@repo/mobile-shared/api/client";
import { createCustomersApi } from "@repo/mobile-shared/api/customers";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";

function useApiClient() {
  const { getToken } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
  return createApiClient({
    baseUrl: "https://api.mark8ly.com", // TODO: read from config
    getToken,
    getStoreId: () => activeStore?.id ?? null,
  });
}

export function useBlockCustomer() {
  const client = useApiClient();
  const customersApi = createCustomersApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => customersApi.block(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ["customers"] });
      queryClient.invalidateQueries({ queryKey: ["customer", id] });
    },
  });
}

export function useUnblockCustomer() {
  const client = useApiClient();
  const customersApi = createCustomersApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => customersApi.unblock(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ["customers"] });
      queryClient.invalidateQueries({ queryKey: ["customer", id] });
    },
  });
}
