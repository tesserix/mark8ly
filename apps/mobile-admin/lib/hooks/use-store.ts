import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import type { PaginatedResponse, Store } from "@repo/mobile-shared/api/types";
import { useApiClient } from "@/lib/api-client";

/**
 * Lists every store the signed-in user has any role on. This is the
 * mobile equivalent of the admin web's `/api/v1/users/me/tenants` —
 * the membership probe used by the post-login resolver and by the
 * in-app store switcher. Routed through the shared api-client so 401
 * (token expired) and tenant-invalid responses self-correct.
 */
export function useStores() {
  const client = useApiClient();
  return useQuery<Store[]>({
    queryKey: ["stores"],
    queryFn: async () => {
      const data = await client.getTenant<PaginatedResponse<Store> | Store[]>("/stores");
      if (Array.isArray(data)) return data;
      return data.items ?? [];
    },
  });
}

export function useSwitchStore() {
  const setActiveStore = useTenantStore((s) => s.setActiveStore);
  const queryClient = useQueryClient();

  return useCallback(
    (store: Store) => {
      setActiveStore(store);
      queryClient.invalidateQueries();
    },
    [setActiveStore, queryClient],
  );
}
