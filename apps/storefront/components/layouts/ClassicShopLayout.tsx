import {
  Card,
  CenteredCta,
  HeroTitle,
  type LayoutProps,
} from "./shared";

/**
 * Classic shop layout — the most familiar storefront pattern.
 *
 * Centered hero, three-up value props, single CTA. Use when the
 * merchant wants something that feels recognisable without being
 * generic — the surface system carries the brand even though the
 * structure is conventional.
 */
export function ClassicShopLayout({ store, theme }: LayoutProps) {
  return (
    <section className="space-y-10">
      <HeroTitle store={store} theme={theme} align="center" />
      <div className="mx-auto grid max-w-5xl gap-4 md:grid-cols-3">
        {classicHighlights.map((item) => (
          <Card
            key={item.title}
            theme={theme}
            title={item.title}
            body={item.body}
          />
        ))}
      </div>
      <CenteredCta theme={theme} />
    </section>
  );
}

const classicHighlights = [
  {
    title: "Ready to launch",
    body: "A balanced storefront shell that feels familiar without looking generic.",
  },
  {
    title: "Clear merchandising",
    body: "Three-up rhythm puts the products front and centre without crowding the page.",
  },
  {
    title: "Designed to convert",
    body: "Quiet hierarchy and a single CTA keep the path forward obvious.",
  },
];
