// Host-policy helpers for admin middleware.
//
// Two related concerns this codifies:
//
//   1. "What kind of host is this?" — we recognise three families:
//      canonical (admin.mark8ly.com), tenanted slug subdomains
//      ({slug}-admin.mark8ly.com), and admin custom domains
//      (admin.<merchant-domain>). Anything else is unknown and must
//      be 404'd at the edge — silently rendering the canonical login
//      on a wildcard like `abc-admin.mark8ly.com` invites brand
//      impersonation and confusion.
//
//   2. "Should this path render on the canonical host?" — the canonical
//      host is auth + tenant-picker only. Tenant-scoped routes
//      (/dashboard, /orders, /products, …) must live on a tenanted
//      slug subdomain so the URL bar always identifies the store the
//      operator is acting on. Stray hits to e.g. `admin.mark8ly.com/orders`
//      are bounced to `{slug}-admin.mark8ly.com/orders` after we resolve
//      the user's primary slug.
//
// Both helpers are pure functions so the middleware integration test
// can pin the contract without standing up a request mock.

export type HostClassification =
  | { kind: "canonical" }
  | { kind: "slug"; slug: string }
  | { kind: "custom_admin"; domain: string }
  | { kind: "unknown" };

/**
 * Recognised mark8ly TLDs. Matches apps/storefront/lib/slug.ts so the
 * two apps treat dev/staging hosts identically. Add new TLDs here
 * (NOT scattered through middleware) if/when they ship.
 */
const MARK8LY_TLDS = new Set(["mark8ly.com", "mark8ly.dev", "lvh.me"]);

export function classifyAdminHost(hostHeader: string | null | undefined): HostClassification {
  if (!hostHeader) return { kind: "unknown" };
  const host = hostHeader.split(":")[0]?.toLowerCase() ?? "";
  if (!host) return { kind: "unknown" };

  // Localhost / dev IPs aren't tenanted — treat as canonical so dev
  // iteration on `localhost:4202` keeps working.
  if (host === "localhost" || host === "127.0.0.1") return { kind: "canonical" };

  const parts = host.split(".");
  if (parts.length < 2) return { kind: "unknown" };
  const tld = parts.slice(-2).join(".");

  if (MARK8LY_TLDS.has(tld)) {
    // mark8ly-controlled TLD. The first label decides the kind.
    const first = parts[0] ?? "";
    if (first === "admin" || first === "admin-uat") return { kind: "canonical" };
    if (first.endsWith("-admin")) {
      const slug = first.slice(0, -"-admin".length);
      // {slug}-admin where slug is empty is malformed (e.g. "-admin.mark8ly.com").
      if (!slug) return { kind: "unknown" };
      return { kind: "slug", slug };
    }
    // UAT mirror — `{slug}-admin.mark8ly.com` becomes
    // `{slug}-admin-uat.mark8ly.com` in the cluster-co-located UAT env.
    // Same slug, different host suffix. Prod hosts take the branches
    // above first, so this is strictly additive.
    if (first.endsWith("-admin-uat")) {
      const slug = first.slice(0, -"-admin-uat".length);
      if (!slug) return { kind: "unknown" };
      return { kind: "slug", slug };
    }
    // Other mark8ly subdomains (api., www., …) aren't admin surfaces.
    return { kind: "unknown" };
  }

  // Off-mark8ly host. The admin app only serves `admin.<merchant-domain>`
  // for custom-domain merchants; anything else routed here in error.
  if (parts[0] === "admin" && parts.length >= 3) {
    return { kind: "custom_admin", domain: parts.slice(1).join(".") };
  }
  return { kind: "unknown" };
}

/**
 * Paths that render on the canonical admin host. Everything else must
 * live on a tenanted slug subdomain — see middleware redirect.
 *
 * Keep this list in lockstep with admin's app/ routing. New
 * tenant-agnostic routes (auth, invites, billing-callbacks) belong
 * here; new tenant-scoped routes do not.
 */
export function isCanonicalAllowedPath(pathname: string): boolean {
  return CANONICAL_ALLOWED_PREFIXES.some((p) => pathname === p || pathname.startsWith(p + "/"));
}

const CANONICAL_ALLOWED_PREFIXES: string[] = [
  "/login",
  "/logout",
  "/forgot-password",
  "/reset-password",
  "/accept-invite",
  "/pick-tenant",
  "/pricing",
  "/webhooks",
  "/api/health",
  "/_next",
  "/favicon",
  "/icon-",
];

/**
 * isValidSlugReturnUrl checks whether a `?returnUrl=…` value points at a
 * legitimate slug-admin or admin custom-domain URL. The middleware
 * uses this to gate the canonical /login page: only middleware-bounced
 * traffic from a real slug-admin carries a returnUrl, and direct
 * visits to admin.mark8ly.com/login fall through to a 404 instead of
 * rendering a discoverable login form.
 *
 * Required shape:
 *   • parseable absolute URL
 *   • https
 *   • host matches `{slug}-admin.mark8ly.{com,dev}` with non-empty slug
 *     OR `admin.<off-mark8ly-host>` with at least three labels
 *
 * We deliberately do NOT verify here that the slug is onboarded — the
 * slug-existence check fires later in middleware when the user reaches
 * the slug-admin URL itself; adding a platform-api fetch on every
 * /login render would slow the auth path. A crafted-but-fake returnUrl
 * gets a login form, but logging in bounces to a slug-admin that 404s,
 * so nothing data-leaks.
 */
export function isValidSlugReturnUrl(raw: string | null | undefined): boolean {
  if (!raw) return false;
  try {
    const u = new URL(raw);
    if (u.protocol !== "https:") return false;
    const host = u.host.toLowerCase();
    if (/^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?-admin\.mark8ly\.(?:com|dev)$/.test(host)) {
      return true;
    }
    if (
      host.startsWith("admin.") &&
      host.split(".").length >= 3 &&
      !host.endsWith(".mark8ly.com") &&
      !host.endsWith(".mark8ly.dev")
    ) {
      return true;
    }
    return false;
  } catch {
    return false;
  }
}
