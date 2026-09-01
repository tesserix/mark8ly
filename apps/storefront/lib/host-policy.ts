// Host-policy helpers for storefront middleware.
//
// Mirrors apps/admin/lib/auth/host-policy but for the storefront's
// {slug}.mark8ly.com URL shape. The mark8ly.com / www.mark8ly.com
// apex is reserved for marketing (a different app) — if a request
// for it ever lands on the storefront pod (Istio config drift,
// dev shortcut, …) we must not silently render DEFAULT_STORE content
// under the wrong brand.
//
// `www` is the one marketing host that gets special treatment. It is a
// real, public, brand-owned hostname that people type and Google crawls,
// and in prod it currently lands here rather than on the marketing app.
// Serving it a "Store not found" body with a 200 makes it a soft 404 on
// the brand's own front door (#147). So `www` carries the canonical apex
// to 301 to; every other marketing host carries `null` and is passed
// through unchanged, because a hit on `api.` or `admin.` is routing
// drift we'd rather surface than paper over with a redirect.

export type StorefrontHostClassification =
  // mark8ly.com / www.mark8ly.com / api. / admin. — not a store.
  // `redirectTo` is the apex host to 301 to, or null to pass through.
  | { kind: "marketing"; redirectTo: string | null }
  | { kind: "slug"; slug: string } // {slug}.mark8ly.com
  | { kind: "custom"; domain: string } // off-mark8ly host — resolve via API
  | { kind: "unknown" };

const MARK8LY_TLDS = new Set(["mark8ly.com", "mark8ly.dev", "lvh.me"]);

// Subdomains under mark8ly-controlled TLDs that aren't tenants. Add to
// this list when new platform-level subdomains ship. Anything not on
// this list AND not ending in `-admin` is treated as a tenant slug
// candidate and validated by the middleware against platform-api.
const RESERVED_SUBDOMAINS = new Set([
  "www",
  "api",
  "api-v1",
  "admin", // canonical admin host — not a storefront slug
  "admin-uat", // canonical UAT admin host
  "uat", // canonical UAT storefront host (the apex equivalent for UAT)
  "uat-landing", // UAT onboarding landing host
  "onboarding-uat", // UAT onboarding wizard host
  "internal-identity",
  "identity",
]);

export function classifyStorefrontHost(
  hostHeader: string | null | undefined,
): StorefrontHostClassification {
  if (!hostHeader) return { kind: "unknown" };
  const host = hostHeader.split(":")[0]?.toLowerCase() ?? "";
  if (!host) return { kind: "unknown" };

  // Local dev serves the storefront straight off localhost — never
  // bounce a developer out to the production marketing site.
  if (host === "localhost" || host === "127.0.0.1") {
    return { kind: "marketing", redirectTo: null };
  }

  const parts = host.split(".");
  if (parts.length < 2) return { kind: "unknown" };
  const tld = parts.slice(-2).join(".");

  if (MARK8LY_TLDS.has(tld)) {
    if (parts.length === 2) {
      // Bare apex — mark8ly.com itself. Marketing site, not storefront.
      // No redirect target: it IS the canonical host, and pointing it at
      // itself would loop.
      return { kind: "marketing", redirectTo: null };
    }
    const first = parts[0] ?? "";
    if (!first) return { kind: "unknown" };
    if (RESERVED_SUBDOMAINS.has(first)) {
      // `www.<tld>` is the public brand alias — send it to the apex of
      // its own TLD so mark8ly.dev / lvh.me dev hosts stay in their env.
      return { kind: "marketing", redirectTo: first === "www" ? tld : null };
    }
    if (first.endsWith("-admin")) return { kind: "unknown" }; // admin host hit storefront pod
    if (first.endsWith("-admin-uat")) return { kind: "unknown" }; // UAT admin host hit storefront pod
    // UAT mirror: `{slug}-uat.mark8ly.com` is the UAT analog of prod's
    // `{slug}.mark8ly.com`. Strip the `-uat` suffix to recover the
    // canonical slug. Prod hosts skip this branch entirely.
    if (first.endsWith("-uat")) {
      const slug = first.slice(0, -"-uat".length);
      if (!slug) return { kind: "unknown" };
      return { kind: "slug", slug };
    }
    return { kind: "slug", slug: first };
  }

  // Off-mark8ly: presumed custom domain. Caller must resolve via the
  // marketplace-api custom-domain endpoint and 404 if not registered.
  return { kind: "custom", domain: host };
}
