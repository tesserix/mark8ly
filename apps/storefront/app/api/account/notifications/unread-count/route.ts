// GET /api/account/notifications/unread-count — the customer's unread count.
import { proxyNotifications } from "../_proxy";

export const dynamic = "force-dynamic";

export async function GET(): Promise<Response> {
  return proxyNotifications("/unread-count", { method: "GET" });
}
