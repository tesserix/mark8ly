import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { createNotificationsApi } from "@repo/mobile-shared/api/notifications";
import type { Notification, PaginatedResponse } from "@repo/mobile-shared/api/types";
import { useApiClient } from "@/lib/api-client";

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
