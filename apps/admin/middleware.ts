import { NextResponse, type NextRequest } from "next/server";
import { applyGeoCookie } from "./lib/geo/geoMiddleware";

/**
 * Admin middleware — Phase J.
 *
 * Validates the session cookie against `auth-bff GET /auth/session`
 * before letting any authenticated route render. On success the
 * resolved user/tenant are forwarded to the downstream page via
 * request headers (`x-session-user-id`, `x-session-email`,
 * `x-session-tenant-id`) so the page can `headers()` them without
 * re-hitting auth-bff.
 *
 * Failure modes:
 *   - no cookie on request       → redirect to /login (admin's own form)
 *   - cookie present but invalid → redirect to /login
 *   - auth-bff unreachable       → redirect to /login (fail closed)
 *
 * The "fail closed" branch is deliberate: an unreachable auth-bff is a
 * real outage, not a gray zone. Rendering an admin page with no
 * session would leak tenant-scoped data to anonymous visitors.
 *
 * Phase M: /login is now hosted on the admin app itself (not the
 * marketing site) so returning users don't bounce across origins.
 */
const SESSION_COOKIE_NAME = process.env.SESSION_COOKIE_NAME ?? "m8_session";
const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8087";
const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";
const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
// Canonical sign-in host. This is the single host we register with
// Google OAuth as an "Authorized JavaScript origin" — every per-tenant
// subdomain ({slug}-admin.mark8ly.com) bounces here for unauthenticated
// traffic, and the session cookie (scoped to .mark8ly.com) carries
// back across the bounce automatically. Dev falls back to same-origin.
const CANONICAL_LOGIN_ORIGIN =
  process.env.NEXT_PUBLIC_ADMIN_LOGIN_ORIGIN ?? "";

// Routes that should never be gated — login redirect targets, static
// assets, and anything that must render without a session.
const PUBLIC_PREFIXES = [
  "/login",
  "/logout",
  "/forgot-password",
  "/reset-password", // branded reset flow — lands here from the email link
  "/accept-invite", // Phase P: invitees must land here without a session
  "/webhooks", // external provider callbacks (Stripe, etc.) — never gated
  "/api/health", // kubelet probe target; must not 30x to /login
  "/_next",
  "/favicon",
  "/icon-",
];

interface SessionResponse {
  data: {
    user_id: string;
    email: string;
    tenant_id: string;
    store_id?: string;
  };
}

