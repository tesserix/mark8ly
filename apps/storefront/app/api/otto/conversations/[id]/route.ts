import { forwardToOtto } from "../../_proxy";

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  return forwardToOtto(
    `/api/v1/storefront/otto/conversations/${encodeURIComponent(id)}`,
  );
}
