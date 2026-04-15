/**
 * Storefront client for marketplace-api's public storefront routes
 * (M6). Mirrors the shape of platform-api.ts — public-only, no
 * session, returns null on 4xx so callers can render not-found
 * states without try/catch.
 *
 * Forwards the X-Storefront-Key header from MARKETPLACE_STOREFRONT_KEY
 * env var when set; M6 marketplace-api accepts an empty key for local
 * dev so the storefront still works without a configured secret.
 */

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function commonHeaders(): HeadersInit {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;
  return headers;
}

export interface StorefrontVariantOptionRef {
  option_name: string;
  value: string;
}

export interface StorefrontProductOptionValue {
  value: string;
  position: number;
}

export interface StorefrontProductOption {
  name: string;
  values: StorefrontProductOptionValue[];
}

export interface StorefrontVariant {
  id: string;
  price: string;
  compare_at_price: string | null;
  currency_code: string;
  in_stock: boolean;
  low_stock: boolean;
  option_values: StorefrontVariantOptionRef[];
}

export interface StorefrontMedia {
  url: string;
  alt: string | null;
  position: number;
  media_type: "image" | "video";
  width: number | null;
  height: number | null;
}

export interface StorefrontCategoryRef {
  name: string;
  slug: string;
}

export interface StorefrontPriceRange {
  min: string;
  max: string;
  currency_code: string;
}

export interface StorefrontProduct {
  id: string;
  handle: string;
  title: string;
  description: string | null;
  tags: string[];
  seo_title: string | null;
  seo_description: string | null;
  categories: StorefrontCategoryRef[];
  options: StorefrontProductOption[];
  variants: StorefrontVariant[];
  media: StorefrontMedia[];
  price_range: StorefrontPriceRange;
  published_at: string;
}

export interface StorefrontCategory {
  name: string;
  slug: string;
  position: number;
}

export interface ListMeta {
  page: number;
  page_size: number;
}

export interface ListProductsResponse {
  data: StorefrontProduct[];
  meta: ListMeta;
}

export interface ListCategoriesResponse {
  data: StorefrontCategory[];
}

interface ListProductsOptions {
  page?: number;
  pageSize?: number;
  categorySlug?: string;
  search?: string;
}

/**
 * Lists published products under a store. Returns null on 404 (unknown
 * store) so the caller can render a not-found state.
 */
export async function listProducts(
  storeSlug: string,
  options: ListProductsOptions = {},
): Promise<ListProductsResponse | null> {
  const params = new URLSearchParams();
  if (options.page) params.set("page", String(options.page));
  if (options.pageSize) params.set("page_size", String(options.pageSize));
  if (options.search) params.set("search", options.search);
  const qs = params.toString();
  const path = options.categorySlug
    ? `categories/${encodeURIComponent(options.categorySlug)}/products`
    : `products`;
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
    storeSlug,
  )}/${path}${qs ? `?${qs}` : ""}`;
  try {
    const res = await fetch(url, {
      headers: commonHeaders(),
      next: { revalidate: 60 },
    });
    if (res.status === 404) return null;
    if (!res.ok) return null;
    return (await res.json()) as ListProductsResponse;
  } catch {
    return null;
  }
}

/**
 * Resolves a batched set of product handles in a single request. Used
 * by the featured_products homepage block when the merchant hand-picks
 * products instead of selecting a collection. Missing handles silently
 * drop (the API does not 404 on partial matches), and the order the
 * handles are returned in matches the order they were requested in.
 *
 * Caller should cap at 6 slugs — matches the validator's
 * maxProductSlugs and the API handler's maxStorefrontSlugBatch.
 */
export async function fetchProductsBySlugs(
  storeSlug: string,
  slugs: string[],
): Promise<StorefrontProduct[]> {
  if (slugs.length === 0) return [];
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
    storeSlug,
  )}/products?slugs=${slugs.map(encodeURIComponent).join(",")}`;
  try {
    const res = await fetch(url, {
      headers: commonHeaders(),
      next: { revalidate: 60 },
    });
    if (!res.ok) return [];
    const body = (await res.json()) as { data?: StorefrontProduct[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}

/**
 * Fetches a single published product by handle.
 */
export async function getProductByHandle(
  storeSlug: string,
  handle: string,
): Promise<StorefrontProduct | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
    storeSlug,
  )}/products/${encodeURIComponent(handle)}`;
  try {
    const res = await fetch(url, {
      headers: commonHeaders(),
      next: { revalidate: 60 },
    });
    if (res.status === 404) return null;
    if (!res.ok) return null;
    return (await res.json()) as StorefrontProduct;
  } catch {
    return null;
  }
}

/**
 * Lists active categories for a store.
 */
export async function listCategories(
  storeSlug: string,
): Promise<StorefrontCategory[]> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
    storeSlug,
  )}/categories`;
  try {
    const res = await fetch(url, {
      headers: commonHeaders(),
      next: { revalidate: 60 },
    });
    if (!res.ok) return [];
    const body = (await res.json()) as ListCategoriesResponse;
    return body.data ?? [];
  } catch {
    return [];
  }
}

