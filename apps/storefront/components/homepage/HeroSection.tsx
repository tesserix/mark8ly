import Link from "next/link";

import type { HomepageHero } from "@/lib/api/marketplace-api";
import { heroStylesFor } from "@/lib/themeBlockStyles";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";
import { safeUrl } from "@/lib/safeUrl";

type HeroSectionProps = {
  hero: HomepageHero | undefined | null;
  theme: StorefrontTheme;
  fallbackHeading: string;
};

/**
 * Shared hero component used by every storefront layout. The hero is
 * enabled by default; merchants can set `hero.enabled = false` to hide
 * it entirely. When a heading is not authored, `fallbackHeading` (the
 * store name) is rendered so the homepage is never empty.
 *
 * Supports optional eyebrow label, secondary CTA, and aside image —
 * each gated by the layout's style map so only themes that declare
 * support for a slot actually render it.
 */
export function HeroSection({
  hero,
  theme,
  fallbackHeading,
}: HeroSectionProps) {
  const enabled = hero?.enabled ?? true;
  if (!enabled) return null;

  const heading = hero?.heading?.trim() || fallbackHeading;
  const subheading = hero?.subheading?.trim() ?? "";
  const imageURL = hero?.image_url ?? null;
  const eyebrow = hero?.eyebrow?.trim() ?? null;
  const cta =
    hero?.cta_label && hero.cta_url
      ? { label: hero.cta_label, url: hero.cta_url }
      : null;
  const secondaryCta =
    hero?.cta_secondary_label && hero.cta_secondary_url
      ? { label: hero.cta_secondary_label, url: hero.cta_secondary_url }
      : null;
  const asideImageUrl = hero?.aside_image_url ?? null;
  const asideImageAlt = hero?.aside_image_alt ?? "";

  const styles = heroStylesFor(theme.layout);

  return (
    <section className="relative isolate overflow-hidden">
      {imageURL && safeUrl(imageURL) !== "#" ? (
        /* eslint-disable-next-line @next/next/no-img-element */
        <img
          src={safeUrl(imageURL)}
          alt=""
          className="absolute inset-0 -z-10 h-full w-full object-cover opacity-70"
          loading="eager"
        />
      ) : null}
      <div className={styles.container}>
        {/* Text block */}
        <div className="flex-1">
          {eyebrow && styles.eyebrow ? (
            <p className={styles.eyebrow}>{eyebrow}</p>
          ) : null}
          <h1 className={styles.title}>{heading}</h1>
          {subheading ? (
            <p className={styles.subheading}>{subheading}</p>
          ) : null}
          {cta || (secondaryCta && styles.secondaryCta) ? (
            <div className="mt-8 flex flex-wrap items-center gap-4">
              {cta ? (
                <Link href={safeUrl(cta.url)} className={styles.primaryCta}>
                  {cta.label}
                </Link>
              ) : null}
              {secondaryCta && styles.secondaryCta ? (
                <Link
                  href={safeUrl(secondaryCta.url)}
                  className={styles.secondaryCta}
                >
                  {secondaryCta.label}
                </Link>
              ) : null}
            </div>
          ) : null}
        </div>
        {/* Aside image — only rendered when layout declares asideSlot support */}
        {asideImageUrl && styles.asideSlot && styles.asideImage && safeUrl(asideImageUrl) !== "#" ? (
          <div className={styles.asideSlot}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={safeUrl(asideImageUrl)}
              alt={asideImageAlt}
              className={styles.asideImage}
            />
          </div>
        ) : null}
      </div>
    </section>
  );
}
