import { forwardToOtto } from "../../../_proxy";

// Mints a short-lived ticket the widget uses to open the WebSocket. The
// ticket is audience-scoped to "customer" + bound to this conversation —
// it cannot be reused for any other thread.
export async function POST(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  return forwardToOtto(
    `/api/v1/storefront/otto/conversations/${encodeURIComponent(id)}/ws-ticket`,
    { method: "POST" },
  );
}
