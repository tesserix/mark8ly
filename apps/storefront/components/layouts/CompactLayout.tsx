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
 * Compact — dense, operational.
 *
 * Tight header, sidebar nav on desktop, 6-up product grid. For wider
 * catalogs with less editorial overhead — the storefront version of
 * a properly-organized workbench.
 */
export function CompactLayout({ store, theme, content }: LayoutProps) {
  return (
    <article className="grid gap-10 lg:grid-cols-[14rem_minmax(0,1fr)]">
      <aside className="space-y-6 lg:sticky lg:top-6 lg:self-start">
        <div>
          <h1
            className="text-2xl font-medium tracking-tight"
            style={headingStyle()}
          >
            {store.name}
          </h1>
          <p
            className="mt-1 text-[11px] font-semibold uppercase tracking-[0.18em]"
            style={{ color: `${theme.colors.primary}99` }}
          >
            {store.slug}.mark8ly.com
          </p>
        </div>

        <nav
          className="space-y-1 border-t pt-4 text-sm"
          style={{ borderColor: `${theme.colors.primary}22` }}
        >
          {compactNav.map((item) => (
            <a
              key={item}
              href="#"
              className="block py-1.5 opacity-70 hover:opacity-100"
            >
              {item}
            </a>
          ))}
        </nav>

        <div
          className="border-t pt-4 text-xs leading-6 opacity-65"
          style={{ borderColor: `${theme.colors.primary}22` }}
        >
          Ships from {store.country_code}. Questions? Email the studio.
        </div>
      </aside>

      <div className="space-y-10">
        <SectionLabel
          theme={theme}
          label="Everything"
          title={`All ${store.name} products`}
          trailing={<LinkCta theme={theme}>Grid · List</LinkCta>}
        />

        <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {["N° 01", "N° 02", "N° 03", "N° 04", "N° 05", "N° 06"].map((label) => (
            <div key={label} className="space-y-2">
              <ProductSlot theme={theme} label={label} ratio="aspect-square" />
              <div className="flex items-baseline justify-between">
                <p className="text-sm font-semibold" style={headingStyle()}>
                  Product name
                </p>
                <p className="text-xs opacity-60">$—</p>
              </div>
            </div>
          ))}
        </div>

        {content?.sections && content.sections.length > 0 ? (
          <SectionsRenderer sections={content.sections} theme={theme} storeSlug={store.slug} />
        ) : null}
      </div>
    </article>
  );
}

const compactNav = [
  "All products",
  "New in",
  "Best sellers",
  "Collections",
  "Gifts",
  "Sale",
  "Journal",
  "About",
];
