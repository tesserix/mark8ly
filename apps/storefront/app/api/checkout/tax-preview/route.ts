import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../_proxy";

export const dynamic = "force-dynamic";

// Same-origin proxy for the public tax-preview endpoint. Lets the
// /checkout page show "Tax: A$24.99" the moment the buyer's shipping
// address is filled in, instead of leaving the line out until submit.
export async function POST(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;
  const body = await req.text();
  return proxyJson(`${marketplaceStoreUrl(parsed.slug)}/tax-preview`, {
    method: "POST",
    body,
  });
}
