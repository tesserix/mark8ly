import type { createApiClient } from "./client";
import {
  notificationListSchema,
  type Notification,
  type NotificationListResponse,
} from "./schemas/notifications";

export function createNotificationsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: { page?: string; per_page?: string }) =>
      client.get<NotificationListResponse>(
        "/notifications",
        params as Record<string, string>,
        notificationListSchema,
      ),
    /**
     * PATCH /notifications/read-all (mobile_routes.go:159).
     * This used to be POST /notifications/mark-all-read — wrong method AND
     * wrong path, so it was an unconditional 404. It went unnoticed because
     * the list is always empty in prod, so the "Mark all" button never renders.
     */
    markAllRead: () => client.patch("/notifications/read-all"),
    registerPushToken: (token: string, platform: string, deviceId: string) =>
      client.post("/push-tokens", { token, platform, device_id: deviceId }),
    deletePushToken: (tokenId: string) => client.delete(`/push-tokens/${tokenId}`),
  };
}

export type { Notification };
