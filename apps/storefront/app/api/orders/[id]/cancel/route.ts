// POST /api/orders/:id/cancel — customer self-service cancel proxy.
//
// Forwards the customer session cookie to marketplace-api which gates
// the cancel on order ownership, status machine, and shipment-in-flight.
// Returns the upstream JSON envelope verbatim so the UI can render
// 409 (already shipped / not cancellable) cleanly.

import { cookies, headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { marketplaceStoreUrl, proxyJson } from "../../../checkout/_proxy";

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

  const h = await headers();
  const slug = await resolveStoreSlug(h.get("host"));
  if (!slug) {
    return Response.json(
      { error: "missing_store", message: "could not resolve store slug" },
      { status: 400 },
    );
  }

  const cookieStore = await cookies();
  const session = cookieStore.get("mp_customer_session")?.value;
  if (!session) {
    return Response.json(
      { error: "unauthorized", message: "Sign in to cancel an order." },
      { status: 401 },
    );
  }

  // Forward the body so the customer can include a reason. Empty body
  // is fine — marketplace-api substitutes a default.
  const bodyText = await req.text().catch(() => "");

  return proxyJson(
    `${marketplaceStoreUrl(slug)}/orders/${encodeURIComponent(id)}/cancel`,
    {
      method: "POST",
      headers: { Cookie: `mp_customer_session=${session}` },
      body: bodyText,
    },
  );
}
