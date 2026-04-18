import { forwardToOtto } from "../../../_proxy";

// GET /api/admin/otto/conversations/[id]/audit — timeline of every
// significant state change on this case, oldest-first.
export async function GET(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  return forwardToOtto(
    `/api/v1/admin/otto/conversations/${encodeURIComponent(id)}/audit`,
    { method: "GET" },
  );
}
