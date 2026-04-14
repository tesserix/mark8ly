import Link from "next/link";

import type { HomepageHero } from "@/lib/api/marketplace-api";
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
 * Themes may wrap this component to apply their own positioning,
 * aspect ratio, or background treatment — this component stays
 * neutral.
 */
export function HeroSection({
  hero,
  fallbackHeading,
}: HeroSectionProps) {
  const enabled = hero?.enabled ?? true;
  if (!enabled) return null;

  const heading = hero?.heading?.trim() || fallbackHeading;
  const subheading = hero?.subheading?.trim() ?? "";
  const imageURL = hero?.image_url ?? null;
  const cta =
    hero?.cta_label && hero.cta_url
      ? { label: hero.cta_label, url: hero.cta_url }
      : null;

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
      <div className="relative px-8 py-24">
        <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
          {heading}
        </h1>
        {subheading ? (
          <p className="mt-5 max-w-2xl text-lg text-foreground-secondary">
            {subheading}
          </p>
        ) : null}
        {cta ? (
          <Link
            href={safeUrl(cta.url)}
            className="mt-8 inline-flex h-12 items-center rounded-md bg-ink-900 px-6 text-base font-medium text-paper-200 transition-colors hover:bg-moss-700"
          >
            {cta.label}
          </Link>
        ) : null}
      </div>
    </section>
  );
}
