// Server-side proxy for the Otto staff console. Reads the session headers
// set by the admin middleware + resolves the active store from the host
// subdomain, then forwards them to the otto service.
//
// The store_id cannot be trusted from the session cookie alone — the
// cookie is minted at login and may not mirror the {slug}-admin.mark8ly.com
// subdomain the agent is currently viewing (multi-store support, store
// switcher). Resolving from the Host header at request time mirrors the
// storefront proxy pattern and guarantees staff see the correct tenant's
// threads.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { platformInternalHeaders } from "@/lib/api/server/platformInternal";

const OTTO_URL = process.env.OTTO_URL ?? "http://localhost:8089";
// Strip surrounding whitespace/newlines — GCP Secret Manager sometimes
// persists random-base64 with a trailing \n, which HTTP strips in transit
// and silently breaks the X-Internal-Auth equality check on otto's side.
const OTTO_INTERNAL_AUTH = (process.env.OTTO_INTERNAL_AUTH ?? "").trim();
const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";
const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

interface ResolvedScope {
  tenantId: string;
  storeId: string;
  slug: string;
}

/**
 * Resolve {tenant_id, store_id} from the current admin subdomain.
 *
 * Host patterns we accept:
 *   {slug}-admin.mark8ly.com     → strip -admin suffix, lookup by slug
 *   admin.<merchant-domain>      → resolve via marketplace-api custom-domain
 *   admin.mark8ly.com            → the canonical admin host; the agent must
 *                                   have picked a specific store via the
 *                                   switcher — we fall back to session's
 *                                   store_id there.
 */
async function resolveScopeFromHost(
  host: string | null,
  sessionTenantId: string,
  sessionStoreId: string,
): Promise<ResolvedScope | null> {
  if (!host) return null;
  const mark8lyMatch = host.match(/^([^.]+)-admin\.mark8ly\.com$/);
  let slug: string | null = null;
  if (mark8lyMatch && mark8lyMatch[1]) {
    slug = mark8lyMatch[1];
  } else if (host.startsWith("admin.") && !host.endsWith(".mark8ly.com")) {
    const customDomain = host.slice("admin.".length);
    try {
      const res = await fetch(
        `${MARKETPLACE_API_URL}/api/v1/storefront/resolve-domain?domain=${encodeURIComponent(customDomain)}`,
        { cache: "no-store" },
      );
      if (res.ok) {
        const body = (await res.json()) as { slug?: string };
        slug = body.slug ?? null;
      }
    } catch {
      /* fall through */
    }
  }
  // No subdomain slug — we're on the canonical admin.mark8ly.com host.
  // Prefer the session's own store_id (set when the agent picks one via
  // the store switcher); otherwise fall back to the first store under
  // the session's tenant. Without this fallback an agent who signs in
  // at admin.mark8ly.com (no tenant subdomain) gets a 400 on every otto
  // call because their session cookie doesn't populate store_id.
  if (!slug) {
    if (sessionStoreId) {
      return {
        tenantId: sessionTenantId,
        storeId: sessionStoreId,
        slug: "",
      };
    }
    return resolveTenantDefaultStore(sessionTenantId);
  }
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/stores/by-slug/${encodeURIComponent(slug)}`,
      { cache: "no-store", headers: platformInternalHeaders() },
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

/**
 * Picks the first store under a tenant. Used by the canonical-admin-host
 * fallback path. Most v1 tenants have exactly one store so "first" is
 * unambiguous; multi-store tenants get the first listed and can switch
 * via the store switcher (which repopulates session.store_id and short-
 * circuits this path on the next request).
 */
async function resolveTenantDefaultStore(
  tenantId: string,
): Promise<ResolvedScope | null> {
  if (!tenantId) return null;
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/tenants/${encodeURIComponent(tenantId)}/stores`,
      { cache: "no-store", headers: platformInternalHeaders() },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as {
      data?: Array<{ id: string; slug: string }>;
    };
    const first = body.data?.[0];
    if (!first?.id) return null;
    return {
      tenantId,
      storeId: first.id,
      slug: first.slug ?? "",
    };
  } catch {
    return null;
  }
}

interface ForwardInit {
  method?: string;
  body?: unknown;
}

export async function forwardToOtto(
  path: string,
  init: ForwardInit = {},
): Promise<Response> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const sessionTenantId = h.get("x-session-tenant-id") ?? "";
  const sessionStoreId = h.get("x-session-store-id") ?? "";
  const email = h.get("x-session-email") ?? "";
  const userName = h.get("x-session-user-name") ?? "";
  const host = h.get("x-forwarded-host") ?? h.get("host");

  if (!userId || !sessionTenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const scope = await resolveScopeFromHost(host, sessionTenantId, sessionStoreId);
  if (!scope) {
    return NextResponse.json(
      { error: "no_active_store", message: "unable to resolve store for this subdomain" },
      { status: 400 },
    );
  }

  const outgoing: Record<string, string> = {
    "Content-Type": "application/json",
    "X-User-Id": userId,
    "X-Tenant-Id": scope.tenantId,
    "X-Store-Id": scope.storeId,
  };
  if (email) outgoing["X-User-Email"] = email;
  if (userName) outgoing["X-User-Name"] = userName;
  if (OTTO_INTERNAL_AUTH) outgoing["X-Internal-Auth"] = OTTO_INTERNAL_AUTH;

  try {
    const upstream = await fetch(`${OTTO_URL}${path}`, {
      method: init.method ?? "GET",
      headers: outgoing,
      body: init.body === undefined ? undefined : JSON.stringify(init.body),
      cache: "no-store",
    });
    const text = await upstream.text();
    return new Response(text, {
      status: upstream.status,
      headers: {
        "Content-Type":
          upstream.headers.get("Content-Type") ?? "application/json",
        // The staff inbox is real-time data — never let the browser (or any
        // intermediary) serve a stale conversation list. Without this the
        // inbox can show an out-of-date list after a fix/store switch until
        // a hard refresh.
        "Cache-Control": "no-store",
      },
    });
  } catch (err) {
    return NextResponse.json(
      {
        error: "upstream_unreachable",
        message:
          err instanceof Error ? err.message : "otto service unreachable",
      },
      { status: 502 },
    );
  }
}
