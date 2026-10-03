import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../../../../checkout/_proxy";
import { readCartToken } from "../../../_cart";

export const dynamic = "force-dynamic";

/**
 * Confirm the object arrived, and hand the server the browser's measured
 * dimensions so it can drive the resolution warning.
 *
 * No cart token means no upload could have been created by this browser,
 * so this is a 400 rather than a mint: minting here would produce an
 * identity that owns nothing and a confirm that can only 404.
 */
export async function POST(
  req: Request,
  { params }: { params: Promise<{ uploadId: string }> },
): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;

  const cartToken = await readCartToken();
  if (!cartToken) {
    return Response.json(
      { error: "invalid_request", message: "no cart to confirm against" },
      { status: 400 },
    );
  }

  let body: unknown = {};
  try {
    body = await req.json();
  } catch {
    // Dimensions are advisory; an empty body is a valid confirm.
  }

  const { uploadId } = await params;
  return proxyJson(
    `${marketplaceStoreUrl(parsed.slug)}/personalisation/uploads/${encodeURIComponent(uploadId)}/confirm`,
    {
      method: "POST",
      body: JSON.stringify({
        ...(typeof body === "object" && body !== null ? body : {}),
        cart_token: cartToken,
      }),
    },
  );
}
