// GET /api/orders/:id/returns — list the customer's return requests for this order.
// POST /api/orders/:id/returns — customer self-service return / replace proxy.
//
// Forwards the customer session cookie; marketplace-api enforces
// ownership + "shipment exists" + "no other open return" gating.

import { cookies, headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { marketplaceStoreUrl, proxyJson } from "../../../checkout/_proxy";

export const dynamic = "force-dynamic";

async function resolveContext(
  id: string,
): Promise<
  | { status: number; body: { error: string; message: string } }
  | { slug: string; session: string | undefined }
> {
  if (!id) {
    return {
      status: 400,
      body: { error: "missing_order_id", message: "order id is required" },
    };
  }
  const h = await headers();
  const slug = await resolveStoreSlug(h.get("host"));
  if (!slug) {
    return {
      status: 400,
      body: { error: "missing_store", message: "could not resolve store slug" },
    };
  }
  const cookieStore = await cookies();
  const session = cookieStore.get("mp_customer_session")?.value;
  return { slug, session };
}

export async function GET(
  _req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  const { id } = await ctx.params;
  const scope = await resolveContext(id);
  if ("status" in scope) return Response.json(scope.body, { status: scope.status });
  if (!scope.session) {
    // Anonymous visitors have nothing to resume; return an empty list
    // rather than 401 so the order page can render cleanly.
    return Response.json({ returns: [] }, { status: 200 });
  }
  return proxyJson(
    `${marketplaceStoreUrl(scope.slug)}/orders/${encodeURIComponent(id)}/returns`,
    {
      method: "GET",
      headers: { Cookie: `mp_customer_session=${scope.session}` },
    },
  );
}

export async function POST(
  req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  const { id } = await ctx.params;
  const scope = await resolveContext(id);
  if ("status" in scope) return Response.json(scope.body, { status: scope.status });
  if (!scope.session) {
    return Response.json(
      { error: "unauthorized", message: "Sign in to request a return." },
      { status: 401 },
    );
  }
  const bodyText = await req.text().catch(() => "");
  return proxyJson(
    `${marketplaceStoreUrl(scope.slug)}/orders/${encodeURIComponent(id)}/returns`,
    {
      method: "POST",
      headers: { Cookie: `mp_customer_session=${scope.session}` },
      body: bodyText,
    },
  );
}
