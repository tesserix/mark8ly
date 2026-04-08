import { ActionRow, HeroTitle, StoryPanel, type LayoutProps } from "./shared";

/**
 * Editorial layout — the Mark8ly default.
 *
 * A warmer, more authored frame that opens with a strong serif
 * headline and a story panel beside it. Best for brands that want
 * to lead with craft, clarity, and a memorable first impression.
 *
 * Asymmetric grid (1.1fr / 0.9fr), left-aligned hero, hairline
 * surfaces — never centered, never card-everything.
 */
export function EditorialLayout({ store, theme }: LayoutProps) {
  return (
    <section className="grid gap-8 lg:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)] lg:items-end">
      <div className="space-y-6">
        <HeroTitle store={store} theme={theme} />
        <p className="max-w-2xl text-lg leading-8 opacity-80">
          A warmer, more authored storefront for merchants who want to lead
          with craft, clarity, and a memorable first impression.
        </p>
        <ActionRow theme={theme} />
      </div>
      <StoryPanel
        theme={theme}
        eyebrow="Launch mood"
        title="A storefront with room for story, products, and launch signals."
        body="Use this layout when your brand needs a richer opening frame before customers start browsing."
      />
    </section>
  );
}
