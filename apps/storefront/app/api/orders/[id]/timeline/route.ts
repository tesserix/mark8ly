// Storefront proxy for the delivery timeline poll. Forwards the
// session cookie so the backend can enforce that a customer only sees
// their own order, and returns just the `shipment` + `timeline` fields
// so payloads stay tight.

import { cookies, headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { marketplaceStoreUrl, proxyJson } from "../../../checkout/_proxy";

export const dynamic = "force-dynamic";

export async function GET(
  _req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  const { id } = await ctx.params;
  if (!id) {
    return Response.json(
      { error: "missing_order_id", message: "order id is required" },
      { status: 400 },
    );
  }

  const h = await headers();
  const host = h.get("host");
  const slug = await resolveStoreSlug(host);
  if (!slug) {
    return Response.json(
      { error: "missing_store", message: "could not resolve store slug" },
      { status: 400 },
    );
  }

  const cookieStore = await cookies();
  const session = cookieStore.get("mp_customer_session")?.value;
  const fwd: Record<string, string> = {};
  if (session) fwd.Cookie = `mp_customer_session=${session}`;

  return proxyJson(
    `${marketplaceStoreUrl(slug)}/orders/${encodeURIComponent(id)}`,
    { headers: fwd },
  );
}
