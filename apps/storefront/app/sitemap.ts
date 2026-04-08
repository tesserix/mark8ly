import type { MetadataRoute } from "next";
import { headers } from "next/headers";

import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { slugFromHost } from "@/lib/slug";
import { canonicalUrl } from "@/lib/seo";

/**
 * Per-tenant sitemap.
 *
 * Resolves the tenant from the request host, then emits the home
 * URL plus any product URLs the platform-api returns. The product
 * fetch is best-effort: if it fails we still emit the home so the
 * sitemap stays valid.
 */
export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const h = await headers();
  const host = h.get("host");
  const slug = slugFromHost(host) ?? process.env.DEFAULT_STORE_SLUG ?? "";

  if (!slug) return [];

  const store = await fetchStoreBySlug(slug).catch(() => null);
  if (!store) return [];

  const base = canonicalUrl(store.slug);
  const now = new Date();

  // Future: fan out to platform-api /products and emit per-product
  // entries. For now we ship the home so the sitemap is at least valid
  // and discoverable.
  return [
    {
      url: base,
      lastModified: now,
      changeFrequency: "daily",
      priority: 1,
    },
  ];
}
