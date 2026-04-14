// Storefront proxy for the Razorpay client-callback verification.
//
// Forwards POST /api/orders/{id}/verify-payment to
// POST /api/v1/storefront/stores/{slug}/orders/{id}/verify-payment.
// The store slug is derived from the request host so the client doesn't
// need to know it.

import { marketplaceStoreUrl, proxyJson } from "../../../checkout/_proxy";
import { slugFromHost } from "@/lib/slug";

export const dynamic = "force-dynamic";

export async function POST(
  req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  const { id } = await ctx.params;
  if (!id) {
    return Response.json(
      { error: "missing_order_id", message: "order id is required" },
      { status: 400 },
    );
  }

  const url = new URL(req.url);
  const explicitSlug = url.searchParams.get("store");
  const slug =
    explicitSlug ||
    slugFromHost(req.headers.get("host")) ||
    process.env.DEFAULT_STORE_SLUG ||
    "";
  if (!slug) {
    return Response.json(
      { error: "missing_store", message: "could not resolve store slug from host" },
      { status: 400 },
    );
  }

  const body = await req.text();
  return proxyJson(
    `${marketplaceStoreUrl(slug)}/orders/${encodeURIComponent(id)}/verify-payment`,
    { method: "POST", body },
  );
}
