/**
 * Extracts a store slug from a Host header.
 *
 * Production layout:
 *   {slug}.mark8ly.com          → "{slug}"   (storefront)
 *   {slug}-admin.mark8ly.com    → served by admin, not here
 *   www.mark8ly.com             → marketing site, not a store
 *   shop.mybrand.com            → custom domain → resolved via API
 *
 * For custom domains (any host that isn't a mark8ly.com subdomain),
 * `slugFromHost` returns null and the caller should try
 * `resolveCustomDomain` to look up the slug via the marketplace API.
 */
const RESERVED_SUBDOMAINS = new Set(["www", "api", "api-v1"]);

const MARK8LY_TLDS = new Set(["mark8ly.com", "mark8ly.dev", "lvh.me"]);

export function slugFromHost(host: string | null): string | null {
  if (!host) return null;
  const withoutPort = host.split(":")[0] ?? "";
  if (!withoutPort) return null;
  if (withoutPort === "localhost" || withoutPort === "127.0.0.1") {
    return null;
  }
  const parts = withoutPort.split(".");
  if (parts.length < 2) return null;
  // Only treat the first label as a slug when the host lives under a
  // mark8ly-controlled TLD. Custom domains (like primasyss.com) must
  // fall through to the resolveCustomDomain API lookup — returning
  // "primasyss" here would send users to a "Store not found" page.
  const parent = parts.slice(-2).join(".");
  if (!MARK8LY_TLDS.has(parent)) return null;
  const sub = parts[0] ?? "";
  if (!sub || RESERVED_SUBDOMAINS.has(sub)) return null;
  if (sub.endsWith("-admin")) return null;
  // Admin UAT subdomain ({slug}-admin-uat) is the admin app's host —
  // ignore here so the storefront pod doesn't try to render it.
  if (sub.endsWith("-admin-uat")) return null;
  // UAT mirror — `{slug}.mark8ly.com` becomes `{slug}-uat.mark8ly.com`
  // in the cluster-co-located UAT env. Strip the suffix to recover the
  // canonical slug; prod hosts (`{slug}.mark8ly.com`) skip this branch
  // because they don't end in `-uat`.
  if (sub.endsWith("-uat")) {
    const baseSlug = sub.slice(0, -"-uat".length);
    if (!baseSlug) return null;
    return baseSlug;
  }
  return sub;
}

/**
 * Returns true when the host is a custom domain (not a mark8ly.com
 * subdomain). Used by the layout to decide whether to call
 * `resolveCustomDomain`.
 */
export function isCustomDomain(host: string | null): boolean {
  if (!host) return false;
  const withoutPort = host.split(":")[0] ?? "";
  if (!withoutPort || withoutPort === "localhost" || withoutPort === "127.0.0.1") {
    return false;
  }
  const parts = withoutPort.split(".");
  if (parts.length < 2) return false;
  const tld = parts.slice(-2).join(".");
  return !MARK8LY_TLDS.has(tld);
}

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL || "http://localhost:8080";

/**
 * Resolves a custom domain to a store slug by calling the marketplace
 * API's public resolve-domain endpoint. Returns null if the domain
 * isn't registered or not yet verified.
 */
/**
 * Full slug resolution: subdomain first, then custom domain API, then
 * DEFAULT_STORE_SLUG env var. Single await, cached for 5 minutes by
 * Next.js fetch. Every page/layout should use this instead of raw
 * slugFromHost + fallback.
 */
export async function resolveStoreSlug(
  host: string | null,
): Promise<string> {
  const sub = slugFromHost(host);
  if (sub) return sub;

  if (isCustomDomain(host)) {
    const withoutPort = host!.split(":")[0] ?? "";
    const resolved = await resolveCustomDomain(withoutPort);
    if (resolved) return resolved;
  }

  return process.env.DEFAULT_STORE_SLUG || "default";
}

export async function resolveCustomDomain(
  domain: string,
): Promise<string | null> {
  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/resolve-domain?domain=${encodeURIComponent(domain)}`,
      { next: { revalidate: 300 } },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { slug?: string };
    if (!body.slug) return null;
    return body.slug;
  } catch {
    return null;
  }
}
