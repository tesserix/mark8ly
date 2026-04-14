import Link from "next/link";

import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";
import { listProducts } from "@/lib/api/marketplace-api";

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
  const data = await listProducts(storeSlug, {
    categorySlug: section.collection_slug,
    pageSize: limit,
  });
  const products = data?.data ?? [];

  // Silent empty state — the collection may have been deleted. Admin
  // surfaces a warning badge on the section card (Task 13); storefront
  // renders nothing so customers never see a broken section.
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
            <p className="mt-3 font-serif text-base text-foreground transition-colors group-hover:text-moss-700">
              {p.title}
            </p>
          </Link>
        ))}
      </div>
    </section>
  );
}
