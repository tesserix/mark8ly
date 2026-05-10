import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createCustomersApi } from "@repo/mobile-shared/api/customers";
import { useApiClient } from "@/lib/api-client";

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
