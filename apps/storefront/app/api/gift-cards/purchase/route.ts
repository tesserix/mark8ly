import {
  marketplaceStoreUrl,
  proxyJson,
  requireStoreSlug,
} from "../../checkout/_proxy";

export const dynamic = "force-dynamic";

// Server-side proxy for the storefront gift card purchase flow. The
// client form POSTs here; we forward to marketplace-api with the
// X-Storefront-Key header, and return the Stripe Checkout URL the
// client should redirect to.
export async function POST(req: Request): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;
  const body = await req.text();
  return proxyJson(
    `${marketplaceStoreUrl(parsed.slug)}/gift-cards/purchase`,
    { method: "POST", body },
  );
}
