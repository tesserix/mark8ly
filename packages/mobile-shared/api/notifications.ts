import type { createApiClient } from "./client";
import type { Notification, PaginatedResponse } from "./types";

export function createNotificationsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: { cursor?: string; limit?: string }) =>
      client.get<PaginatedResponse<Notification>>("/notifications", params as Record<string, string>),
    markAllRead: () => client.post("/notifications/mark-all-read"),
    registerPushToken: (token: string, platform: string, deviceId: string) =>
      client.post("/push-tokens", { token, platform, device_id: deviceId }),
    deletePushToken: (tokenId: string) => client.delete(`/push-tokens/${tokenId}`),
  };
}
