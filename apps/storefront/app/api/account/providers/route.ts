// apps/storefront/app/api/account/providers/route.ts
//
// Returns the linked sign-in providers for the current customer.
// Reads the storefront-side mp_customer_session HMAC cookie to get the
// customer's identity-provider user id, then asks auth-bff.
//
// It used to call Google Identity Toolkit accounts:lookup from this pod
// with GIP_WEB_API_KEY. That was the last live GIP reader on the
// storefront customer path (#787). The replacement is a service-to-service
// hop rather than a direct Zitadel read for the same reason the customer
// login endpoints are: the Zitadel credential that can answer this is the
// instance-level login-client PAT, and it lives only in auth-bff.
//
// Upstream shape (services/auth-bff/internal/session/internal_users.go,
// GET /internal/users/:id/providers, X-Internal-Auth gated):
//   200 -> {"data":{"providers":[{"provider_id":"password","email":"…"}]}}
//   401 -> bad or missing internal secret
//   502 -> Zitadel unreachable
//   503 -> lookup not configured on that deployment
//
// The response this route returns is unchanged from the GIP version, so
// app/account/security/SecurityClient.tsx needed no edit.

import { cookies, headers } from "next/headers";
import { NextResponse } from "next/server";
import { decodeSessionForScope } from "@/lib/session";
import { resolveStoreSlug } from "@/lib/slug";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Read at call time, not module scope, so a value injected after module
// evaluation (and a test's stubbed env) is still seen. Same convention as
// lib/auth/auth-bff-customer.ts's internalAuthHeader.
function authBffURL(): string {
  return process.env.AUTH_BFF_URL ?? "http://localhost:8087";
}

function internalSecret(): string {
  return process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";
}

export async function GET(): Promise<Response> {
  const secret = internalSecret();
  if (!secret) {
    return NextResponse.json(
      { error: "providers_not_configured" },
      { status: 503 },
    );
  }

  const c = await cookies();
  const sessionCookie = c.get("mp_customer_session")?.value;
  if (!sessionCookie) {
    return NextResponse.json({ error: "no_session" }, { status: 401 });
  }

  const h = await headers();
  const storeSlug = await resolveStoreSlug(h.get("host"));
  const session = decodeSessionForScope(sessionCookie, { storeSlug });
  if (!session) {
    return NextResponse.json({ error: "invalid_session" }, { status: 401 });
  }

  // encodeURIComponent, not raw interpolation: the uid comes out of a
  // signed cookie, but a path segment built from an identity value must
  // not be able to reach a different endpoint.
  const endpoint = `${authBffURL()}/internal/users/${encodeURIComponent(session.uid)}/providers`;

  let lookupRes: Response;
  try {
    lookupRes = await fetch(endpoint, {
      method: "GET",
      headers: { "X-Internal-Auth": secret },
      cache: "no-store",
    });
  } catch {
    // Deliberately no detail from the caught error: it could echo
    // request internals, including the internal secret in a URL.
    return NextResponse.json({ error: "providers_unreachable" }, { status: 502 });
  }

  if (!lookupRes.ok) {
    // The status only. auth-bff's error bodies are safe, but there is no
    // reason to widen what a failed provider lookup can put in a log or
    // hand to the browser.
    return NextResponse.json(
      { error: "providers_error", status: lookupRes.status },
      { status: 502 },
    );
  }

  const data = (await lookupRes.json().catch(() => null)) as {
    data?: { providers?: { provider_id: string; email?: string }[] };
  } | null;
  const providers = data?.data?.providers;
  if (!Array.isArray(providers)) {
    return NextResponse.json({ error: "providers_error" }, { status: 502 });
  }

  return NextResponse.json({ data: { providers } });
}
