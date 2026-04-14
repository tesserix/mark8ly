import { cookies } from "next/headers";
import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../_proxy";

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
  const headers: Record<string, string> = {};
  if (session) headers.Cookie = `mp_customer_session=${session}`;

  return proxyJson(`${marketplaceStoreUrl(parsed.slug)}/checkout`, {
    method: "POST",
    body,
    headers,
  });
}
