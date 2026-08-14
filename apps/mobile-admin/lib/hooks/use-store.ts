import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { storesResponseSchema } from "@repo/mobile-shared/api/schemas/stores";
import { ApiError } from "@repo/mobile-shared/api/client";
import type { Store } from "@repo/mobile-shared/api/types";
import { useApiClient } from "@/lib/api-client";

// Longer than the app-wide budget of 2: every screen waits on this one call,
// and a backend restart outlasts two attempts. A denial or a contract
// mismatch is terminal — retrying either just fails again.
export function shouldRetryStores(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && (error.status === 403 || error.code === "contract_mismatch")) {
    return false;
  }
  return failureCount < 5;
}

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
      const res = await client.getTenant("/stores", undefined, storesResponseSchema);
      return res.data;
    },
    retry: shouldRetryStores,
    retryDelay: (attempt) => Math.min(500 * 2 ** attempt, 8_000),
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
