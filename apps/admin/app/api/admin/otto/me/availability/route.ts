import { forwardToOtto } from "../../_proxy";

// GET — current availability row for the signed-in staff member.
// POST — toggle available true/false.
export async function GET() {
  return forwardToOtto(`/api/v1/admin/otto/me/availability`, { method: "GET" });
}

export async function POST(req: Request) {
  const body = await req.text();
  return forwardToOtto(`/api/v1/admin/otto/me/availability`, {
    method: "POST",
    body,
  });
}
