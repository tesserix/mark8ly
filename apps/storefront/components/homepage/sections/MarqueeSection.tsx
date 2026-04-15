import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

import { marqueeStylesFor } from "@/lib/themeBlockStyles";

type MarqueeSectionProps = {
  section: Extract<HomepageSection, { type: "marquee" }>;
  theme: StorefrontTheme;
};

const MAX_ITEMS = 8;

export function MarqueeSection({ section, theme }: MarqueeSectionProps) {
  const styles = marqueeStylesFor(theme.layout);
  const items = section.items.slice(0, MAX_ITEMS);
  const speed = section.speed ?? "normal";

  if (items.length === 0) return null;

  // Track contains the items twice so the CSS keyframe (-50% translate)
  // produces a seamless loop. `motion-reduce:animate-none` honours
  // prefers-reduced-motion at the utility layer; the global CSS rule
  // belt-and-suspenders pins the animation to one frame.
  return (
    <section className={styles.section} aria-label="Brand marquee">
      <div
        className={`${styles.track} motion-reduce:animate-none motion-reduce:overflow-x-auto`}
        data-speed={speed}
      >
        {items.map((item, i) => (
          <span key={`a-${i}-${item}`} className={styles.item}>
            {item}
          </span>
        ))}
        <span aria-hidden="true" className="contents">
          {items.map((item, i) => (
            <span key={`b-${i}-${item}`} className={styles.item}>
              {item}
            </span>
          ))}
        </span>
      </div>
    </section>
  );
}
