// POST /api/reviews/[handle]/replies/[id]/guest — anonymous visitor
// posts a comment or nested reply under an approved review. Body:
// { content, parent_reply_id?, customer_name, customer_email }.

import { headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { marketplaceStoreUrl, proxyJson } from "../../../../../checkout/_proxy";

export const dynamic = "force-dynamic";

export async function POST(
  req: Request,
  { params }: { params: Promise<{ handle: string; id: string }> },
): Promise<Response> {
  const { id } = await params;
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
    `${marketplaceStoreUrl(slug)}/reviews/${encodeURIComponent(id)}/replies-guest`,
    { method: "POST", body },
  );
}
