import {
  Eyebrow,
  HeroTitle,
  LinkCta,
  ProductSlot,
  PullQuote,
  headingStyle,
  type LayoutProps,
} from "./shared";

/**
 * Story-led — long-form brand narrative.
 *
 * Reads like an essay: eyebrow, big headline, narrative body, inline
 * image, pull quote, then a gentle product lead-in at the bottom.
 * For founder-led brands where the why matters more than the what.
 */
export function StoryLedLayout({ store, theme }: LayoutProps) {
  return (
    <article className="mx-auto max-w-4xl space-y-16">
      <header className="space-y-6">
        <Eyebrow theme={theme}>A story · Founded 2026</Eyebrow>
        <HeroTitle store={store} theme={theme} size="lg" />
        <p className="max-w-2xl text-xl leading-9 opacity-80">
          {store.name} began as a single question: what if a store felt like a
          letter from the people who made it?
        </p>
      </header>

      <section className="grid gap-8 sm:grid-cols-[0.4fr_0.6fr] sm:items-start">
        <ProductSlot theme={theme} label="Workshop" ratio="aspect-[4/5]" />
        <div className="space-y-5 text-base leading-8 opacity-80">
          <p>
            Every piece starts in a small studio in {store.country_code}. The
            catalog is short by design — we only list things we&apos;ve worn,
            used, and lived with.
          </p>
          <p>
            Seasons move slower here. Restocks happen when the work is done.
            Customer notes get read the same week they arrive.
          </p>
        </div>
      </section>

      <PullQuote
        theme={theme}
        quote="The best storefronts feel like walking into a studio, not a supermarket."
        attribution={`${store.name} · Founder's note`}
      />

      <section className="grid gap-6 sm:grid-cols-2">
        <ProductSlot theme={theme} label="Chapter 01" ratio="aspect-[5/4]" />
        <ProductSlot theme={theme} label="Chapter 02" ratio="aspect-[5/4]" />
      </section>

      <footer className="border-t pt-10"
        style={{ borderColor: `${theme.colors.primary}22` }}
      >
        <h2
          className="text-3xl font-medium tracking-tight sm:text-4xl"
          style={headingStyle()}
        >
          When you&apos;re ready, we&apos;re ready.
        </h2>
        <p className="mt-4 max-w-xl text-base leading-8 opacity-75">
          No urgency, no countdown timers. Just quiet things we made because
          we wanted them to exist.
        </p>
        <div className="mt-6">
          <LinkCta theme={theme}>Browse the catalog</LinkCta>
        </div>
      </footer>
    </article>
  );
}
