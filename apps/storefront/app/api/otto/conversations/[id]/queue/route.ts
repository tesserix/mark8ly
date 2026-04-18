import { forwardToOtto } from "../../../_proxy";

// GET /api/otto/conversations/[id]/queue — returns position in the
// pending queue + estimated wait. The widget polls this every few
// seconds until status flips from pending to active.
export async function GET(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  return forwardToOtto(
    `/api/v1/storefront/otto/conversations/${encodeURIComponent(id)}/queue`,
    { method: "GET" },
  );
}
