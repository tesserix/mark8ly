// Note: multiple FeaturedProductsSection instances with the same
// collection_slug (or same ordered set of product_slugs) will make
// identical fetch() calls — Next.js automatically dedupes these within
// a single render tree, so the backend only sees one request per
// unique (storeSlug, ...key) tuple. Revalidation window is 60s.
import Link from "next/link";

import type { HomepageSection, StorefrontProduct } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";
import { fetchProductsBySlugs, listProducts } from "@/lib/api/marketplace-api";

const MAX_SLUGS = 6;

type FeaturedProductsSectionProps = {
  section: Extract<HomepageSection, { type: "featured_products" }>;
  theme: StorefrontTheme;
  storeSlug: string;
};

export async function FeaturedProductsSection({
  section,
  storeSlug,
}: FeaturedProductsSectionProps) {
  const limit = Math.max(1, Math.min(section.limit ?? 8, 24));

  // Hand-picked mode: merchant chose specific products by slug. Order is
  // preserved server-side. Falls through to collection mode when no slugs.
  const handPickedSlugs =
    section.product_slugs && section.product_slugs.length > 0
      ? section.product_slugs.slice(0, MAX_SLUGS)
      : null;

  let products: StorefrontProduct[];
  if (handPickedSlugs) {
    products = await fetchProductsBySlugs(storeSlug, handPickedSlugs);
  } else if (section.collection_slug) {
    const data = await listProducts(storeSlug, {
      categorySlug: section.collection_slug,
      pageSize: limit,
    });
    products = data?.data ?? [];
  } else {
    products = [];
  }

  // Silent empty state — the collection may have been deleted or be empty.
  // Storefront renders nothing so customers never see a broken section.
  if (products.length === 0) return null;

  return (
    <section className="mx-auto max-w-6xl px-6">
      {section.heading ? (
        <h2 className="font-serif text-3xl font-medium tracking-tight text-foreground">
          {section.heading}
        </h2>
      ) : null}
      <div
        className={
          (section.heading ? "mt-8 " : "") +
          "grid grid-cols-2 gap-6 sm:grid-cols-3 lg:grid-cols-4"
        }
      >
        {products.slice(0, limit).map((p) => (
          <Link
            key={p.handle}
            href={`/products/${encodeURIComponent(p.handle)}`}
            className="group block"
          >
            {p.media?.[0]?.url ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={p.media[0].url}
                alt={p.media[0].alt ?? p.title}
                className="aspect-square w-full rounded-md object-cover"
                loading="lazy"
              />
            ) : (
              <div className="aspect-square w-full rounded-md bg-muted" />
            )}
            <p className="mt-3 font-serif text-base text-foreground transition-colors group-hover:text-[color:var(--storefront-accent,theme(colors.moss.700))]">
              {p.title}
            </p>
          </Link>
        ))}
      </div>
    </section>
  );
}
