import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

type QuoteSectionProps = {
  section: Extract<HomepageSection, { type: "quote" }>;
  theme: StorefrontTheme;
};

export function QuoteSection({ section }: QuoteSectionProps) {
  // Editorial asymmetry — left-aligned quote with a hairline rule as
  // the anchor, instead of a centred block. Reads like a pull quote
  // in a magazine spread rather than a testimonial card.
  return (
    <section className="mx-auto max-w-3xl px-6">
      <div
        aria-hidden
        className="mb-6 h-px w-12 bg-[color:var(--storefront-text,var(--ink-900))]/20"
      />
      <blockquote className="font-serif text-2xl italic leading-snug text-foreground sm:text-3xl">
        &ldquo;{section.text}&rdquo;
      </blockquote>
      {section.attribution ? (
        <p className="mt-4 text-sm uppercase tracking-wide text-foreground-secondary">
          — {section.attribution}
        </p>
      ) : null}
    </section>
  );
}
