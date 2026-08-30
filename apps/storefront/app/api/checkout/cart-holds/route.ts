import { cookies } from "next/headers";
import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../_proxy";

export const dynamic = "force-dynamic";

/**
 * Cart stock holds (#232).
 *
 * # Why this route owns the cookie
 *
 * marketplace-api sets its own `mk_cart_token` cookie, but `proxyJson`
 * deliberately does not forward upstream `Set-Cookie` headers — and even if
 * it did, the cookie would carry the API's domain, not the storefront's.
 *
 * So the cart identity is minted and stored HERE, on the storefront origin,
 * and passed upstream in the request body (which the API accepts for exactly
 * this case). One owner, no cross-domain cookie rewriting.
 *
 * httpOnly because the token identifies a stock reservation: script access
 * buys the storefront nothing and costs XSS exposure. The client learns its
 * token from the response body when it needs it.
 */
const CART_TOKEN_COOKIE = "mk_cart_token";

// Matches HoldTTL in the API. If they drift, the countdown lies to the
// shopper — the API's value is the one that decides, this only bounds how
// long the browser keeps the identity.
const HOLD_TTL_SECONDS = 15 * 60;

export async function POST(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;

  const cookieStore = await cookies();
  const existing = cookieStore.get(CART_TOKEN_COOKIE)?.value;

  let body: unknown;
  try {
    body = await req.json();
  } catch {
    return Response.json(
      { error: "invalid_request", message: "cart hold request could not be parsed" },
      { status: 400 },
    );
  }

  const payload = {
    ...(typeof body === "object" && body !== null ? body : {}),
    ...(existing ? { cart_token: existing } : {}),
  };

  const upstream = await proxyJson(`${marketplaceStoreUrl(parsed.slug)}/cart/holds`, {
    method: "POST",
    body: JSON.stringify(payload),
  });

  // Persist whatever token the API settled on — it mints one when we had
  // none, so reading it back is how a first cart write gains an identity.
  if (upstream.ok) {
    const text = await upstream.clone().text();
    try {
      const parsedBody = JSON.parse(text) as { data?: { cart_token?: string } };
      const token = parsedBody?.data?.cart_token;
      if (token && token !== existing) {
        cookieStore.set(CART_TOKEN_COOKIE, token, {
          httpOnly: true,
          sameSite: "lax",
          secure: process.env.NODE_ENV === "production",
          path: "/",
          maxAge: HOLD_TTL_SECONDS,
        });
      }
    } catch {
      // A response we cannot parse is still a successful hold upstream;
      // the shopper keeps whatever identity they had rather than losing
      // their reservation to a parsing detail.
    }
  }

  return upstream;
}

/**
 * Release the cart's holds — called when the shopper empties their cart.
 *
 * Idempotent and quiet: releasing a cart that holds nothing is a success,
 * because the caller's intent is satisfied either way.
 */
export async function DELETE(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;

  const cookieStore = await cookies();
  const token = cookieStore.get(CART_TOKEN_COOKIE)?.value;
  if (!token) return new Response(null, { status: 204 });

  const upstream = await proxyJson(
    `${marketplaceStoreUrl(parsed.slug)}/cart/holds/${encodeURIComponent(token)}`,
    { method: "DELETE" },
  );
  cookieStore.delete(CART_TOKEN_COOKIE);
  return upstream;
}
