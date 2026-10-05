import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../../checkout/_proxy";
import { readCartToken } from "../_cart";

export const dynamic = "force-dynamic";

/**
 * Signs a whole cart's thumbnails in one request (#966).
 *
 * The per-upload /preview route next door is right for the product page,
 * where there is one image. A cart has a line per personalised item and
 * the URLs are short-lived and uncacheable, so re-signing them one at a
 * time is a round trip per line on every render.
 *
 * POST, not GET: a cart's worth of uuids does not belong in a query
 * string, where access logs and referrer headers would keep them.
 *
 * The cart token comes from the httpOnly cookie, never the body — it is
 * the only thing proving these uploads are the caller's.
 */
export async function POST(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;

  const cartToken = await readCartToken();
  if (!cartToken) {
    // No cart means no uploads to show. An empty result rather than a
    // 404: the caller is rendering a cart page and this is not an error
    // state, it is simply nothing to draw.
    return Response.json({ previews: {}, unavailable: [] });
  }

  let uploadIds: unknown = [];
  try {
    const body = (await req.json()) as { upload_ids?: unknown };
    uploadIds = body?.upload_ids ?? [];
  } catch {
    return Response.json(
      { error: "invalid_request", message: "a list of upload ids is required" },
      { status: 400 },
    );
  }
  if (!Array.isArray(uploadIds)) {
    return Response.json(
      { error: "invalid_request", message: "a list of upload ids is required" },
      { status: 400 },
    );
  }

  const url = new URL(`${marketplaceStoreUrl(parsed.slug)}/personalisation/previews`);
  url.searchParams.set("cart_token", cartToken);
  return proxyJson(url.toString(), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ upload_ids: uploadIds.filter((v) => typeof v === "string") }),
  });
}
