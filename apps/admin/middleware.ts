import { NextResponse, type NextRequest } from "next/server";
import { applyGeoCookie } from "./lib/geo/geoMiddleware";
import {
  classifyAdminHost,
  isCanonicalAllowedPath,
  isValidSlugReturnUrl,
} from "./lib/auth/host-policy";
import { platformInternalHeaders } from "./lib/api/server/platformInternal";
import { isCrossOriginStateChange } from "@repo/ui/auth/csrf";

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
  "/auth/handoff", // cross-TLD admin handoff — mints a session for this host, so it MUST render before the cookie exists
  "/webhooks", // external provider callbacks (Stripe, etc.) — never gated
  "/api/health", // kubelet probe target; must not 30x to /login
  "/api/analytics-config", // non-secret client id, read before sign-in
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

  // CSRF: SameSite=Lax stops third-party sites but not sibling tenant
  // subdomains, which share the cookie's .mark8ly.com scope.
  if (isCrossOriginStateChange(req)) {
    return NextResponse.json(
      { error: "forbidden", message: "cross-origin request rejected" },
      { status: 403 },
    );
  }
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

  // Social sign-in (Google GSI / Apple JS) only works on the canonical
  // login origin — it's the only host registered as an Authorized
  // JavaScript origin with Google and a Website URL with Apple. A
  // /login render on a tenant host shows social buttons that ALWAYS
  // fail (Google `origin_mismatch`, Apple `invalid_client`), so bounce
  // it to the canonical host instead. This runs AFTER the
  // slug-existence check above so unonboarded slugs still 404, and
  // BEFORE the PUBLIC_PREFIXES short-circuit that would otherwise
  // render the form in place. A returnUrl already carried on the
  // request is preserved when valid; otherwise the tenant's /dashboard
  // is the post-auth landing (canonical /login 404s without a valid
  // returnUrl, so we must always send one).
  if (
    (hostKind.kind === "slug" || hostKind.kind === "custom_admin") &&
    (pathname === "/login" || pathname === "/login/")
  ) {
    const canonical =
      deriveCanonicalLoginOrigin(req) ?? (CANONICAL_LOGIN_ORIGIN || null);
    if (canonical) {
      // Same RSC-prefetch guard as redirectToLogin — a cross-origin 3xx
      // on a prefetch poisons the RSC cache; the real click still gets
      // the redirect.
      if (isRscPrefetch(req)) {
        return new NextResponse(null, { status: 401 });
      }
      const carried = req.nextUrl.searchParams.get("returnUrl");
      const returnUrl = isValidSlugReturnUrl(carried)
        ? (carried as string)
        : `${externalOrigin(req)}/dashboard`;
      const loginUrl = new URL("/login", canonical);
      loginUrl.searchParams.set("returnUrl", returnUrl);
      return NextResponse.redirect(loginUrl);
    }
  }

  // Public pricing page — geo-localize currency from CF-IPCountry before RSC
  // render. No auth or tenant extraction needed; the page is unauthenticated.
  if (pathname === "/pricing" || pathname.startsWith("/pricing/")) {
    const response = NextResponse.next();
    applyGeoCookie(req, response);
    return response;
  }

  // Canonical /login isn't meant for direct visits. The merchant logs
  // in via THEIR slug-admin URL (`{slug}-admin.mark8ly.com`); the form
  // only ends up on canonical because that's the host registered as
  // a Google OAuth Authorized JavaScript origin, and middleware
  // bounces unauthenticated slug-admin traffic here mid-flow with a
  // `returnUrl` pointing back at the slug. Without that returnUrl
  // there's no legitimate reason to be on this URL — 404 instead of
  // rendering a discoverable login form.
  //
  // Same reasoning for /forgot-password, /reset-password,
  // /accept-invite, /pick-tenant, /logout: they're either
  // email-link landings (carry a token in the path/query and don't
  // need a returnUrl gate) or post-auth utilities. Login is the only
  // one a casual visitor could "just type" and expect a form.
  if (
    hostKind.kind === "canonical" &&
    (pathname === "/login" || pathname === "/login/") &&
    !isValidSlugReturnUrl(req.nextUrl.searchParams.get("returnUrl"))
  ) {
    return new NextResponse(null, { status: 404 });
  }

  if (PUBLIC_PREFIXES.some((p) => pathname.startsWith(p))) {
    return NextResponse.next();
  }

  const cookie = req.cookies.get(SESSION_COOKIE_NAME);
  if (!cookie || !cookie.value) {
    // Strict canonical-host policy: the bare `admin.mark8ly.com` host
    // isn't tenant-onboarded — anything that isn't an explicit auth /
    // invite / utility path returns 404 to anonymous traffic instead
    // of friendly-bouncing to /login. We don't want `admin.mark8ly.com`
    // (or random sub-paths under it) showing up as discoverable URLs
    // alongside the real slug-admin subdomains. /login itself is on
    // the allow-list and still serves 200 — that's the explicit auth
    // entry point and OAuth callback target.
    if (
      hostKind.kind === "canonical" &&
      !isCanonicalAllowedPath(pathname) &&
      !isStaticOrHealthPath(pathname)
    ) {
      return new NextResponse(null, { status: 404 });
    }
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
        { cache: "no-store", headers: platformInternalHeaders() },
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
      { cache: "no-store", headers: platformInternalHeaders() },
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
  // `{slug}-admin.mark8ly.com/orders`. /pick-tenant + auth utility
  // paths render on canonical as designed.
  //
  // Slug resolution falls back through:
  //   1. session.store_id (when auth-bff embeds it on the cookie)
  //   2. listStoresByTenant(session.tenant_id)[0] — the user's primary
  //      store on the active tenant; this is the path that fires for
  //      sessions that only carry tenant_id (the common case today).
  // If both resolve nothing, redirect to /pick-tenant rather than
  // render tenant data on the canonical host — the picker is the
  // single sanctioned recovery surface.
  if (
    hostKind.kind === "canonical" &&
    !isCanonicalAllowedPath(pathname) &&
    !isStaticOrHealthPath(pathname)
  ) {
    let slug: string | null = null;
    if (session.store_id) {
      slug = await fetchPrimaryStoreSlug(session.store_id);
    }
    if (!slug && session.tenant_id) {
      slug = await fetchFirstSlugForTenant(session.tenant_id);
    }
    if (slug) {
      const target = new URL(req.nextUrl);
      // Preserve the env suffix from the request host so prod redirects
      // to {slug}-admin.mark8ly.com and UAT redirects to
      // {slug}-admin-uat.mark8ly.com — never cross the env boundary.
      // Hardcoding "admin.mark8ly.com" would punt every UAT user back
      // to prod after sign-in.
      const currentHost = (req.headers.get("host") ?? "").split(":")[0]?.toLowerCase() ?? "";
      const parts = currentHost.split(".");
      const first = parts[0] ?? "";
      const tld = parts.slice(-2).join(".");
      const adminPrefix = first === "admin-uat" ? "admin-uat" : "admin";
      target.host = `${slug}-${adminPrefix}.${tld}`;
      target.protocol = "https:";
      target.port = "";
      return NextResponse.redirect(target);
    }
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

// Derive the canonical login origin from the current request host so
// the SAME image works for both prod and UAT without rebuilding. UAT
// requests on {slug}-admin-uat.mark8ly.com get bounced to
// admin-uat.mark8ly.com (NOT prod's admin.mark8ly.com) — without this,
// the build-time CANONICAL_LOGIN_ORIGIN baked from a single GitHub
// secret would punt every UAT user across to prod login and the
// env isolation breaks.
function deriveCanonicalLoginOrigin(req: NextRequest): string | null {
  const host = (req.headers.get("host") ?? "").split(":")[0]?.toLowerCase() ?? "";
  if (!host) return null;
  const parts = host.split(".");
  if (parts.length < 2) return null;
  const tld = parts.slice(-2).join(".");
  if (tld !== "mark8ly.com" && tld !== "mark8ly.dev") return null;
  const first = parts[0] ?? "";
  // Canonical hosts host the sign-in form themselves — don't loop.
  if (first === "admin" || first === "admin-uat") return null;
  if (first.endsWith("-admin-uat")) return `https://admin-uat.${tld}`;
  if (first.endsWith("-admin")) return `https://admin.${tld}`;
  return null;
}

function redirectToLogin(req: NextRequest): NextResponse {
  // Bounce slug-admin traffic to the matching canonical sign-in host.
  // Prefer the host-derived value over the build-time
  // CANONICAL_LOGIN_ORIGIN env var so the prod and UAT images can be
  // identical — the env-var fallback stays for backward compat with
  // off-mark8ly admin custom domains.
  const externalCurrent = externalOrigin(req);
  const derivedCanonical = deriveCanonicalLoginOrigin(req);
  const targetCanonical = derivedCanonical || CANONICAL_LOGIN_ORIGIN || null;
  const useCanonical = !!targetCanonical && externalCurrent !== targetCanonical;

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
    useCanonical ? (targetCanonical as string) : externalCurrent,
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
// First arm of the canonical-host redirect's slug resolution chain;
// fires when the session cookie carries a store_id. Returns null on
// any failure — caller tries the tenant-id fallback before giving up.
async function fetchPrimaryStoreSlug(storeID: string): Promise<string | null> {
  if (!storeID) return null;
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/stores/${encodeURIComponent(storeID)}`,
      { cache: "no-store", headers: platformInternalHeaders() },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data?: { slug?: string } };
    return body.data?.slug ?? null;
  } catch {
    return null;
  }
}

// fetchFirstSlugForTenant pulls the first (oldest) store slug under
// a tenant. Second arm of the canonical-host redirect's resolution
// chain — fires when the session cookie carries tenant_id but not
// store_id (the common case for sessions minted before auth-bff
// started embedding store_id). Mirrors the platform-api endpoint at
// `GET /internal/tenants/:tenant_id/stores`.
async function fetchFirstSlugForTenant(tenantID: string): Promise<string | null> {
  if (!tenantID) return null;
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/tenants/${encodeURIComponent(tenantID)}/stores`,
      { cache: "no-store", headers: platformInternalHeaders() },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data?: Array<{ slug?: string }> };
    const stores = body.data ?? [];
    return stores[0]?.slug ?? null;
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
