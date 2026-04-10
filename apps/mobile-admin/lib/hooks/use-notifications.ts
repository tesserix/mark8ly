import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { createApiClient } from "@repo/mobile-shared/api/client";
import { createNotificationsApi } from "@repo/mobile-shared/api/notifications";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import type { Notification, PaginatedResponse } from "@repo/mobile-shared/api/types";

function useApiClient() {
  const { getToken } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
  return createApiClient({
    baseUrl: "https://api.mark8ly.com", // TODO: read from config
    getToken,
    getStoreId: () => activeStore?.id ?? null,
  });
}

export function useNotifications() {
  const client = useApiClient();
  const notificationsApi = createNotificationsApi(client);

  return useQuery<PaginatedResponse<Notification>>({
    queryKey: ["notifications"],
    queryFn: () => notificationsApi.list(),
    refetchOnWindowFocus: true,
  });
}

export function useMarkAllRead() {
  const client = useApiClient();
  const notificationsApi = createNotificationsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => notificationsApi.markAllRead(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
    },
  });
}
