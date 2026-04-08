import {
  StoryPanel,
  headingStyle,
  type LayoutProps,
} from "./shared";

/**
 * Story-led layout — perspective before catalog. Stub.
 */
export function StoryLedLayout({ store, theme }: LayoutProps) {
  return (
    <section className="grid gap-10 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)] lg:items-center">
      <div className="space-y-4">
        <p
          className="text-[11px] font-semibold uppercase tracking-[0.24em]"
          style={{ color: `${theme.colors.accent}D9` }}
        >
          Story-led layout
        </p>
        <h1
          className="text-5xl font-medium tracking-tight sm:text-6xl"
          style={headingStyle()}
        >
          {store.name}
        </h1>
      </div>
      <StoryPanel
        theme={theme}
        eyebrow="Brand perspective"
        title="Tell customers why this store exists before you ask them to browse."
        body="This layout works well for founder-led brands, crafted products, and stores where the point of view matters as much as the catalog."
      />
    </section>
  );
}
