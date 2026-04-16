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
  const sub = parts[0] ?? "";
  if (!sub || RESERVED_SUBDOMAINS.has(sub)) return null;
  if (sub.endsWith("-admin")) return null;
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
    // slug comes back as "india-store.mark8ly.com" — extract just the slug part
    const slug = body.slug.split(".")[0];
    return slug || null;
  } catch {
    return null;
  }
}
