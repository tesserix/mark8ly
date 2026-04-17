// apps/storefront/app/api/reviews/[handle]/route.ts
//
// Proxy for product review endpoints. GET lists reviews, POST submits
// a new review (requires mp_customer_session cookie forwarded upstream).

import { headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { marketplaceStoreUrl, proxyJson } from "../../checkout/_proxy";

export const dynamic = "force-dynamic";

async function resolveSlug(req: Request): Promise<string | null> {
  // Prefer ?store= query param (dev convenience), fall back to host header.
  const url = new URL(req.url);
  const fromQuery = url.searchParams.get("store");
  if (fromQuery) return fromQuery;

  const h = await headers();
  const host = h.get("host");
  return await resolveStoreSlug(host);
}

export async function GET(
  req: Request,
  { params }: { params: Promise<{ handle: string }> },
): Promise<Response> {
  const { handle } = await params;
  const slug = await resolveSlug(req);
  if (!slug) {
    return Response.json(
      { error: "missing_store", message: "Could not resolve store slug" },
      { status: 400 },
    );
  }

  // Forward the customer cookie even on the public GET so the backend's
  // OptionalCustomerAuth can populate viewer_reaction on each review.
  // Anonymous calls still work (no cookie → no viewer_reaction field).
  const cookieHeader = req.headers.get("cookie") ?? "";
  return proxyJson(
    `${marketplaceStoreUrl(slug)}/products/${encodeURIComponent(handle)}/reviews`,
    cookieHeader ? { headers: { Cookie: cookieHeader } } : {},
  );
}

export async function POST(
  req: Request,
  { params }: { params: Promise<{ handle: string }> },
): Promise<Response> {
  const { handle } = await params;
  const slug = await resolveSlug(req);
  if (!slug) {
    return Response.json(
      { error: "missing_store", message: "Could not resolve store slug" },
      { status: 400 },
    );
  }

  // Forward the session cookie so the backend can authenticate the customer.
  const cookieHeader = req.headers.get("cookie") ?? "";
  const body = await req.text();

  return proxyJson(
    `${marketplaceStoreUrl(slug)}/account/products/${encodeURIComponent(handle)}/reviews`,
    {
      method: "POST",
      body,
      headers: { Cookie: cookieHeader },
    },
  );
}
