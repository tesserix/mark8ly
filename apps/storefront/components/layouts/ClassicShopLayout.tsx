import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";
import {
  HeroTitle,
  LinkCta,
  PrimaryButton,
  ProductSlot,
  SectionLabel,
  headingStyle,
  type LayoutProps,
} from "./shared";

/**
 * Classic Shop — the familiar storefront pattern, modernized.
 *
 * Tight sticky category rail, centered hero with generous whitespace,
 * then a proper 4-column product grid with reserved aspect ratios.
 * The surface system + theme tokens carry the brand so the
 * conventional structure never reads as generic.
 */
export function ClassicShopLayout({ store, theme, content }: LayoutProps) {
  return (
    <article className="space-y-16">
      <nav
        className="-mx-6 flex gap-8 overflow-x-auto border-b px-6 pb-4 text-[11px] font-semibold uppercase tracking-[0.2em]"
        style={{
          borderColor: `${theme.colors.primary}22`,
          color: `${theme.colors.primary}CC`,
        }}
      >
        {categories.map((c) => (
          <span key={c} className="shrink-0">
            {c}
          </span>
        ))}
      </nav>

      {content?.hero !== undefined ? (
        <HeroSection hero={content.hero} theme={theme} fallbackHeading={store.name} />
      ) : (
        <section className="mx-auto max-w-3xl space-y-6 py-6 text-center">
          <p
            className="text-[11px] font-semibold uppercase tracking-[0.24em]"
            style={{ color: `${theme.colors.accent}D9` }}
          >
            Spring collection · Live now
          </p>
          <HeroTitle store={store} theme={theme} align="center" size="lg" />
          <p className="mx-auto max-w-xl text-lg leading-8 opacity-75">
            A clean storefront for merchants who want familiar footing without
            sacrificing craft.
          </p>
          <div className="flex justify-center pt-2">
            <PrimaryButton theme={theme} size="lg">
              Shop everything
            </PrimaryButton>
          </div>
        </section>
      )}

      <section>
        <SectionLabel
          theme={theme}
          label="New arrivals"
          title="What's in this week"
          trailing={<LinkCta theme={theme}>View all</LinkCta>}
        />
        <div className="mt-6 grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
          {["N° 01", "N° 02", "N° 03", "N° 04"].map((label) => (
            <div key={label} className="space-y-3">
              <ProductSlot theme={theme} label={label} />
              <div>
                <p
                  className="text-sm font-semibold"
                  style={headingStyle()}
                >
                  Product name
                </p>
                <p className="text-xs opacity-60">Reserved price</p>
              </div>
            </div>
          ))}
        </div>
      </section>

      <section
        className="grid gap-6 border-y py-10 sm:grid-cols-3"
        style={{ borderColor: `${theme.colors.primary}22` }}
      >
        {valueProps.map((v) => (
          <div key={v.title}>
            <p
              className="text-[11px] font-semibold uppercase tracking-[0.2em]"
              style={{ color: `${theme.colors.accent}D9` }}
            >
              {v.title}
            </p>
            <p className="mt-2 text-sm leading-7 opacity-75">{v.body}</p>
          </div>
        ))}
      </section>

      {content?.sections && content.sections.length > 0 ? (
        <SectionsRenderer sections={content.sections} theme={theme} storeSlug={store.slug} />
      ) : null}
    </article>
  );
}

const categories = [
  "All",
  "New in",
  "Best sellers",
  "Collections",
  "Gifts",
  "Sale",
  "Journal",
];

const valueProps = [
  {
    title: "Free shipping",
    body: "Worldwide, carbon offset at checkout.",
  },
  {
    title: "Thirty day returns",
    body: "Change your mind, no questions asked.",
  },
  {
    title: "Made to last",
    body: "Repairable, recyclable, documented.",
  },
];
