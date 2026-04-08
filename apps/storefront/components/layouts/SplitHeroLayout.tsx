import { Card, MiniStat, StoryPanel, type LayoutProps } from "./shared";

/**
 * Split-hero layout — equal weight to message and action.
 *
 * Practical two-column structure that gives equal visual weight to
 * the store narrative and a featured action. Stub-quality today;
 * inherits the editorial primitives until merchants demand a
 * dedicated implementation.
 */
export function SplitHeroLayout({ store, theme }: LayoutProps) {
  return (
    <section className="grid gap-6 lg:grid-cols-2 lg:items-stretch">
      <StoryPanel
        theme={theme}
        eyebrow="Store launch"
        title={store.name}
        body="A practical split layout that gives equal weight to the store message and launch actions."
        large
      />
      <Card
        theme={theme}
        title="Open with confidence"
        body="Highlight collections, mention delivery expectations, or use this area for a featured launch offer."
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <MiniStat theme={theme} label="Visual style" value="Structured" />
          <MiniStat theme={theme} label="Best for" value="Focused launches" />
        </div>
      </Card>
    </section>
  );
}
