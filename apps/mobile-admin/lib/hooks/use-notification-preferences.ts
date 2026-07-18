import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { createNotificationsApi } from "@repo/mobile-shared/api/notifications";
import type { NotificationPreferences } from "@repo/mobile-shared/api/notifications";
import { useApiClient } from "@/lib/api-client";

const QUERY_KEY = ["notification-preferences"] as const;

/**
 * The closed set of user-toggleable notification types, in display order.
 * Mirrors the backend `allowedPreferenceKeys`; `system_alert` etc. are
 * intentionally absent — merchants must not be able to silence them.
 */
export const PREFERENCE_TYPES = [
  "new_order",
  "low_stock",
  "return_requested",
  "payment_received",
  "review_submitted",
] as const;

export type PreferenceType = (typeof PREFERENCE_TYPES)[number];

/**
 * Reads per-type preferences and normalizes them to a complete map — any key
 * the wire omits defaults to enabled, matching the backend's send-time
 * fallback. Callers get all five keys, always, so the UI never renders an
 * undefined toggle.
 */
export function useNotificationPreferences() {
  const client = useApiClient();
  const notificationsApi = createNotificationsApi(client);

  return useQuery({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const res = await notificationsApi.getPreferences();
      const prefs = res.preferences;
      return PREFERENCE_TYPES.reduce<Record<PreferenceType, boolean>>(
        (acc, key) => {
          acc[key] = prefs[key] ?? true;
          return acc;
        },
        {} as Record<PreferenceType, boolean>,
      );
    },
  });
}

/**
 * Persists the FULL preference set. The backend overwrites the whole JSONB
 * from the submitted keys, so a partial body silently resets the omitted
 * types — callers must pass every toggle's current value.
 */
export function useUpdateNotificationPreferences() {
  const client = useApiClient();
  const notificationsApi = createNotificationsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (preferences: NotificationPreferences) =>
      notificationsApi.updatePreferences(preferences),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: QUERY_KEY });
    },
  });
}
