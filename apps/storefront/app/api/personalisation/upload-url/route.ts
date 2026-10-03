import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../../checkout/_proxy";
import { ensureCartToken } from "../_cart";

export const dynamic = "force-dynamic";

/**
 * Mint a signed PUT for one buyer image (#963/#965).
 *
 * The cart token is added here rather than taken from the client: it is
 * the only authorisation on the resulting upload, so letting the caller
 * name it would let any shopper claim any cart.
 */
export async function POST(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;

  let body: unknown;
  try {
    body = await req.json();
  } catch {
    return Response.json(
      { error: "invalid_request", message: "upload request could not be parsed" },
      { status: 400 },
    );
  }

  const cartToken = await ensureCartToken();
  const payload = {
    ...(typeof body === "object" && body !== null ? body : {}),
    cart_token: cartToken,
  };

  return proxyJson(`${marketplaceStoreUrl(parsed.slug)}/personalisation/upload-url`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}
