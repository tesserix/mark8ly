// Host-policy helpers for storefront middleware.
//
// Mirrors apps/admin/lib/auth/host-policy but for the storefront's
// {slug}.mark8ly.com URL shape. The mark8ly.com / www.mark8ly.com
// apex is reserved for marketing (a different app) — if a request
// for it ever lands on the storefront pod (Istio config drift,
// dev shortcut, …) we 404 instead of silently rendering DEFAULT_STORE
// content under the wrong brand.

export type StorefrontHostClassification =
  | { kind: "marketing" } // mark8ly.com / www.mark8ly.com — not a store
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
  "internal-identity",
  "identity",
]);

export function classifyStorefrontHost(
  hostHeader: string | null | undefined,
): StorefrontHostClassification {
  if (!hostHeader) return { kind: "unknown" };
  const host = hostHeader.split(":")[0]?.toLowerCase() ?? "";
  if (!host) return { kind: "unknown" };

  if (host === "localhost" || host === "127.0.0.1") return { kind: "marketing" };

  const parts = host.split(".");
  if (parts.length < 2) return { kind: "unknown" };
  const tld = parts.slice(-2).join(".");

  if (MARK8LY_TLDS.has(tld)) {
    if (parts.length === 2) {
      // Bare apex — mark8ly.com itself. Marketing site, not storefront.
      return { kind: "marketing" };
    }
    const first = parts[0] ?? "";
    if (!first) return { kind: "unknown" };
    if (RESERVED_SUBDOMAINS.has(first)) return { kind: "marketing" };
    if (first.endsWith("-admin")) return { kind: "unknown" }; // admin host hit storefront pod
    return { kind: "slug", slug: first };
  }

  // Off-mark8ly: presumed custom domain. Caller must resolve via the
  // marketplace-api custom-domain endpoint and 404 if not registered.
  return { kind: "custom", domain: host };
}
