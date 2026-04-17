import { forwardToOtto } from "../../../_proxy";

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  return forwardToOtto(
    `/api/v1/admin/otto/conversations/${encodeURIComponent(id)}/messages`,
  );
}

export async function POST(
  req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const body = await req.json().catch(() => ({}));
  return forwardToOtto(
    `/api/v1/admin/otto/conversations/${encodeURIComponent(id)}/messages`,
    { method: "POST", body },
  );
}
