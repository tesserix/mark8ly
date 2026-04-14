import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";
import {
  LinkCta,
  ProductSlot,
  SectionLabel,
  headingStyle,
  type LayoutProps,
} from "./shared";

/**
 * Catalog-first — browsing starts immediately.
 *
 * Minimal hero, then a proper product grid with varied slot sizes
 * (hero tile + 5 supporting tiles) that feels curated rather than
 * mechanical. Great for wide catalogs where discovery is the point.
 */
export function CatalogFirstLayout({ store, theme, content }: LayoutProps) {
  return (
    <article className="space-y-12">
      {content?.hero !== undefined ? (
        <HeroSection hero={content.hero} theme={theme} fallbackHeading={store.name} />
      ) : (
        <header className="flex flex-wrap items-end justify-between gap-4 border-b pb-6"
          style={{ borderColor: `${theme.colors.primary}22` }}
        >
          <div>
            <p
              className="text-[11px] font-semibold uppercase tracking-[0.24em]"
              style={{ color: `${theme.colors.accent}D9` }}
            >
              The catalog
            </p>
            <h1
              className="mt-2 text-4xl font-medium tracking-tight sm:text-5xl"
              style={headingStyle()}
            >
              {store.name}
            </h1>
          </div>
          <div className="flex flex-wrap gap-2 text-[11px] font-semibold uppercase tracking-[0.16em]">
            {filters.map((f) => (
              <span
                key={f}
                className="border px-3 py-2"
                style={{
                  borderColor: `${theme.colors.primary}33`,
                  borderRadius: "var(--store-radius)",
                }}
              >
                {f}
              </span>
            ))}
          </div>
        </header>
      )}

      <section className="grid gap-4 lg:grid-cols-4 lg:grid-rows-2">
        <div className="lg:col-span-2 lg:row-span-2">
          <ProductSlot
            theme={theme}
            label="Featured"
            ratio="aspect-square lg:aspect-auto lg:h-full"
            size="lg"
          />
        </div>
        {["N° 01", "N° 02", "N° 03", "N° 04"].map((label) => (
          <ProductSlot key={label} theme={theme} label={label} ratio="aspect-square" />
        ))}
      </section>

      <section>
        <SectionLabel
          theme={theme}
          label="Collections"
          title="Shop by mood"
          trailing={<LinkCta theme={theme}>See all collections</LinkCta>}
        />
        <div className="mt-6 grid gap-6 sm:grid-cols-3">
          {["Quiet essentials", "Workshop staples", "Gifts under $50"].map(
            (label) => (
              <div key={label} className="space-y-3">
                <ProductSlot theme={theme} label={label} ratio="aspect-[5/4]" />
                <p className="text-sm font-semibold" style={headingStyle()}>
                  {label}
                </p>
              </div>
            ),
          )}
        </div>
      </section>

      {content?.sections && content.sections.length > 0 ? (
        <SectionsRenderer sections={content.sections} theme={theme} storeSlug={store.slug} />
      ) : null}
    </article>
  );
}

const filters = ["All", "In stock", "New", "Under $100", "Gifts"];
