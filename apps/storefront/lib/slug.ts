/**
 * Extracts a store slug from a Host header.
 *
 * Production layout:
 *   {slug}.mark8ly.com          → "{slug}"   (storefront)
 *   {slug}-admin.mark8ly.com    → served by admin, not here
 *   www.mark8ly.com             → marketing site, not a store
 *
 * Examples:
 *   acme.mark8ly.com              → "acme"
 *   acme.staging.mark8ly.com      → "acme"
 *   acme.lvh.me:4203              → "acme" (dev loopback)
 *   localhost:4203                → null (use DEFAULT_STORE_SLUG)
 *
 * Subdomains ending in `-admin` are rejected — those belong to
 * the admin app, not the storefront. Named reserved subdomains
 * (www, api) also return null so they can be used for the
 * marketing site or platform APIs.
 */
const RESERVED_SUBDOMAINS = new Set(["www", "api", "api-v1"]);

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
