import { cookies } from "next/headers";
import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../_proxy";
import { CART_TOKEN_COOKIE } from "../../personalisation/_cart";

export const dynamic = "force-dynamic";

export async function POST(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;
  const body = await req.text();

  // Forward the customer session cookie so the marketplace-api's
  // OptionalCustomerAuth middleware can set the profile context, and
  // the checkout handler can stamp orders.customer_id for signed-in
  // shoppers (otherwise /account/orders comes back empty).
  const cookieStore = await cookies();
  const session = cookieStore.get("mp_customer_session")?.value;

  // And the CART token. cartTokenForCheckout reads mk_cart_token off
  // this request and, failing that, mints a brand new uuid — which
  // matches no upload, so every personalised line is rejected with
  // "the image for X is no longer available" even though the object is
  // right there and its own preview endpoint serves it happily (#966).
  //
  // The cart token is the ONLY thing proving a buyer's artwork is
  // theirs, so this is also what scopes the lookup: without it the
  // order cannot be matched to the upload at all.
  const cartToken = cookieStore.get(CART_TOKEN_COOKIE)?.value;

  const cookiePairs: string[] = [];
  if (session) cookiePairs.push(`mp_customer_session=${session}`);
  if (cartToken) cookiePairs.push(`${CART_TOKEN_COOKIE}=${cartToken}`);
  const headers: Record<string, string> = {};
  if (cookiePairs.length > 0) headers.Cookie = cookiePairs.join("; ");

  // Forward the buyer's storefront origin so marketplace-api can build
  // absolute hosted-checkout return URLs (Stripe success_url / cancel_url).
  // Without this header the backend silently takes the embedded-intent
  // path and the buyer ends up on a checkout page with no payment widget.
  // Fall back to the inbound Host + forwarded scheme — same-origin POSTs
  // sometimes strip Origin entirely.
  const origin =
    req.headers.get("origin") ??
    (() => {
      const host = req.headers.get("host");
      if (!host) return "";
      const proto = req.headers.get("x-forwarded-proto") ?? "https";
      return `${proto}://${host}`;
    })();
  if (origin) headers.Origin = origin;

  return proxyJson(`${marketplaceStoreUrl(parsed.slug)}/checkout`, {
    method: "POST",
    body,
    headers,
  });
}
