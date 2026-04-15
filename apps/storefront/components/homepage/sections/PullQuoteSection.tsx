import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

import { pullQuoteStylesFor } from "@/lib/themeBlockStyles";

type PullQuoteSectionProps = {
  section: Extract<HomepageSection, { type: "pull_quote" }>;
  theme: StorefrontTheme;
};

export function PullQuoteSection({ section, theme }: PullQuoteSectionProps) {
  const s = pullQuoteStylesFor(theme.layout);
  return (
    <blockquote className={s.container}>
      <p className={s.text}>&ldquo;{section.text}&rdquo;</p>
      {section.attribution ? (
        <cite className={s.attribution}>— {section.attribution}</cite>
      ) : null}
    </blockquote>
  );
}
