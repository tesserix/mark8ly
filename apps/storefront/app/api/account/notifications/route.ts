// GET /api/account/notifications — the logged-in customer's notifications.
import { proxyNotifications } from "./_proxy";

export const dynamic = "force-dynamic";

export async function GET(): Promise<Response> {
  return proxyNotifications("", { method: "GET" });
}
