import Link from "next/link";

import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

import { letterStylesFor } from "@/lib/themeBlockStyles";
import { safeUrl } from "@/lib/safeUrl";
import { Markdown } from "@/lib/markdown";

type LetterSectionProps = {
  section: Extract<HomepageSection, { type: "letter" }>;
  theme: StorefrontTheme;
};

export function LetterSection({ section, theme }: LetterSectionProps) {
  const s = letterStylesFor(theme.layout);
  const href = section.cta_url ? safeUrl(section.cta_url) : null;
  const showCta = !!section.cta_label && !!href && href !== "#";

  return (
    <article className={s.container}>
      {section.eyebrow ? <p className={s.eyebrow}>{section.eyebrow}</p> : null}
      <h2 className={s.title}>{section.title}</h2>
      <Markdown className={s.body}>{section.body}</Markdown>
      {showCta ? (
        <Link href={href!} className={s.cta}>
          {section.cta_label}
        </Link>
      ) : null}
    </article>
  );
}
