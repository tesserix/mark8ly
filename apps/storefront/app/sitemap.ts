import type { MetadataRoute } from "next";
import { headers } from "next/headers";

import { fetchStoreBySlug } from "@/lib/api/platform-api";
import {
  listProducts,
  listCategories,
} from "@/lib/api/marketplace-api";
import { slugFromHost } from "@/lib/slug";
import { canonicalUrl } from "@/lib/seo";

/**
 * Per-tenant sitemap.
 *
 * Resolves the tenant from the request host, then emits the home,
 * /products, every category landing, and every published product page.
 * Product + category enumeration is best-effort; failures still ship
 * the home so the sitemap stays valid.
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

  const entries: MetadataRoute.Sitemap = [
    {
      url: base,
      lastModified: now,
      changeFrequency: "daily",
      priority: 1,
    },
    {
      url: `${base}/products`,
      lastModified: now,
      changeFrequency: "daily",
      priority: 0.9,
    },
  ];

  // Categories — best-effort, never fails the sitemap.
  const categories = await listCategories(slug).catch(() => []);
  for (const cat of categories) {
    entries.push({
      url: `${base}/categories/${encodeURIComponent(cat.slug)}`,
      lastModified: now,
      changeFrequency: "weekly",
      priority: 0.7,
    });
  }

  // Products — fan-out across pages until exhausted, capped at 10 pages
  // (200 products) to keep the sitemap small. Stores larger than that
  // need a chunked sitemap index, which is a follow-up.
  const seen = new Set<string>();
  for (let page = 1; page <= 10; page++) {
    const response = await listProducts(slug, { page, pageSize: 20 }).catch(
      () => null,
    );
    if (!response || response.data.length === 0) break;
    for (const p of response.data) {
      if (seen.has(p.handle)) continue;
      seen.add(p.handle);
      entries.push({
        url: `${base}/products/${encodeURIComponent(p.handle)}`,
        lastModified: p.published_at ? new Date(p.published_at) : now,
        changeFrequency: "weekly",
        priority: 0.8,
      });
    }
    if (response.data.length < 20) break;
  }

  return entries;
}
