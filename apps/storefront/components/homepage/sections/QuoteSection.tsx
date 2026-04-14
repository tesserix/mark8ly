import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

type QuoteSectionProps = {
  section: Extract<HomepageSection, { type: "quote" }>;
  theme: StorefrontTheme;
};

export function QuoteSection({ section }: QuoteSectionProps) {
  return (
    <section className="mx-auto max-w-3xl px-6 text-center">
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
