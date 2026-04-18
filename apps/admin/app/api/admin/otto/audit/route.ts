import { forwardToOtto } from "../_proxy";

// GET /api/admin/otto/audit — recent events across the whole store.
// Feeds the Support → Audit log admin page.
export async function GET() {
  return forwardToOtto(`/api/v1/admin/otto/audit`, { method: "GET" });
}
