import type { Metadata } from "next";

import type { PublicStore } from "@/lib/api/platform-api";

/**
 * Per-tenant metadata helper.
 *
 * Every storefront page calls this from `generateMetadata` so the
 * page-level title, description, canonical URL, OG tags, and JSON-LD
 * Store schema all derive from the same source of truth — the live
 * tenant record. When the store can't be resolved we return a quiet
 * "Store not found" surface that stays out of the index.
 */
export function makeTenantMetadata(
  store: PublicStore | null,
  slug: string,
): Metadata {
  if (!store) {
    return {
      title: "Store not found",
      description:
        "This Mark8ly store either isn't live yet or the URL is wrong.",
      robots: { index: false, follow: false },
    };
  }

  const canonical = canonicalUrl(store.slug);
  const description =
    `Shop ${store.name} on Mark8ly. ${store.country_code} · ` +
    `${store.currency_code}. Powered by Mark8ly.`;

  return {
    metadataBase: new URL(canonical),
    title: {
      default: store.name,
      template: `%s · ${store.name}`,
    },
    description,
    alternates: {
      canonical,
    },
    openGraph: {
      type: "website",
      url: canonical,
      title: store.name,
      description,
      siteName: store.name,
    },
    twitter: {
      card: "summary_large_image",
      title: store.name,
      description,
    },
    robots: {
      index: true,
      follow: true,
    },
    other: {
      "store:slug": store.slug,
      "store:currency": store.currency_code,
      "store:country": store.country_code,
      // JSON-LD Store schema is added inline by the page so the helper
      // stays serializable. See app/page.tsx for the script tag.
    },
  };
}

/**
 * Canonical URL for a tenant. Honors STOREFRONT_BASE_DOMAIN so
 * staging/dev environments produce the right host.
 */
export function canonicalUrl(slug: string): string {
  const base = process.env.STOREFRONT_BASE_DOMAIN ?? "mark8ly.com";
  const protocol = base.includes("localhost") ? "http" : "https";
  return `${protocol}://${slug}.${base}`;
}

/**
 * JSON-LD Store schema. Inline this in the page via a
 * <script type="application/ld+json"> tag.
 */
export function storeJsonLd(store: PublicStore): string {
  const url = canonicalUrl(store.slug);
  return JSON.stringify({
    "@context": "https://schema.org",
    "@type": "Store",
    name: store.name,
    url,
    currenciesAccepted: store.currency_code,
    address: {
      "@type": "PostalAddress",
      addressCountry: store.country_code,
    },
  });
}
