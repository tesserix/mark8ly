import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";
import { Markdown } from "@/lib/markdown";

type TextSectionProps = {
  section: Extract<HomepageSection, { type: "text" }>;
  theme: StorefrontTheme;
};

export function TextSection({ section }: TextSectionProps) {
  return (
    <section className="mx-auto max-w-3xl px-6">
      {section.heading ? (
        <h2 className="font-serif text-3xl font-medium tracking-tight text-foreground">
          {section.heading}
        </h2>
      ) : null}
      <Markdown className="prose mt-6 max-w-none prose-headings:font-serif prose-headings:text-foreground prose-a:text-moss-700">
        {section.markdown}
      </Markdown>
    </section>
  );
}
