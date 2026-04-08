import type { MetadataRoute } from "next";
import { headers } from "next/headers";

import { slugFromHost } from "@/lib/slug";
import { canonicalUrl } from "@/lib/seo";

/**
 * Per-tenant robots.txt.
 *
 * Reads the request host so each subdomain gets a robots file scoped
 * to its own canonical URL. /api is always disallowed; the rest of
 * the storefront is fair game for crawlers.
 */
export default async function robots(): Promise<MetadataRoute.Robots> {
  const h = await headers();
  const host = h.get("host");
  const slug = slugFromHost(host) ?? process.env.DEFAULT_STORE_SLUG ?? "";
  const base = slug ? canonicalUrl(slug) : `https://${host ?? "mark8ly.com"}`;

  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: ["/api/", "/_next/"],
      },
    ],
    sitemap: `${base}/sitemap.xml`,
    host: base,
  };
}
