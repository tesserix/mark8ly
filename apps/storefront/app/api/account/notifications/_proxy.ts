// Shared proxy for the customer notification bell. Resolves the store slug +
// validates the mp_customer_session cookie, then forwards to marketplace-api's
// /account/notifications* endpoints with the session cookie so the backend can
// authenticate the customer.
import { cookies, headers } from "next/headers";

import { resolveStoreSlug } from "@/lib/slug";
import { decodeSessionForScope } from "@/lib/session";
import { marketplaceStoreUrl, proxyJson } from "../../checkout/_proxy";

export async function proxyNotifications(
  path: string,
  init: RequestInit = {},
): Promise<Response> {
  const h = await headers();
  const slug = await resolveStoreSlug(h.get("host"));
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
      { error: "unauthorized", message: "Please sign in" },
      { status: 401 },
    );
  }
  return proxyJson(`${marketplaceStoreUrl(slug)}/account/notifications${path}`, {
    ...init,
    headers: { ...(init.headers ?? {}), Cookie: `mp_customer_session=${sessionCookie}` },
  });
}
