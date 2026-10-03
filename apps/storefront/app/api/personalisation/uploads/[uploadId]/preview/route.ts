import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../../../../checkout/_proxy";
import { readCartToken } from "../../../_cart";

export const dynamic = "force-dynamic";

/**
 * A short-lived signed GET so the buyer can see their own image.
 *
 * The bucket is private, so this is the only way they see what they
 * uploaded. The URL is returned rather than redirected to, which keeps a
 * signed URL out of browser history and out of referrer headers.
 */
export async function GET(
  req: Request,
  { params }: { params: Promise<{ uploadId: string }> },
): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;

  const cartToken = await readCartToken();
  if (!cartToken) {
    return Response.json({ error: "not_found", message: "upload not found" }, { status: 404 });
  }

  const { uploadId } = await params;
  const url = new URL(
    `${marketplaceStoreUrl(parsed.slug)}/personalisation/uploads/${encodeURIComponent(uploadId)}/preview`,
  );
  url.searchParams.set("cart_token", cartToken);
  return proxyJson(url.toString(), { method: "GET" });
}
