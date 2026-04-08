import {
  Eyebrow,
  HeroTitle,
  LinkCta,
  Marquee,
  PrimaryButton,
  ProductSlot,
  PullQuote,
  SecondaryButton,
  SectionLabel,
  headingStyle,
  type LayoutProps,
} from "./shared";

/**
 * Editorial — Mark8ly's default layout.
 *
 * Magazine-style opening: gigantic serif wordmark, off-center
 * asymmetric grid, editorial pull quote, then a three-up featured
 * row with honest product slots, capped by a brand marquee.
 *
 * The whole page reads left-to-right like a spread — left rail of
 * text, right rail of imagery — never centered hero + card grid.
 */
export function EditorialLayout({ store, theme }: LayoutProps) {
  return (
    <article className="space-y-20">
      <section className="grid gap-10 lg:grid-cols-12 lg:items-end">
        <div className="space-y-6 lg:col-span-7">
          <Eyebrow theme={theme}>Issue 01 · Now open</Eyebrow>
          <HeroTitle store={store} theme={theme} size="xl" />
          <p className="max-w-xl text-lg leading-8 opacity-75">
            A considered storefront for {store.name.toLowerCase()} — a place
            where the catalog feels written, not assembled.
          </p>
          <div className="flex flex-wrap items-center gap-4 pt-2">
            <PrimaryButton theme={theme} size="lg">
              Shop the edit
            </PrimaryButton>
            <LinkCta theme={theme}>Read the story</LinkCta>
          </div>
        </div>
        <aside className="lg:col-span-5">
          <ProductSlot
            theme={theme}
            label="Cover story"
            ratio="aspect-[4/5]"
            size="lg"
          />
        </aside>
      </section>

      <Marquee
        theme={theme}
        items={[
          "Hand picked",
          "Small batch",
          "Ships worldwide",
          store.slug.toUpperCase(),
          "Est. 2026",
        ]}
      />

      <section className="grid gap-10 lg:grid-cols-12">
        <div className="lg:col-span-4">
          <PullQuote
            theme={theme}
            quote="We chose fewer things and we chose them well."
            attribution={`Editor's note · ${store.name}`}
          />
        </div>
        <div className="lg:col-span-8">
          <SectionLabel
            theme={theme}
            label="The edit"
            title="Three pieces we love right now"
            trailing={<LinkCta theme={theme}>View all</LinkCta>}
          />
          <div className="mt-6 grid gap-6 sm:grid-cols-3">
            <ProductSlot theme={theme} label="N° 01" />
            <ProductSlot theme={theme} label="N° 02" />
            <ProductSlot theme={theme} label="N° 03" />
          </div>
        </div>
      </section>

      <section className="grid gap-8 border-t pt-10 lg:grid-cols-2"
        style={{ borderColor: `${theme.colors.primary}22` }}
      >
        <div className="space-y-4">
          <Eyebrow theme={theme}>Letter from the studio</Eyebrow>
          <h3
            className="text-3xl font-medium tracking-tight sm:text-4xl"
            style={headingStyle()}
          >
            Built for the long shelf life.
          </h3>
          <p className="text-base leading-8 opacity-75">
            Every piece in {store.name} is chosen to outlast a season. Paper
            packaging, repairable construction, and a quiet attitude toward
            trend cycles.
          </p>
          <SecondaryButton theme={theme}>About the studio</SecondaryButton>
        </div>
        <ProductSlot theme={theme} label="Workshop" ratio="aspect-[5/4]" />
      </section>
    </article>
  );
}