export async function middleware(req: NextRequest) {
  const { pathname } = req.nextUrl;

  // Public pricing page — geo-localize currency from CF-IPCountry before RSC
  // render. No auth or tenant extraction needed; the page is unauthenticated.
  if (pathname === "/pricing" || pathname.startsWith("/pricing/")) {
    const response = NextResponse.next();
    applyGeoCookie(req, response);
    return response;
  }

  if (PUBLIC_PREFIXES.some((p) => pathname.startsWith(p))) {
    return NextResponse.next();
  }

  const cookie = req.cookies.get(SESSION_COOKIE_NAME);
  if (!cookie || !cookie.value) {
    return redirectToLogin(req);
  }

  // Validate against auth-bff. We forward the entire Cookie header so
  // the HttpOnly cookie is seen exactly as the browser sent it.
  const cookieHeader = req.headers.get("cookie") ?? "";
  let session: SessionResponse["data"] | null = null;
  try {
    const res = await fetch(`${AUTH_BFF_URL}/auth/session`, {
      method: "GET",
      headers: { Cookie: cookieHeader },
      cache: "no-store",
    });
    if (res.ok) {
      const body = (await res.json()) as SessionResponse;
      session = body.data;
    }
  } catch {
    // Auth-bff unreachable — fall through to the login redirect.
  }

  if (!session) {
    return redirectToLogin(req);
  }

  // Tenant-subdomain resolution — when the user lands on
  // `{slug}-admin.mark8ly.com`, make sure the session's tenant_id
  // matches the tenant that owns the store with that slug. If not,
  // auto-switch the session to the correct tenant (when the user has
  // membership) instead of silently rendering an empty dashboard.
  const host = req.headers.get("host") ?? "";
  let requestedSlug: string | null = null;
  const mark8lyMatch = host.match(/^([^.]+)-admin\.mark8ly\.com$/);
  if (mark8lyMatch && mark8lyMatch[1]) {
    requestedSlug = mark8lyMatch[1];
  } else if (host.startsWith("admin.") && !host.endsWith(".mark8ly.com")) {
    // Custom domain pattern: admin.<merchant-domain>. Resolve via
    // marketplace-api's custom domain resolver.
    const customDomain = host.slice("admin.".length);
    try {
      const res = await fetch(
        `${MARKETPLACE_API_URL}/api/v1/storefront/resolve-domain?domain=${encodeURIComponent(customDomain)}`,
        { cache: "no-store" },
      );
      if (res.ok) {
        const body = (await res.json()) as { slug?: string };
        if (body.slug) requestedSlug = body.slug;
      }
    } catch {
      // fall through
    }
  }
  // Tenant-subdomain handling. When the user lands on
  // {slug}-admin.mark8ly.com with a session for a tenant that owns that
  // slug — perfect, no-op. When the slug belongs to a tenant the user
  // also has membership on — opportunistically auto-switch their session
  // so the rendered data lines up with the URL bar. When neither — let
  // the request fall through to the normal session/role check below;
  // the user sees their own tenant's data, the URL bar keeps the slug
  // subdomain they typed, and they can escape via the in-app store
  // switcher. Earlier iterations redirected the failure case to
  // /pick-tenant, which created a /dashboard ↔ /pick-tenant ping-pong
  // for single-tenant users (the picker auto-redirects to /dashboard
  // when there's only one tenant, and /dashboard re-fires the
  // auto-switch failure here). The /pick-tenant guard remains as a
  // defensive backstop — it's reached when the user clicks the in-app
  // "Switch store" affordance.
  const isPickTenant =
    pathname === "/pick-tenant" || pathname.startsWith("/pick-tenant/");
  if (requestedSlug && !isPickTenant) {
    try {
      const storeRes = await fetch(
        `${PLATFORM_API_URL}/internal/stores/by-slug/${encodeURIComponent(requestedSlug)}`,
        { cache: "no-store" },
      );
      if (storeRes.ok) {
        const body = (await storeRes.json()) as {
          data: { tenant_id: string };
        };
        const requestedTenantId = body.data.tenant_id;
        if (requestedTenantId && requestedTenantId !== session.tenant_id) {
          // Best-effort switch. Success → reload with the new cookie.
          // Failure → fall through; the user keeps their existing
          // tenant context on this URL.
          const switchRes = await fetch(`${AUTH_BFF_URL}/auth/switch-tenant`, {
            method: "POST",
            headers: {
              Cookie: cookieHeader,
              "Content-Type": "application/json",
            },
            body: JSON.stringify({ tenant_id: requestedTenantId }),
            cache: "no-store",
          });
          if (switchRes.ok) {
            const setCookie = switchRes.headers.get("set-cookie");
            const response = NextResponse.redirect(req.nextUrl);
            if (setCookie) response.headers.set("set-cookie", setCookie);
            return response;
          }
          // No membership on the slug's tenant — silent fall-through.
        }
      }
    } catch {
      // platform-api unreachable — fall through to normal flow.
    }
  }

  // Phase O — fetch the caller's role on the workspace tenant. One
  // extra round-trip to platform-api per authenticated request. We
  // don't cache in the cookie (yet) because that would require
  // auth-bff to embed role in session mint, coupling two services
  // on the same redeploy. A dedicated edge cache (Redis or even an
  // in-memory LRU) lands when latency warrants it.
  let role: "owner" | "admin" | "staff" | "viewer" | null = null;
  try {
    const roleRes = await fetch(
      `${PLATFORM_API_URL}/internal/tenants/${session.tenant_id}/me?uid=${encodeURIComponent(session.user_id)}`,
      { cache: "no-store" },
    );
    if (roleRes.ok) {
      const body = (await roleRes.json()) as {
        data: { role: "owner" | "admin" | "staff" | "viewer" };
      };
      role = body.data.role;
    }
  } catch {
    // platform-api unreachable — treat like an auth outage below.
  }

  if (!role) {
    // The session is valid but the user has no role on the tenant
    // (deleted tuple, wrong tenant, FGA down). Fail closed — an
    // admin page without a role is meaningless.
    return redirectToLogin(req);
  }

  // Forward the resolved session to the server component via request
  // headers. Headers are the cleanest Next.js-native way to pass
  // per-request data from middleware to pages.
  const headers = new Headers(req.headers);
  headers.set("x-session-user-id", session.user_id);
  headers.set("x-session-email", session.email);
  headers.set("x-session-tenant-id", session.tenant_id);
  headers.set("x-session-store-id", session.store_id ?? "");
  headers.set("x-session-role", role);

  return NextResponse.next({ request: { headers } });
}

