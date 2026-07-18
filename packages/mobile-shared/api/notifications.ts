import type { createApiClient } from "./client";
import {
  notificationListSchema,
  notificationPreferencesResponseSchema,
  type Notification,
  type NotificationListResponse,
  type NotificationPreferences,
  type NotificationPreferencesResponse,
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
    /**
     * GET /notification-preferences (mobile_routes.go). Store-wide per-type
     * toggles — governs whether each notification type is generated at all
     * (bell + push), not a per-device mute.
     */
    getPreferences: () =>
      client.get<NotificationPreferencesResponse>(
        "/notification-preferences",
        undefined,
        notificationPreferencesResponseSchema,
      ),
    /**
     * PATCH /notification-preferences. The backend overwrites the whole
     * JSONB from the submitted keys, so callers MUST send the complete set of
     * toggles — a partial body silently resets the omitted types to default.
     */
    updatePreferences: (preferences: NotificationPreferences) =>
      client.patch<NotificationPreferencesResponse>(
        "/notification-preferences",
        { preferences },
        notificationPreferencesResponseSchema,
      ),
    /** Returns the server-side token id so the caller can later delete it. */
    registerPushToken: (token: string, platform: string, deviceId: string) =>
      client.post<{ id: string }>("/push-tokens", {
        token,
        platform,
        device_id: deviceId,
      }),
    deletePushToken: (tokenId: string) => client.delete(`/push-tokens/${tokenId}`),
  };
}

export type { Notification, NotificationPreferences };
