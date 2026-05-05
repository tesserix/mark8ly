// Server-side proxy for the Otto support-chat service.
//
// The browser cannot call otto directly — it must go through a same-origin
// /api/otto/* route that adds the X-Internal-Auth shared secret and injects
// the resolved tenant_id + store_id for the current store. That resolution
// must happen on the server (otto has no concept of slugs) which is what
// this helper does.
//
// The store lookup is cached per request by `getOttoScope` — we only go to
// platform-api once per HTTP call even if several proxies run in one page
// render.

import { cookies, headers } from "next/headers";

import { decodeSessionForScope } from "@/lib/session";
import { resolveStoreSlug } from "@/lib/slug";
import { platformInternalFetch } from "@/lib/api/server/platformInternal";

const OTTO_URL = process.env.OTTO_URL ?? "http://localhost:8089";
// Trim surrounding whitespace/newlines — the GCP secret-manager source
// sometimes stores random-base64 values with a trailing \n, which would
// otherwise be re-emitted into the X-Internal-Auth header and stripped
// in transit, breaking the equality check on the otto side.
const OTTO_INTERNAL_AUTH = (process.env.OTTO_INTERNAL_AUTH ?? "").trim();
const OTTO_SESSION_COOKIE =
  process.env.OTTO_SESSION_COOKIE ?? "otto_session";
const CUSTOMER_SESSION_COOKIE = "mp_customer_session";

interface OttoScope {
  tenantId: string;
  storeId: string;
  slug: string;
}

/**
 * Resolves the current store's tenant_id + store_id from the Host header.
 * Uses platform-api's internal by-slug endpoint (same one admin middleware
 * uses) so we don't need to duplicate the public PublicStore payload.
 */
export async function getOttoScope(): Promise<OttoScope | null> {
  const h = await headers();
  const host = h.get("x-forwarded-host") ?? h.get("host");
  const slug = await resolveStoreSlug(host);
  if (!slug) return null;
  try {
    const res = await platformInternalFetch(
      `/internal/stores/by-slug/${encodeURIComponent(slug)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as {
      data?: { id: string; tenant_id: string; slug: string };
    };
    if (!body.data?.id || !body.data?.tenant_id) return null;
    return {
      tenantId: body.data.tenant_id,
      storeId: body.data.id,
      slug: body.data.slug,
    };
  } catch {
    return null;
  }
}

interface ForwardInit {
  method?: string;
  body?: unknown;
}

/**
 * Forwards an HTTP call to otto with the tenant/store + internal-auth
 * headers + the session cookie. The response is streamed back verbatim.
 */
export async function forwardToOtto(
  path: string,
  init: ForwardInit = {},
): Promise<Response> {
  const scope = await getOttoScope();
  if (!scope) {
    return Response.json(
      { error: "unknown_store", message: "store not found for host" },
      { status: 404 },
    );
  }

  const c = await cookies();
  const session = c.get(OTTO_SESSION_COOKIE)?.value;
  // Forward the storefront's logged-in customer identity when available so
  // Otto can skip the anonymous OTP flow for already-verified users. The
  // mp_customer_session cookie is HMAC-signed; decodeSession rejects any
  // forgery, so we trust the fields it returns.
  const customerCookie = c.get(CUSTOMER_SESSION_COOKIE)?.value;
  const customer = customerCookie
    ? decodeSessionForScope(customerCookie, {
        storeSlug: scope.slug,
        storeId: scope.storeId,
        tenantId: scope.tenantId,
      })
    : null;

  const outgoing: Record<string, string> = {
    "Content-Type": "application/json",
    "X-Tenant-Id": scope.tenantId,
    "X-Store-Id": scope.storeId,
  };
  if (OTTO_INTERNAL_AUTH) outgoing["X-Internal-Auth"] = OTTO_INTERNAL_AUTH;
  if (customer?.uid) outgoing["X-User-Id"] = customer.uid;
  if (customer?.email) outgoing["X-User-Email"] = customer.email;
  // If the client presented an otto_session cookie we forward it as a
  // header — works for WebSocket-preflight paths and gives the service a
  // uniform input.
  if (session) outgoing["Cookie"] = `${OTTO_SESSION_COOKIE}=${session}`;

  try {
    const upstream = await fetch(`${OTTO_URL}${path}`, {
      method: init.method ?? "GET",
      body: init.body === undefined ? undefined : JSON.stringify(init.body),
      headers: outgoing,
      cache: "no-store",
    });

    // Forward Set-Cookie from otto (this is how the session cookie is
    // minted on the first call) and the body.
    const text = await upstream.text();
    const resHeaders = new Headers({
      "Content-Type":
        upstream.headers.get("Content-Type") ?? "application/json",
    });
    const setCookie = upstream.headers.get("set-cookie");
    if (setCookie) resHeaders.append("set-cookie", setCookie);
    return new Response(text, {
      status: upstream.status,
      headers: resHeaders,
    });
  } catch (err) {
    return Response.json(
      {
        error: "upstream_unreachable",
        message: err instanceof Error ? err.message : "otto unreachable",
      },
      { status: 502 },
    );
  }
}

/**
 * Resolve the public WebSocket URL for the widget to connect to. The WS
 * upgrade has to leave the Next.js process — the browser opens a socket
 * directly to otto via the Istio gateway at `/api/v1/otto/...`.
 */
export function publicWsPathForConversation(id: string): string {
  return `/api/v1/storefront/otto/conversations/${encodeURIComponent(id)}/ws`;
}

export const config = {
  ottoSessionCookie: OTTO_SESSION_COOKIE,
};
