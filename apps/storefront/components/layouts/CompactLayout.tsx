import {
  Card,
  HeroTitle,
  StoryPanel,
  type LayoutProps,
} from "./shared";

/**
 * Compact layout — dense, operational tone. Stub.
 */
export function CompactLayout({ store, theme }: LayoutProps) {
  return (
    <section className="space-y-5">
      <HeroTitle store={store} theme={theme} />
      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <div className="grid gap-3 sm:grid-cols-2">
          {compactTiles.map((item) => (
            <Card
              key={item}
              theme={theme}
              title={item}
              body="A tighter storefront rhythm for practical, category-first shops."
              compact
            />
          ))}
        </div>
        <StoryPanel
          theme={theme}
          eyebrow="Quick scan"
          title="Dense enough to browse quickly without losing polish."
          body="Ideal for stores with wider catalogs or a more operational, no-fuss brand voice."
        />
      </div>
    </section>
  );
}

const compactTiles = [
  "Featured arrivals",
  "Fast-moving picks",
  "Everyday essentials",
  "Seasonal updates",
];
