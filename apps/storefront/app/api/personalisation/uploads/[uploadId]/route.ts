import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../../../checkout/_proxy";
import { readCartToken } from "../../_cart";

export const dynamic = "force-dynamic";

/**
 * Remove an upload the buyer changed their mind about, before checkout.
 *
 * Quiet when there is no cart: the caller's intent — "this should not be
 * attached to my order" — is already satisfied.
 */
export async function DELETE(
  req: Request,
  { params }: { params: Promise<{ uploadId: string }> },
): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;

  const cartToken = await readCartToken();
  if (!cartToken) return new Response(null, { status: 204 });

  const { uploadId } = await params;
  const url = new URL(
    `${marketplaceStoreUrl(parsed.slug)}/personalisation/uploads/${encodeURIComponent(uploadId)}`,
  );
  url.searchParams.set("cart_token", cartToken);
  return proxyJson(url.toString(), { method: "DELETE" });
}
