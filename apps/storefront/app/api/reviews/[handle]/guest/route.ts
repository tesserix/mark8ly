// POST /api/reviews/[handle]/guest — anonymous visitor submits a
// product review by supplying name + email in the body. No session
// cookie required. Backend still gates on UNIQUE(store, product,
// email) so re-posting from the same inbox trips a 409.

import { headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { marketplaceStoreUrl, proxyJson } from "../../../checkout/_proxy";

export const dynamic = "force-dynamic";

export async function POST(
  req: Request,
  { params }: { params: Promise<{ handle: string }> },
): Promise<Response> {
  const { handle } = await params;
  const h = await headers();
  const slug = await resolveStoreSlug(h.get("host"));
  if (!slug) {
    return Response.json(
      { error: "missing_store", message: "Could not resolve store slug" },
      { status: 400 },
    );
  }
  const body = await req.text();
  return proxyJson(
    `${marketplaceStoreUrl(slug)}/products/${encodeURIComponent(handle)}/reviews-guest`,
    { method: "POST", body },
  );
}
