import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";
import { safeUrl } from "@/lib/safeUrl";

type ImageSectionProps = {
  section: Extract<HomepageSection, { type: "image" }>;
  theme: StorefrontTheme;
};

export function ImageSection({ section }: ImageSectionProps) {
  const alt = section.alt?.trim() || "";
  const src = safeUrl(section.url);
  return (
    <section className="mx-auto max-w-5xl px-6">
      {section.heading ? (
        <h2 className="font-serif text-3xl font-medium tracking-tight text-foreground">
          {section.heading}
        </h2>
      ) : null}
      {src !== "#" ? (
        <figure className={section.heading ? "mt-6" : ""}>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={src}
            alt={alt}
            className="w-full rounded-md"
            loading="lazy"
          />
          {section.caption ? (
            <figcaption className="mt-2 text-sm text-foreground-secondary">
              {section.caption}
            </figcaption>
          ) : null}
        </figure>
      ) : null}
    </section>
  );
}
