import { forwardToOtto } from "../../_proxy";

export async function POST(req: Request) {
  const body = await req.json().catch(() => ({}));
  return forwardToOtto("/api/v1/storefront/otto/verify/start", {
    method: "POST",
    body,
  });
}
