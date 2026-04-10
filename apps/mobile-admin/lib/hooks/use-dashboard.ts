import { useQuery } from "@tanstack/react-query";
import { createApiClient } from "@repo/mobile-shared/api/client";
import { createDashboardApi } from "@repo/mobile-shared/api/dashboard";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import type { DashboardResponse } from "@repo/mobile-shared/api/types";

function useApiClient() {
  const { getToken } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
  return createApiClient({
    baseUrl: "https://api.mark8ly.com", // TODO: read from config
    getToken,
    getStoreId: () => activeStore?.id ?? null,
  });
}

export function useDashboard() {
  const client = useApiClient();
  const dashboardApi = createDashboardApi(client);

  return useQuery<DashboardResponse>({
    queryKey: ["dashboard"],
    queryFn: () => dashboardApi.get(),
    refetchOnWindowFocus: true,
  });
}
