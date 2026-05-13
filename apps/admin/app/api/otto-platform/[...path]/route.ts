// Admin-side proxy for the Otto support-chat service, targeting the
// `platform` tenant — admin ↔ Tesserix-employee conversations.
//
// Distinct from /api/admin/otto/* which is the merchant-side inbox for
// the merchant's OWN customers. This route is "merchant ↔ Tesserix":
// the merchant admin is the customer, a Tesserix employee picks the
// chat up from tesserix.app's platform-support inbox.
//
// Tenant: hard-coded to "platform" because the slm-router config maps
// that exact key to /etc/slm-router/prompts/platform.md + the
// `platform` RAG namespace.
//
// Auth: the admin's session is verified by getServerSessionContext()
// (same Keycloak tesserix-internal flow as every other admin route),
// then the merchant's user id is re-emitted as X-User-Id to Otto so
// it skips the OTP step entirely.

import { type NextRequest, NextResponse } from "next/server";
import { cookies } from "next/headers";

import { getServerSessionContext } from "@/lib/auth/serverSession";

const OTTO_URL = process.env.OTTO_URL ?? "http://localhost:8089";
const OTTO_INTERNAL_AUTH = (process.env.OTTO_INTERNAL_AUTH ?? "").trim();
const OTTO_SESSION_COOKIE =
  process.env.OTTO_SESSION_COOKIE ?? "otto_session_platform";

// `platform` tenant has no store concept — every merchant talks to
// the same "support" surface. Using a single deterministic store_id
// keeps the Otto inbox + queue + analytics scoped cleanly without
// inventing a per-tenant store row per merchant.
const PLATFORM_TENANT_ID = "platform";
const PLATFORM_STORE_ID = "default";

async function forward(
  request: NextRequest,
  pathSegments: string[],
): Promise<NextResponse> {
  // Admin must be authenticated. RST in the proxy itself rather than
  // letting Otto 401 — the chat then renders a clean "please log in"
  // banner instead of leaving the widget connecting forever.
  const session = await getServerSessionContext().catch(() => null);
  if (!session?.userId) {
    return NextResponse.json(
      { error: "unauthorized", message: "log in to use platform support" },
      { status: 401 },
    );
  }

  const path = pathSegments.join("/");
  const search = request.nextUrl.searchParams.toString();
  const upstream = `${OTTO_URL}/api/v1/storefront/otto/${path}${
    search ? "?" + search : ""
  }`;

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    "X-Tenant-Id": PLATFORM_TENANT_ID,
    "X-Store-Id": PLATFORM_STORE_ID,
    // Identity — Otto's CustomerContext treats these as a logged-in
    // customer and skips OTP. The admin's tenant_id/store_id are
    // carried separately as X-Client-Tenant-Id so the Tesserix
    // employee can see which merchant they're talking to.
    "X-User-Id": session.userId,
    "X-Client-Tenant-Id": session.tenantId ?? "",
    "X-Client-Tenant-Name": session.tenantName ?? "",
  };
  if (session.email) headers["X-User-Email"] = session.email;
  if (OTTO_INTERNAL_AUTH) headers["X-Internal-Auth"] = OTTO_INTERNAL_AUTH;

  // Forward the platform otto_session cookie so multi-turn
  // conversations stay bound to the same merchant.
  const c = await cookies();
  const ottoCookie = c.get(OTTO_SESSION_COOKIE)?.value;
  if (ottoCookie) headers["Cookie"] = `${OTTO_SESSION_COOKIE}=${ottoCookie}`;

  const method = request.method;
  const body =
    method === "GET" || method === "HEAD" ? undefined : await request.text();

  try {
    const res = await fetch(upstream, {
      method,
      headers,
      body,
      cache: "no-store",
    });
    const text = await res.text();
    const out = new NextResponse(text, { status: res.status });
    out.headers.set(
      "Content-Type",
      res.headers.get("Content-Type") || "application/json",
    );
    const setCookie = res.headers.get("set-cookie");
    if (setCookie) out.headers.set("set-cookie", setCookie);
    return out;
  } catch (err) {
    return NextResponse.json(
      {
        error: "upstream_unreachable",
        message: err instanceof Error ? err.message : "otto unreachable",
      },
      { status: 502 },
    );
  }
}

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const { path } = await params;
  return forward(request, path);
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const { path } = await params;
  return forward(request, path);
}

export async function PUT(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const { path } = await params;
  return forward(request, path);
}

export async function DELETE(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const { path } = await params;
  return forward(request, path);
}
