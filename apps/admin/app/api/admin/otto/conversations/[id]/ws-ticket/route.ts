import { forwardToOtto } from "../../../_proxy";

// Staff-audience ticket bound to a specific thread.
export async function POST(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  return forwardToOtto(
    `/api/v1/admin/otto/conversations/${encodeURIComponent(id)}/ws-ticket`,
    { method: "POST" },
  );
}