// ---------------------------------------------------------------------------
// Homepage content
// ---------------------------------------------------------------------------

export interface HomepageHero {
  enabled: boolean;
  image_url?: string | null;
  heading?: string | null;
  subheading?: string | null;
  cta_label?: string | null;
  cta_url?: string | null;
  eyebrow?: string | null;
  cta_secondary_label?: string | null;
  cta_secondary_url?: string | null;
  aside_image_url?: string | null;
  aside_image_alt?: string | null;
}

export type HomepageSection =
  | { type: "text"; heading?: string | null; markdown: string }
  | { type: "image"; url: string; alt?: string | null; caption?: string | null; heading?: string | null }
  | {
      type: "featured_products";
      collection_slug?: string;
      product_slugs?: string[];
      limit?: number;
      heading?: string | null;
    }
  | { type: "quote"; text: string; attribution?: string | null; heading?: string | null }
  | { type: "marquee"; items: string[]; speed?: "slow" | "normal" | "fast" }
  | { type: "pull_quote"; text: string; attribution?: string | null; heading?: string | null }
  | {
      type: "letter";
      eyebrow?: string | null;
      title: string;
      body: string;
      cta_label?: string | null;
      cta_url?: string | null;
    };

export interface HomepageContent {
  hero?: HomepageHero;
  sections?: HomepageSection[];
}

// ---------------------------------------------------------------------------
// Branding + active promotion
// ---------------------------------------------------------------------------

export interface ActivePromotion {
  label: string;
  coupon_code?: string;
  link?: string;
}

export interface FooterLinkItem {
  label: string;
  kind: "page" | "url";
  page_slug?: string;
  url?: string;
}

export interface FooterSection {
  label: string;
  items: FooterLinkItem[];
}

export interface StorefrontBranding {
  // announcement (existing)
  announcement_text?: string;
  announcement_link?: string;
  announcement_bg?: string;
  announcement_active: boolean;

  // footer identity
  footer_tagline?: string | null;
  footer_copyright?: string | null;

  // social links
  social_instagram?: string | null;
  social_twitter?: string | null;
  social_facebook?: string | null;
  social_tiktok?: string | null;
  social_youtube?: string | null;

  // footer menu
  footer_sections?: FooterSection[];

  // homepage content (hero + sections). Wire format is the raw JSONB
  // object; empty stores see `{}`.
  homepage_content?: HomepageContent;
}

export interface StorefrontStoreSummary {
  name: string;
  slug: string;
  currency_code: string;
  country_code: string;
}

export interface BrandingResponse {
  branding: StorefrontBranding;
  store?: StorefrontStoreSummary;
  active_promotion?: ActivePromotion;
}

export async function fetchBranding(
  storeSlug: string,
): Promise<BrandingResponse | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/branding`;
  try {
    const res = await fetch(url, {
      headers: commonHeaders(),
      next: { revalidate: 300 },
    });
    if (!res.ok) return null;
    return (await res.json()) as BrandingResponse;
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Pages
// ---------------------------------------------------------------------------

export interface StorefrontPage {
  slug: string;
  title: string;
  body: string;
  seo_title: string | null;
  seo_description: string | null;
}

export interface StorefrontPageSummary {
  slug: string;
  title: string;
}

/**
 * Lists published pages for a store. Returns an empty array on 4xx or
 * network failure so consumers (e.g. the footer) can render an empty
 * state without try/catch.
 *
 * Note: the backend returns a bare array (not wrapped in {data: [...]}).
 */
export async function listPages(
  storeSlug: string,
): Promise<StorefrontPageSummary[]> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/pages`;
  try {
    const res = await fetch(url, {
      headers: commonHeaders(),
      next: { revalidate: 300 },
    });
    if (!res.ok) return [];
    return (await res.json()) as StorefrontPageSummary[];
  } catch {
    return [];
  }
}

/**
 * Fetches a single published page by slug. Returns null on 404 or any
 * other non-OK status so the route can render `notFound()`.
 */
export async function fetchPage(
  storeSlug: string,
  pageSlug: string,
): Promise<StorefrontPage | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/pages/${encodeURIComponent(pageSlug)}`;
  try {
    const res = await fetch(url, {
      headers: commonHeaders(),
      next: { revalidate: 60 },
    });
    if (res.status === 404) return null;
    if (!res.ok) return null;
    return (await res.json()) as StorefrontPage;
  } catch {
    return null;
  }
}