// Behind a reverse proxy (Istio / Cloudflare), `req.nextUrl.origin`
// resolves to the internal pod bind address (e.g. http://0.0.0.0:4202)
// rather than the customer-facing origin. Always prefer the
// x-forwarded-host / x-forwarded-proto headers when present.
function externalOrigin(req: NextRequest): string {
  const forwardedHost = req.headers.get("x-forwarded-host");
  const host = forwardedHost ?? req.headers.get("host") ?? "";
  if (!host) return req.nextUrl.origin;
  const forwardedProto = req.headers.get("x-forwarded-proto");
  const proto =
    forwardedProto ?? (host.startsWith("localhost") || host.startsWith("127.") ? "http" : "https");
  return `${proto}://${host}`;
}

function externalUrl(req: NextRequest): string {
  return `${externalOrigin(req)}${req.nextUrl.pathname}${req.nextUrl.search}`;
}

// React Server Components prefetches cannot follow a cross-origin 3xx —
// the browser issues a `fetch()` for the RSC payload and CORS blocks the
// redirect. Previously this manifested as a visible login-form wipe:
// the hover-triggered RSC prefetch got bounced to the canonical host,
// the fetch failed, the form component re-mounted, and anything the
// user had typed disappeared.
//
// Next.js tags RSC prefetches with a couple of headers depending on the
// trigger path. We accept any of them as a positive signal.
function isRscPrefetch(req: NextRequest): boolean {
  if (req.headers.get("RSC") === "1") return true;
  if (req.headers.get("Next-Router-Prefetch") === "1") return true;
  if (req.nextUrl.searchParams.has("_rsc")) return true;
  return false;
}

function redirectToLogin(req: NextRequest): NextResponse {
  // If CANONICAL_LOGIN_ORIGIN is configured AND the current request is
  // NOT already on that host, bounce the user to the canonical host
  // for sign-in. The session cookie is scoped to .mark8ly.com so once
  // auth-bff mints it at admin.mark8ly.com, every {slug}-admin.mark8ly.com
  // subdomain picks it up without extra plumbing.
  const externalCurrent = externalOrigin(req);
  const useCanonical =
    CANONICAL_LOGIN_ORIGIN && externalCurrent !== CANONICAL_LOGIN_ORIGIN;

  // Never cross-origin redirect an RSC prefetch (see isRscPrefetch
  // above for why). Return a lightweight 401 instead — the RSC client
  // discards the response without poisoning its cache, and the real
  // click-driven navigation will still hit this middleware and get a
  // proper 307 redirect.
  if (useCanonical && isRscPrefetch(req)) {
    return new NextResponse(null, { status: 401 });
  }

  const loginUrl = new URL(
    "/login",
    useCanonical ? CANONICAL_LOGIN_ORIGIN : externalCurrent,
  );
  loginUrl.searchParams.set("returnUrl", externalUrl(req));
  return NextResponse.redirect(loginUrl);
}

export const config = {
  // Match every route except Next internals and static files. The
  // public-prefix check inside middleware() handles /login etc.
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
