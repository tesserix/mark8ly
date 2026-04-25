// apps/storefront/app/api/account/orders/route.ts
//
// Proxy for customer order history. Reads customer session from the
// mp_customer_session cookie, resolves the store slug, and forwards
// the request to marketplace-api with the session cookie so the
// backend middleware can authenticate the customer.

import { cookies, headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { decodeSessionForScope } from "@/lib/session";
import { marketplaceStoreUrl, proxyJson } from "../../checkout/_proxy";

export const dynamic = "force-dynamic";

async function resolveSlug(): Promise<string | null> {
  const h = await headers();
  const host = h.get("host");
  return await resolveStoreSlug(host);
}

export async function GET(): Promise<Response> {
  const slug = await resolveSlug();
  if (!slug) {
    return Response.json(
      { error: "missing_store", message: "Could not resolve store slug" },
      { status: 400 },
    );
  }

  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value ?? "";
  const session = decodeSessionForScope(sessionCookie, { storeSlug: slug });

  if (!session) {
    return Response.json(
      { error: "unauthorized", message: "Please sign in to view orders" },
      { status: 401 },
    );
  }

  // Forward the session cookie so the backend OptionalCustomerAuth
  // middleware can authenticate the customer and set profile context.
  return proxyJson(`${marketplaceStoreUrl(slug)}/account/orders`, {
    headers: { Cookie: `mp_customer_session=${sessionCookie}` },
  });
}
