// Storefront proxy for server-side payment confirmation (Cashfree).
//
// Forwards POST /api/orders/{id}/confirm-payment to
// POST /api/v1/storefront/stores/{slug}/orders/{id}/confirm-payment.
// The store slug is derived from the request host so the client doesn't
// need to know it.
//
// Unlike verify-payment, the body carries nothing the backend trusts: the
// provider, the provider-side order id and the amount all come from the
// order's own payment_transactions row server-side. See the backend's
// payment_confirm.go.

import { marketplaceStoreUrl, proxyJson } from "../../../checkout/_proxy";
import { resolveStoreSlug } from "@/lib/slug";

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
    await resolveStoreSlug(req.headers.get("host"));
  if (!slug) {
    return Response.json(
      { error: "missing_store", message: "could not resolve store slug from host" },
      { status: 400 },
    );
  }

  return proxyJson(
    `${marketplaceStoreUrl(slug)}/orders/${encodeURIComponent(id)}/confirm-payment`,
    { method: "POST", body: "{}" },
  );
}
