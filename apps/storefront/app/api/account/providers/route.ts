// apps/storefront/app/api/account/providers/route.ts
//
// Returns the linked auth providers for the current customer.
// Reads the storefront-side mp_customer_session HMAC cookie to get
// the GIP UID, then calls Identity Toolkit accounts:lookup against
// the MP-Customer pool. Mirrors auth-bff's /auth/me/providers but
// lives in the storefront pod because customers don't have an
// auth-bff session.

import { cookies, headers } from "next/headers";
import { NextResponse } from "next/server";
import { decodeSessionForScope } from "@/lib/session";
import { resolveStoreSlug } from "@/lib/slug";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const GIP_API_KEY = process.env.GIP_WEB_API_KEY ?? "";
const GIP_CUSTOMER_TENANT_ID = process.env.GIP_CUSTOMER_TENANT_ID ?? "";

export async function GET(): Promise<Response> {
  if (!GIP_API_KEY || !GIP_CUSTOMER_TENANT_ID) {
    return NextResponse.json(
      { error: "gip_not_configured" },
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
  const session = sessionCookie
    ? decodeSessionForScope(sessionCookie, { storeSlug })
    : null;
  if (!session) {
    return NextResponse.json({ error: "invalid_session" }, { status: 401 });
  }

  const lookupRes = await fetch(
    `https://identitytoolkit.googleapis.com/v1/accounts:lookup?key=${encodeURIComponent(GIP_API_KEY)}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        localId: [session.uid],
        tenantId: GIP_CUSTOMER_TENANT_ID,
      }),
      cache: "no-store",
    },
  );
  if (!lookupRes.ok) {
    return NextResponse.json(
      { error: "gip_error", status: lookupRes.status },
      { status: 502 },
    );
  }
  const data = (await lookupRes.json()) as {
    users?: {
      email?: string;
      passwordHash?: string;
      providerUserInfo?: { providerId: string; email?: string }[];
    }[];
  };
  if (!data.users || data.users.length === 0) {
    return NextResponse.json({ error: "user_not_found" }, { status: 404 });
  }
  const user = data.users[0]!;
  const providers: { provider_id: string; email?: string }[] = [];
  if (user.passwordHash) {
    providers.push({ provider_id: "password", email: user.email });
  }
  for (const p of user.providerUserInfo ?? []) {
    providers.push({ provider_id: p.providerId, email: p.email });
  }
  return NextResponse.json({ data: { providers } });
}
