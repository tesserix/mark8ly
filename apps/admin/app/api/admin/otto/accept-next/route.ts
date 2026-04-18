import { forwardToOtto } from "../_proxy";

// Pop the oldest pending conversation FIFO and assign it to the
// signed-in staff. Body is empty — identity + store come from the
// session the proxy forwards. Returns { conversation: null } when
// the queue is empty.
export async function POST() {
  return forwardToOtto(`/api/v1/admin/otto/accept-next`, { method: "POST" });
}
