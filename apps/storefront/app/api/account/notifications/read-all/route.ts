// PATCH /api/account/notifications/read-all — mark all the customer's read.
import { proxyNotifications } from "../_proxy";

export const dynamic = "force-dynamic";

export async function PATCH(): Promise<Response> {
  return proxyNotifications("/read-all", { method: "PATCH" });
}
