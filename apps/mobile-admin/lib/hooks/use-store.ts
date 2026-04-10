import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import type { Store } from "@repo/mobile-shared/api/types";

const BASE_URL = "https://api.mark8ly.com"; // TODO: read from config

export function useStores() {
  const { getToken } = useAuth();

  return useQuery<Store[]>({
    queryKey: ["stores"],
    queryFn: async () => {
      const token = await getToken();
      if (!token) throw new Error("Not authenticated");

      const res = await fetch(`${BASE_URL}/api/v1/mobile/admin/stores`, {
        headers: {
          Authorization: `Bearer ${token}`,
          Accept: "application/json",
        },
      });

      if (!res.ok) {
        throw new Error(`Failed to fetch stores: ${res.statusText}`);
      }

      const data = await res.json();
      return data.items ?? data;
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
