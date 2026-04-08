import { Card, HeroTitle, type LayoutProps } from "./shared";

/**
 * Catalog-first layout — browsing starts immediately. Stub.
 */
export function CatalogFirstLayout({ store, theme }: LayoutProps) {
  return (
    <section className="space-y-8">
      <HeroTitle store={store} theme={theme} />
      <div className="grid gap-4 md:grid-cols-4">
        {catalogTiles.map((item) => (
          <Card
            key={item}
            theme={theme}
            title={item}
            body="Use this layout when browsing should start immediately."
            compact
          />
        ))}
      </div>
    </section>
  );
}

const catalogTiles = ["Featured", "New in", "Best sellers", "Collections"];
