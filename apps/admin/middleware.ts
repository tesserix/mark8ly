import { NextResponse, type NextRequest } from "next/server";
import { applyGeoCookie } from "./lib/geo/geoMiddleware";
import {
  classifyAdminHost,
  isCanonicalAllowedPath,
} from "./lib/auth/host-policy";

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
  const hostHeader = req.headers.get("host") ?? "";
  const hostKind = classifyAdminHost(hostHeader);

  // Edge guard: reject hosts the admin app should never serve. This is
  // the wildcard-subdomain check — structurally-malformed hosts get an
  // immediate 404 before any rendering so the wildcard never serves
  // the canonical login form (which would invite phishing / brand
  // impersonation: a victim sees a Mark8ly-branded login on a host
  // that isn't actually theirs). Static assets + health probes are
  // exempt because CDNs / kubelet hit them on whatever host they were
  // warmed against.
  if (hostKind.kind === "unknown" && !isStaticOrHealthPath(pathname)) {
    return new NextResponse(null, { status: 404 });
  }

  // Same wildcard block, semantic flavour: the host is structurally a
  // slug subdomain (`{slug}-admin.mark8ly.com`) or admin custom domain,
  // but the slug isn't an onboarded store, OR the slug now has a
  // verified custom-domain takeover. Validate BEFORE the public-prefix
  // short-circuit below so `abc-admin.mark8ly.com/login` 404s instead
  // of rendering the canonical login form. Static + health paths skip
  // the lookup so probes don't fail closed during a platform-api blip.
  if (
    (hostKind.kind === "slug" || hostKind.kind === "custom_admin") &&
    !isStaticOrHealthPath(pathname)
  ) {
    if (hostKind.kind === "slug") {
      const lookup = await fetchSlugStatus(hostKind.slug);
      if (lookup === "not_found") {
        return new NextResponse(null, { status: 404 });
      }
      if (lookup && lookup.customDomain) {
        // Takeover: the merchant has a verified custom domain.
        // Permanently retire the platform `{slug}-admin.mark8ly.com`
        // URL by 301'ing to `admin.<custom-domain><path>`. This
        // preserves bookmarks / inbound links and transfers SEO.
        const target = new URL(req.nextUrl);
        target.host = `admin.${lookup.customDomain}`;
        target.protocol = "https:";
        target.port = "";
        return NextResponse.redirect(target, 301);
      }
      // null → platform-api unreachable. Fail open.
    } else {
      // custom_admin host (`admin.<merchant>`). Existence is checked
      // via the resolve-domain endpoint that already lives on
      // marketplace-api.
      const exists = await customDomainExists(hostKind.domain);
      if (exists === false) {
        return new NextResponse(null, { status: 404 });
      }
    }
  }

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
  let requestedSlug: string | null = null;
  if (hostKind.kind === "slug") {
    requestedSlug = hostKind.slug;
  } else if (hostKind.kind === "custom_admin") {
    // admin.<merchant-domain>. Resolve via marketplace-api's
    // custom-domain resolver.
    try {
      const res = await fetch(
        `${MARKETPLACE_API_URL}/api/v1/storefront/resolve-domain?domain=${encodeURIComponent(hostKind.domain)}`,
        { cache: "no-store" },
      );
      if (res.ok) {
        const body = (await res.json()) as { slug?: string };
        if (body.slug) requestedSlug = body.slug;
      } else if (res.status === 404) {
        // Custom domain isn't registered on any store. Don't serve the
        // admin app for it — same brand-impersonation concern as
        // wildcard slug subdomains.
        return new NextResponse(null, { status: 404 });
      }
    } catch {
      // platform-api / marketplace-api unreachable: fall through; the
      // role check below will fail closed if we genuinely don't know.
    }
  }
  // Tenant-subdomain handling for {slug}-admin.mark8ly.com URLs.
  //
  // Three outcomes:
  //   1. session.tenant_id == slug's tenant_id  →  no-op, fall through
  //   2. user has membership on the slug's tenant →  switch session and
  //      reload; the buyer of "auto-switch on subdomain mismatch" UX
  //   3. user has no membership on the slug's tenant  →  redirect to
  //      /pick-tenant on the SAME host. The /pick-tenant page is the
  //      single source of truth for "wrong store" recovery: it renders
  //      a picker scoped to the user's actual tenants and never
  //      auto-redirects to /dashboard when the host's slug isn't one
  //      they can access (which is what created the original
  //      /dashboard ↔ /pick-tenant loop).
  //
  // The isPickTenant guard below prevents the /pick-tenant page itself
  // from re-entering this block — it's the defensive bottom turtle that
  // makes (3) safe. Earlier "fall-through" iterations silently rendered
  // the user's own tenant data on a wrong-slug URL; that's the exact
  // cross-tenant UI bleed 67e517bb was trying to prevent in the first
  // place, so we don't do it.
  const isPickTenant =
    pathname === "/pick-tenant" || pathname.startsWith("/pick-tenant/");
  if (requestedSlug && !isPickTenant) {
    try {
      const storeRes = await fetch(
        `${PLATFORM_API_URL}/internal/stores/by-slug/${encodeURIComponent(requestedSlug)}`,
        { cache: "no-store" },
      );
      // Hard 404 when the slug isn't a known store — this is the
      // wildcard-subdomain block: `abc-admin.mark8ly.com` for an
      // unonboarded `abc` no longer falls through to render the
      // canonical login form.
      if (storeRes.status === 404) {
        return new NextResponse(null, { status: 404 });
      }
      if (storeRes.ok) {
        const body = (await storeRes.json()) as {
          data: { tenant_id: string };
        };
        const requestedTenantId = body.data.tenant_id;
        if (requestedTenantId && requestedTenantId !== session.tenant_id) {
          // Try to switch. If the user has membership on the slug's
          // tenant, auth-bff mints a new cookie and we reload.
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
          // Switch failed — no membership on this tenant. Hand off to
          // /pick-tenant on the same host; the page detects the
          // wrong-slug condition and renders the picker instead of
          // auto-redirecting to /dashboard.
          return NextResponse.redirect(new URL("/pick-tenant", req.nextUrl));
        }
      }
    } catch {
      // platform-api unreachable — fall through to normal flow rather
      // than redirect to /pick-tenant (which would also depend on
      // platform-api). Better to render the page with the user's
      // existing tenant than to send them to a broken picker.
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

  // Canonical-host policy: tenant-scoped routes (/dashboard, /orders,
  // /products, …) must live on a tenanted slug subdomain so the URL
  // bar always identifies the active store. If the user lands here
  // by typing `admin.mark8ly.com/orders` directly, redirect them to
  // `{slug}-admin.mark8ly.com/orders` using the session's primary
  // store slug. /pick-tenant + auth utility paths render here as
  // designed.
  if (
    hostKind.kind === "canonical" &&
    !isCanonicalAllowedPath(pathname) &&
    !isStaticOrHealthPath(pathname) &&
    session.store_id
  ) {
    const slug = await fetchPrimaryStoreSlug(session.store_id);
    if (slug) {
      const target = new URL(req.nextUrl);
      target.host = `${slug}-admin.mark8ly.com`;
      target.protocol = "https:";
      target.port = "";
      return NextResponse.redirect(target);
    }
    // Couldn't resolve a slug — send them to /pick-tenant as a
    // fallback rather than rendering tenant data on the canonical host.
    return NextResponse.redirect(new URL("/pick-tenant", req.nextUrl));
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

// SlugStatus is the result of fetchSlugStatus: either the slug is
// unknown (`"not_found"`), or the slug exists and we know whether the
// merchant has a verified custom-domain takeover via `customDomain`.
// `null` is a transient failure (platform-api / marketplace-api
// unreachable) — caller fails open.
type SlugStatus =
  | "not_found"
  | { kind: "ok"; customDomain: string };

// fetchSlugStatus calls marketplace-api's
// `/internal/store-active-domain/:slug` (added in this PR). One round
// trip gives us both:
//   - whether the slug is an onboarded store (404 → not_found), and
//   - whether that store has a verified custom-domain takeover (200 +
//     non-empty `custom_domain`).
//
// Falls open (`null`) on any non-2xx / non-404 response so a transient
// marketplace-api blip doesn't 404 a real merchant.
async function fetchSlugStatus(slug: string): Promise<SlugStatus | null> {
  if (!slug) return "not_found";
  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/internal/store-active-domain/${encodeURIComponent(slug)}`,
      { cache: "no-store" },
    );
    if (res.status === 404) return "not_found";
    if (!res.ok) return null;
    const body = (await res.json().catch(() => ({}))) as {
      slug?: string;
      custom_domain?: string;
    };
    return {
      kind: "ok",
      customDomain: (body.custom_domain ?? "").trim().toLowerCase(),
    };
  } catch {
    return null;
  }
}

// customDomainExists confirms an admin custom-domain (admin.<merchant>)
// is registered. Same true/false/null contract as slugExists.
async function customDomainExists(domain: string): Promise<boolean | null> {
  if (!domain) return false;
  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/resolve-domain?domain=${encodeURIComponent(domain)}`,
      { cache: "no-store" },
    );
    if (res.ok) {
      const body = (await res.json().catch(() => ({}))) as { slug?: string };
      return Boolean(body.slug);
    }
    if (res.status === 404) return false;
    return null;
  } catch {
    return null;
  }
}

// fetchPrimaryStoreSlug resolves a store_id to its slug via platform-api.
// Used by the canonical-host redirect to send `admin.mark8ly.com/orders`
// to the right `{slug}-admin.mark8ly.com/orders` for the user's active
// session. Returns null on any failure — caller falls back to
// /pick-tenant rather than rendering tenant data on the canonical host.
async function fetchPrimaryStoreSlug(storeID: string): Promise<string | null> {
  if (!storeID) return null;
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/stores/${encodeURIComponent(storeID)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data?: { slug?: string } };
    return body.data?.slug ?? null;
  } catch {
    return null;
  }
}

// isStaticOrHealthPath is a narrow allow-list for paths that bypass the
// host-classification 404 — Next static assets and the kubelet probe.
// Everything else on an unknown host gets rejected upstream.
function isStaticOrHealthPath(pathname: string): boolean {
  return (
    pathname.startsWith("/_next") ||
    pathname.startsWith("/favicon") ||
    pathname.startsWith("/icon-") ||
    pathname === "/api/health"
  );
}

export const config = {
  // Match every route except Next internals and static files. The
  // public-prefix check inside middleware() handles /login etc.
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
