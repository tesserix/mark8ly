// POST /api/reviews/[handle]/replies/[id] — customer posts a comment
// (or nested reply) on an approved review. Body: { content,
// parent_reply_id? }.

import { headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { marketplaceStoreUrl, proxyJson } from "../../../../checkout/_proxy";

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
  const cookieHeader = req.headers.get("cookie") ?? "";
  if (!cookieHeader.includes("mp_customer_session=")) {
    return Response.json(
      { error: "unauthorized", message: "Sign in to comment." },
      { status: 401 },
    );
  }
  const body = await req.text();
  return proxyJson(
    `${marketplaceStoreUrl(slug)}/account/reviews/${encodeURIComponent(id)}/replies`,
    {
      method: "POST",
      body,
      headers: { Cookie: cookieHeader },
    },
  );
}
