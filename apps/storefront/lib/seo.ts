import type { Metadata } from "next";

import type { PublicStore } from "@/lib/api/platform-api";
import type {
  StorefrontBranding,
  StorefrontProduct,
} from "@/lib/api/marketplace-api";

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
  branding?: StorefrontBranding | null,
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
    branding?.seo_default_description?.trim() ||
    `Shop ${store.name} on Mark8ly. ${store.country_code} · ${store.currency_code}.`;

  // Merchant-provided title template takes precedence. The template
  // must contain `%s` — that's where Next.js inserts the page title.
  const customTemplate = branding?.seo_title_template?.trim();
  const titleTemplate =
    customTemplate && customTemplate.includes("%s")
      ? customTemplate
      : `%s · ${store.name}`;

  const ogImage = branding?.seo_og_image_url?.trim() || undefined;
  const twitterHandle = branding?.seo_twitter_handle?.trim() || undefined;
  const aiPolicy = branding?.seo_ai_policy ?? "allow";

  return {
    metadataBase: new URL(canonical),
    title: {
      default: store.name,
      template: titleTemplate,
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
      ...(ogImage ? { images: [ogImage] } : {}),
    },
    twitter: {
      card: "summary_large_image",
      title: store.name,
      description,
      ...(twitterHandle ? { site: twitterHandle, creator: twitterHandle } : {}),
      ...(ogImage ? { images: [ogImage] } : {}),
    },
    robots: {
      index: true,
      follow: true,
      ...(aiPolicy === "deny" && { nocache: true }),
    },
    // Merchant-owned favicon when uploaded in admin → Settings →
    // Branding; otherwise the built-in default ships from
    // /public/favicon.ico. Apple touch icon falls back to the same
    // asset — browsers tolerate a .ico there, and keeping one URL
    // avoids a separate upload surface for something 99% of merchants
    // won't customise.
    icons: (() => {
      const custom = branding?.favicon_url?.trim();
      const src = custom || "/favicon.ico";
      return {
        icon: src,
        shortcut: src,
        apple: custom || "/icon-192.png",
      };
    })(),
    other: {
      "store:slug": store.slug,
      "store:currency": store.currency_code,
      "store:country": store.country_code,
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
 * schema.org Product JSON-LD for a storefront product page. Makes each
 * product eligible for Google product rich results (price, availability,
 * image). Prices come from the tenant's own price_range; availability is
 * InStock when any variant has stock. A single Offer is emitted when the
 * range collapses to one price, otherwise an AggregateOffer.
 *
 * The returned object is serialized by <StructuredData>, which escapes
 * `<` so merchant-controlled title/description can't break out of the
 * <script> tag.
 */
export function buildProductJsonLd(
  store: PublicStore,
  product: StorefrontProduct,
): Record<string, unknown> {
  const url = `${canonicalUrl(store.slug)}/products/${product.handle}`;
  const images = product.media
    .filter((m) => m.media_type === "image")
    .map((m) => m.url);
  const { min, max, currency_code } = product.price_range;
  const availability = product.variants.some((v) => v.in_stock)
    ? "https://schema.org/InStock"
    : "https://schema.org/OutOfStock";
  const description = product.seo_description ?? product.description;

  const offers =
    min === max
      ? {
          "@type": "Offer",
          price: min,
          priceCurrency: currency_code,
          availability,
          url,
        }
      : {
          "@type": "AggregateOffer",
          lowPrice: min,
          highPrice: max,
          priceCurrency: currency_code,
          offerCount: product.variants.length,
          availability,
          url,
        };

  return {
    "@context": "https://schema.org",
    "@type": "Product",
    name: product.title,
    ...(description ? { description } : {}),
    ...(images.length > 0 ? { image: images } : {}),
    ...(product.categories[0]
      ? { category: product.categories[0].name }
      : {}),
    brand: { "@type": "Brand", name: store.name },
    offers,
  };
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
