import { useQuery } from "@tanstack/react-query";
import { createDashboardApi } from "@repo/mobile-shared/api/dashboard";
import type { DashboardResponse } from "@repo/mobile-shared/api/types";
import { useApiClient } from "@/lib/api-client";

export function useDashboard() {
  const client = useApiClient();
  const dashboardApi = createDashboardApi(client);

  return useQuery<DashboardResponse>({
    queryKey: ["dashboard"],
    queryFn: () => dashboardApi.get(),
    refetchOnWindowFocus: true,
  });
}
