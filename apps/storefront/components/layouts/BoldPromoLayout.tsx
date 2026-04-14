import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";
import {
  HeroTitle,
  PrimaryButton,
  ProductSlot,
  headingStyle,
  type LayoutProps,
} from "./shared";

/**
 * Bold promo — full-bleed statement frame with numeric markers.
 *
 * High-energy without being loud. Uses large serif numerals
 * (01 / 02 / 03) as structure rather than countdown urgency. For
 * seasonal drops and launch moments where the first fold needs
 * presence but not shouting.
 */
export function BoldPromoLayout({ store, theme, content }: LayoutProps) {
  return (
    <article className="space-y-16">
      {content?.hero !== undefined ? (
        <HeroSection hero={content.hero} theme={theme} fallbackHeading={store.name} />
      ) : (
        <section
          className="-mx-6 overflow-hidden px-6 py-16 sm:-mx-8 sm:px-12 sm:py-20"
          style={{
            background: `linear-gradient(135deg, ${theme.colors.primary} 0%, ${theme.colors.accent} 100%)`,
            color: "#fff",
          }}
        >
          <div className="grid gap-10 lg:grid-cols-12 lg:items-end">
            <div className="space-y-6 lg:col-span-7">
              <p className="text-[11px] font-semibold uppercase tracking-[0.28em] text-white/75">
                The drop · Live now
              </p>
              <HeroTitle store={store} theme={theme} size="xl" />
              <p className="max-w-xl text-lg leading-8 text-white/80">
                A limited edition release. When it&apos;s gone, it&apos;s gone.
              </p>
              <div className="flex flex-wrap gap-4 pt-2">
                <button
                  type="button"
                  className="inline-flex h-14 items-center justify-center bg-white px-8 text-sm font-semibold uppercase tracking-[0.14em] text-neutral-900 transition-opacity hover:opacity-90"
                  style={{ borderRadius: "var(--store-radius)" }}
                >
                  Shop the drop
                </button>
                <button
                  type="button"
                  className="inline-flex h-14 items-center justify-center border border-white/40 px-8 text-sm font-semibold uppercase tracking-[0.14em] text-white transition-colors hover:bg-white/10"
                  style={{ borderRadius: "var(--store-radius)" }}
                >
                  Lookbook
                </button>
              </div>
            </div>

            <div className="grid grid-cols-3 gap-6 lg:col-span-5">
              {dropStats.map((stat) => (
                <div key={stat.label}>
                  <p
                    className="text-5xl font-medium leading-none sm:text-6xl"
                    style={headingStyle()}
                  >
                    {stat.value}
                  </p>
                  <p className="mt-2 text-[10px] font-semibold uppercase tracking-[0.2em] text-white/70">
                    {stat.label}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </section>
      )}

      <section>
        <div
          className="flex items-end justify-between gap-4 border-b pb-4"
          style={{ borderColor: `${theme.colors.primary}22` }}
        >
          <h2
            className="text-2xl font-medium tracking-tight sm:text-3xl"
            style={headingStyle()}
          >
            The pieces
          </h2>
        </div>
        <div className="mt-6 grid gap-6 sm:grid-cols-3">
          {["N° 01", "N° 02", "N° 03"].map((label) => (
            <ProductSlot key={label} theme={theme} label={label} />
          ))}
        </div>
      </section>

      <div className="flex justify-center">
        <PrimaryButton theme={theme} size="lg">
          View all drops
        </PrimaryButton>
      </div>

      {content?.sections && content.sections.length > 0 ? (
        <SectionsRenderer sections={content.sections} theme={theme} storeSlug={store.slug} />
      ) : null}
    </article>
  );
}

const dropStats = [
  { value: "12", label: "Pieces" },
  { value: "01", label: "Drop" },
  { value: "∞", label: "Care" },
];
