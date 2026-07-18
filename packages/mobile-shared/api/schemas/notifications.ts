import { z } from "zod";
import { legacyPaged } from "../schema-helpers";

/**
 * Wire truth for the admin notifications endpoint —
 * NotificationResponse (notifications.go:31-40).
 *
 * The list envelope is NOT {data, meta}: it is
 * {notifications, page, per_page, total} (verified live 2026-07-16).
 *
 * The app's old type invented `body`, `read` and `deep_link`. The wire sends
 * `message`, `is_read`, and resource_type/resource_id. There is no deep_link
 * anywhere in the backend; tapping a notification is a no-op until a
 * resource_type -> route map can be built against real data (the endpoint is
 * empty in prod, so any mapping today would be a guess).
 */
export const notificationSchema = z.object({
  id: z.string(),
  /**
   * Real values (notification/models.go:16-30): new_order, low_stock,
   * return_requested, payment_received, review_submitted, order_cancelled,
   * order_fulfilled, system_alert. Kept as a plain string so a new backend
   * type never hard-fails a merchant's inbox.
   */
  type: z.string(),
  title: z.string(),
  message: z.string().optional(),
  resource_type: z.string().optional(),
  resource_id: z.string().optional(),
  is_read: z.boolean(),
  created_at: z.string(),
});
export type Notification = z.infer<typeof notificationSchema>;

export const notificationListSchema = legacyPaged("notifications", notificationSchema);
export type NotificationListResponse = z.infer<typeof notificationListSchema>;

/**
 * Per-type notification preferences — the closed set of user-toggleable types
 * (notifications.go `allowedPreferenceKeys`). Each key is optional on the wire:
 * a partial PATCH overwrites the whole JSONB, so a stored row may carry a
 * subset; any absent key falls back to enabled (the backend default). Callers
 * must treat missing as `true` and always PATCH the full set.
 */
export const notificationPreferencesSchema = z.object({
  new_order: z.boolean().optional(),
  low_stock: z.boolean().optional(),
  return_requested: z.boolean().optional(),
  payment_received: z.boolean().optional(),
  review_submitted: z.boolean().optional(),
});
export type NotificationPreferences = z.infer<typeof notificationPreferencesSchema>;

/** GET/PATCH .../notification-preferences envelope (notifications.go). */
export const notificationPreferencesResponseSchema = z.object({
  store_id: z.string().optional(),
  preferences: notificationPreferencesSchema,
});
export type NotificationPreferencesResponse = z.infer<
  typeof notificationPreferencesResponseSchema
>;
