import { forwardToOtto } from "../_proxy";

// Mints a staff-audience ticket scoped to the tenant+store inbox (no
// specific conversation). The inbox WS uses this to receive new-thread
// notifications in real time.
export async function POST() {
  return forwardToOtto(`/api/v1/admin/otto/ws-ticket`, { method: "POST" });
}
