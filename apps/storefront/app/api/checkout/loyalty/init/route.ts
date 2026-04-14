import {
  marketplaceStoreUrl,
  proxyJson,
  requireStoreSlug,
} from "../../_proxy";

export const dynamic = "force-dynamic";

// GET /api/checkout/loyalty/init?store=:slug&email=:email
//
// Returns the public loyalty program config and — if the email is enrolled —
// the customer's current balance. Used by the checkout page to surface the
// loyalty redemption toggle with real data. One combined call rather than
// two separate proxies to keep the client simple.
export async function GET(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;

  const url = new URL(req.url);
  const email = url.searchParams.get("email")?.trim() ?? "";

  const storeUrl = marketplaceStoreUrl(parsed.slug);

  const programPromise = proxyJson(`${storeUrl}/loyalty/program`);
  const customerPromise = email
    ? proxyJson(
        `${storeUrl}/loyalty/me?email=${encodeURIComponent(email)}`,
      )
    : Promise.resolve<Response | null>(null);

  const [programRes, customerRes] = await Promise.all([
    programPromise,
    customerPromise,
  ]);

  const programBody = await programRes
    .json()
    .catch(() => ({ data: null }));
  const customerBody = customerRes
    ? await customerRes.json().catch(() => ({ data: null }))
    : { data: null };

  return Response.json({
    program: programBody.data,
    customer: customerBody.data,
  });
}
