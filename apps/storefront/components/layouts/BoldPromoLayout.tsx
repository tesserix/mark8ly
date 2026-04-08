import {
  PromoButton,
  PromoGhostButton,
  headingStyle,
  type LayoutProps,
} from "./shared";

/**
 * Bold-promo layout — high-energy launch frame. Stub.
 */
export function BoldPromoLayout({ store, theme }: LayoutProps) {
  return (
    <section
      className="rounded-[2rem] border p-8 sm:p-10"
      style={{
        background: `linear-gradient(135deg, ${theme.colors.primary}, ${theme.colors.accent})`,
        color: "#fff",
        borderColor: `${theme.colors.primary}66`,
      }}
    >
      <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-white/75">
        Promotional layout
      </p>
      <h1
        className="mt-4 text-5xl font-medium tracking-tight sm:text-6xl"
        style={headingStyle()}
      >
        {store.name}
      </h1>
      <p className="mt-4 max-w-2xl text-lg leading-8 text-white/85">
        For launches, seasonal drops, and stores that want the first fold to
        carry stronger energy.
      </p>
      <div className="mt-8 flex flex-wrap gap-3">
        <PromoButton>Browse launch drop</PromoButton>
        <PromoGhostButton>Read the story</PromoGhostButton>
      </div>
    </section>
  );
}
