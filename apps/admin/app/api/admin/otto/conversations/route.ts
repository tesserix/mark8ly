import { forwardToOtto } from "../_proxy";

export async function GET(req: Request) {
  const url = new URL(req.url);
  const qs = url.search; // "?status=pending&assignee=mine"
  return forwardToOtto(
    `/api/v1/admin/otto/conversations${qs}`,
  );
}
