import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../_proxy";

export const dynamic = "force-dynamic";

export async function GET(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;
  return proxyJson(`${marketplaceStoreUrl(parsed.slug)}/shipping-options`);
}
