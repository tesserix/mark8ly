// PATCH /api/account/notifications/[id]/read — mark one read.
import { proxyNotifications } from "../../_proxy";

export const dynamic = "force-dynamic";

export async function PATCH(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
): Promise<Response> {
  const { id } = await params;
  if (!/^[0-9a-f-]{36}$/i.test(id)) {
    return Response.json({ error: "bad_id" }, { status: 400 });
  }
  return proxyNotifications(`/${id}/read`, { method: "PATCH" });
}
