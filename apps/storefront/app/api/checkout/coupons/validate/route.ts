import {
  marketplaceStoreUrl,
  proxyJson,
  requireStoreSlug,
} from "../../../checkout/_proxy";

export const dynamic = "force-dynamic";

export async function POST(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;
  const body = await req.text();
  return proxyJson(
    `${marketplaceStoreUrl(parsed.slug)}/coupons/validate`,
    { method: "POST", body },
  );
}
