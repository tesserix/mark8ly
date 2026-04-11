import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../_proxy";

export const dynamic = "force-dynamic";

export async function POST(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;
  const body = await req.text();
  return proxyJson(`${marketplaceStoreUrl(parsed.slug)}/shipping-rates`, {
    method: "POST",
    body,
  });
}
