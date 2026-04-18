import { forwardToOtto } from "../../_proxy";

// GET — current availability row for the signed-in staff member.
// POST — toggle available true/false.
export async function GET() {
  return forwardToOtto(`/api/v1/admin/otto/me/availability`, { method: "GET" });
}

export async function POST(req: Request) {
  // forwardToOtto JSON.stringify's the body internally — pass a
  // parsed object, not a raw text string (doing both double-wraps
  // the payload and Gin rejects it as an invalid body).
  const body = await req.json().catch(() => ({}));
  return forwardToOtto(`/api/v1/admin/otto/me/availability`, {
    method: "POST",
    body,
  });
}
